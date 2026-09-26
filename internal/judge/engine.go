package judge

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"runtime"
	"sync"

	"github.com/hybridgroup/yzma/pkg/llama"
)

const (
	decodeChunk = 512
	ctxTokens   = 4096 + 64
)

// EngineOptions configure the llama.cpp backend.
type EngineOptions struct {
	LibDir, Model string
	CPU           bool // never offload to a GPU
	Threads       int  // 0: half the CPUs
}

// Engine scores prompts with llama.cpp via yzma. The model stays loaded
// until Close. Safe for concurrent use; calls are serialised.
type Engine struct {
	mu     sync.Mutex
	model  llama.Model
	ctx    llama.Context
	mem    llama.Memory
	vocab  llama.Vocab
	nVocab int
	slots  []llama.Token // token of each option letter
	GPU    bool
}

var loadOnce sync.Once
var loadErr error

// OpenEngine loads the libraries and the model. It uses the GPU when a
// GPU backend (Vulkan) is available, otherwise the CPU.
func OpenEngine(o EngineOptions) (*Engine, error) {
	loadOnce.Do(func() {
		if runtime.GOOS == "windows" {
			// Windows resolves a DLL's dependencies (ggml-base.dll, …) from
			// the exe folder and PATH, not from the DLL's own folder.
			os.Setenv("PATH", o.LibDir+string(os.PathListSeparator)+os.Getenv("PATH"))
		}
		if err := llama.Load(o.LibDir); err != nil {
			loadErr = fmt.Errorf("loading llama.cpp libraries from %s: %w", o.LibDir, err)
			return
		}
		llama.LogSet(llama.LogSilent())
		llama.Init()
	})
	if loadErr != nil {
		return nil, loadErr
	}
	threads := o.Threads
	if threads == 0 {
		threads = max(1, runtime.NumCPU()/2)
	}
	gpu := !o.CPU && llama.SupportsGpuOffload()

	mp := llama.ModelDefaultParams()
	mp.NGpuLayers = 0
	if gpu {
		mp.NGpuLayers = 99
	}
	model, err := llama.ModelLoadFromFile(o.Model, mp)
	if err != nil || model == 0 {
		return nil, fmt.Errorf("loading model %s: %v", o.Model, err)
	}
	cp := llama.ContextDefaultParams()
	cp.NCtx = ctxTokens
	cp.NSeqMax = 1
	cp.NOutputsMax = 1
	cp.NThreads = int32(threads)
	cp.NThreadsBatch = int32(threads)
	ctx, err := llama.InitFromModel(model, cp)
	if err != nil || ctx == 0 {
		llama.ModelFree(model)
		return nil, fmt.Errorf("creating llama context: %v", err)
	}
	mem, err := llama.GetMemory(ctx)
	if err != nil {
		return nil, err
	}
	vocab := llama.ModelGetVocab(model)
	e := &Engine{model: model, ctx: ctx, mem: mem, vocab: vocab, nVocab: int(llama.VocabNTokens(vocab)), GPU: gpu}
	for _, l := range letters {
		t := llama.Tokenize(vocab, string(l), false, false)
		buf := make([]byte, 16)
		if len(t) != 1 {
			e.Close()
			return nil, fmt.Errorf("letter %c is not a single token", l)
		}
		if n := llama.TokenToPiece(vocab, t[0], buf, 0, true); string(buf[:n]) != string(l) {
			e.Close()
			return nil, fmt.Errorf("letter %c does not round-trip", l)
		}
		e.slots = append(e.slots, t[0])
	}
	return e, nil
}

func (e *Engine) Close() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.ctx != 0 {
		llama.Free(e.ctx)
		e.ctx = 0
	}
	if e.model != 0 {
		llama.ModelFree(e.model)
		e.model = 0
	}
}

// Tokenize returns the token IDs of a prompt (special tokens parsed).
func (e *Engine) Tokenize(s string) []int {
	toks := e.tokenize(s)
	out := make([]int, len(toks))
	for i, t := range toks {
		out[i] = int(t)
	}
	return out
}

func (e *Engine) tokenize(s string) []llama.Token { return llama.Tokenize(e.vocab, s, false, true) }

// Score prefills the shared evidence prefix once, then decodes each gate's
// remaining tokens from that state. A single gate uses the same split so
// results do not depend on how many gates share the evidence.
func (e *Engine) Score(ctx context.Context, state string, gates []Gate) ([][]float64, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.ctx == 0 {
		return nil, errors.New("engine closed")
	}
	pre := e.tokenize(statePrefix(state))
	pre = pre[:len(pre)-1] // the last token may merge with what follows
	full := make([][]llama.Token, len(gates))
	for i, g := range gates {
		full[i] = e.tokenize(g.Prompt(state))
		if len(full[i]) > ctxTokens {
			return nil, fmt.Errorf("prompt for %s is %d tokens, over the context of %d", g.Name, len(full[i]), ctxTokens)
		}
		for k := range pre {
			if full[i][k] != pre[k] {
				pre = pre[:k] // tokenisation diverged; share less
				break
			}
		}
	}

	llama.MemoryClear(e.mem, false)
	if _, err := e.decode(pre, 0, false); err != nil {
		return nil, err
	}
	var saved []byte
	if len(gates) > 1 {
		size := llama.StateSeqGetSize(e.ctx, 0)
		saved = make([]byte, size)
		if llama.StateSeqGetData(e.ctx, saved, 0) != size {
			return nil, errors.New("incomplete state save")
		}
	}

	out := make([][]float64, len(gates))
	for i, g := range gates {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if i > 0 {
			if ok, _ := llama.MemorySeqRm(e.mem, 0, -1, -1); !ok {
				return nil, errors.New("clearing sequence failed")
			}
			if llama.StateSeqSetData(e.ctx, saved, 0) == 0 {
				return nil, errors.New("state restore failed")
			}
		}
		lg, err := e.decode(full[i][len(pre):], len(pre), true)
		if err != nil {
			return nil, err
		}
		out[i] = e.readout(lg, len(g.Options))
	}
	return out, nil
}

func (e *Engine) decode(toks []llama.Token, start int, wantLogits bool) ([]float32, error) {
	for off := 0; off < len(toks); off += decodeChunk {
		end := min(off+decodeChunk, len(toks))
		b := llama.BatchInit(int32(end-off), 0, 1)
		for i := off; i < end; i++ {
			if err := b.Add(toks[i], llama.Pos(start+i), []llama.SeqId{0}, wantLogits && i == len(toks)-1); err != nil {
				llama.BatchFree(b)
				return nil, err
			}
		}
		rc, err := llama.Decode(e.ctx, b)
		llama.BatchFree(b)
		if err != nil || rc != 0 {
			return nil, fmt.Errorf("decode rc=%d: %v", rc, err)
		}
	}
	if !wantLogits {
		return nil, nil
	}
	lg, err := llama.GetLogitsIth(e.ctx, -1, e.nVocab)
	if err != nil || lg == nil {
		return nil, fmt.Errorf("no logits: %v", err)
	}
	return lg, nil
}

// readout is the softmax over the logits of the first n option letters.
func (e *Engine) readout(lg []float32, n int) []float64 {
	vals := make([]float64, n)
	mx := math.Inf(-1)
	for i := 0; i < n; i++ {
		vals[i] = float64(lg[e.slots[i]])
		mx = math.Max(mx, vals[i])
	}
	sum := 0.0
	for i := range vals {
		vals[i] = math.Exp(vals[i] - mx)
		sum += vals[i]
	}
	for i := range vals {
		vals[i] /= sum
	}
	return vals
}
