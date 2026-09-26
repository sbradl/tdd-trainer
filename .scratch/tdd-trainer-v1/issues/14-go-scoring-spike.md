# Go scoring spike

Type: task
Status: resolved
Blocked by: none
Map: ../map.md

## Question

Can a Go program reproduce the judge's option scoring without Python? Build a throwaway Go spike (branch spike/go-scoring) that loads the Qwen3.5-4B Q4 GGUF via yzma (purego llama.cpp) or llama-server, applies the same chat template and state layout as prototype-judge-eval/judge.py and SemIf's direct mode, reads the option logits and softmaxes them. Run it on the fixtures in prototype-judge-eval and compare verdicts and p-values with regress-out.txt. Record: agreement, warm latency per gate, load time, binary + shared-lib size, whether KV-state reuse across gates for one step works, CPU vs Vulkan. Note what would be needed for Windows (can't be run here).


## Answer

**Yes.** A 4.4 MB pure-Go binary (yzma v1.28.0, purego, `CGO_ENABLED=0`) + llama.cpp b11146 prebuilt shared libs reproduces judge.py + SemIf direct mode. Prompts and token ids are identical to SemIf's HF reference for all 98 probes. On CPU, 86/88 verdicts agree with regress-out.txt and it scores 68/88 confident+correct (Python: 66/88). Vulkan on the iGPU works out of the box and is about 5× faster. Code is on branch `spike/go-scoring`, dir `spike-go-scoring/`, worktree `/home/sbradl/Projects/tdd-trainer/.claude/worktrees/agent-a90a45fac521ed9ae` (commit 7719a3a). Raw outputs are in `spike-go-scoring/results/`.

| Run (Qwen3.5-4B Q4_K_M, 88 fixtures = 98 probes) | Load | Verdict agreement vs Python | \|Δp\| mean / median / p90 / max | Confident+correct | Total | Warm s/probe by gate |
|---|---:|---:|---|---:|---:|---|
| Python SemIf (regress-out.txt), CPU | — | — | — | 66/88 | 651 s | ~6.6 avg |
| Go yzma, CPU 8 threads (`ngl 0`, as SemIf) | 1.8 s | 86/88 | 0.015 / 0.007 / 0.042 / 0.120 | 68/88 | 696 s | multi 4.0, red-check 6.5, tpp 8.4, step-size 13.3 |
| Go yzma, Vulkan Radeon 860M (`ngl 99`) | 1.1 s | 83/88 | 0.016 / 0.006 / 0.045 / 0.112 | 67/88 | 118 s | multi 0.76, red-check 1.13, tpp 1.33, step-size 2.17 |

The per-probe times are the forward pass over a 160–600-token prompt, with the model warm. A review gate costs two probes.

**Facts**
- **Parity.** The Go code renders the Qwen ChatML template with an empty `<think>` block, matching `apply_chat_template(enable_thinking=False)`. A small encoder writes the user turn exactly like Python `json.dumps(ensure_ascii=False)`. It is not Go's `encoding/json`, which escapes `<>&` and uses compact separators. `llama_tokenize(parse_special=true)` then gives the same ids as the HF tokenizer: 0/98 prompt mismatches and 0/98 token mismatches (checked with `dumpref.py` + `-ref`). The letters A–P are single tokens (ids 32–47). Readout: `llama_get_logits_ith(-1)`, then softmax over the letter slots. Thresholds are 0.8 (step-size 0.45), and a review gate says "no" only when both probes agree.
- **Remaining p drift is numerical.** Python uses llama-cpp-python 0.3.35, compiled locally. The spike uses the official b11146 binaries with runtime CPU-variant dispatch. Every verdict difference is a fixture sitting at the 0.8 line. CPU: cheating.yes (0.778→0.815) and multi.yes.cs-list-and-loop (0.767→0.807) go from uncertain to confident and correct. Vulkan: 5 flips, 3 better and 2 worse (tpp.recursion.ex-sum 0.855→0.743, one-behaviour.no.ex-parse-format-error 0.815→0.788). Go CPU and Go Vulkan also differ from each other by a mean |Δp| of 0.018. Treat p as ±~0.05 across builds and backends. That matters for a fixed 0.8 gate and argues for calibration or a margin band.
- **KV/state reuse across the gates of one step works.** The spike prefills the state prefix once, using SemIf's `_state_prefix`: the prompt up to `{"evidence": <state>`, minus the last token. It saves the state with `llama_state_seq_get_data`, then for each gate does `seq_rm` + `state_seq_set_data` + decode of the suffix. The test was all 14 diff probes of one step (tpp, multi, structural, refactor-effect, and 5 lenses × 2):

  | State | Backend | Fresh direct per probe | Reuse: prefill once + per probe | 14 probes total |
  |---|---|---:|---:|---|
  | ~160 tok (fixture) | CPU | 5.5 s | 3.0 s + 2.6 s | 78 s → 40 s |
  | ~160 tok | Vulkan | 1.16 s | 0.8 s + 0.58 s | 16 s → 9 s |
  | ~1850 tok (diff padded to 6000 chars) | CPU | 38.6 s | 35.7 s + 2.8 s | 541 s → 75 s |
  | ~1850 tok | Vulkan | 6.75 s | 6.25 s + 0.60 s | 94 s → 15 s |

  The suffix (question + options) is about 133 tokens, so on small states it dominates. The per-probe floor is about 2.6 s on CPU and 0.6 s on Vulkan. The saved state is 54–110 MB, because Qwen3.5's recurrent memory is large. Restoring is exact. Reuse still differs from fresh direct by up to 0.17 in p, but decoding the same prefix and suffix as two batches *without* save/restore gives the same drift, so the cause is the batch split (the hybrid linear-attention path is chunk-sensitive), not restore. Argmax flipped in 1/14 probes on CPU and 1/14 on Vulkan. If reuse ships, re-baseline the fixtures in reuse mode.
- **CPU prefill is slow**: about 50 tok/s with 8 threads, which gives 4–8 s per small gate and about 39 s for a 1.9k-token state. Vulkan on the 860M iGPU manages about 300 tok/s. For an interactive Judge on a machine like this one: Vulkan when available, state reuse per step, and small states.
- **Sizes.** Go binary: 4.4 MB, or 2.9 MB stripped (`-s -w`); the Windows `.exe` is 5.0 MB. yzma loads only `libllama`, `libggml` and `libggml-base` (5.6 MB). `ggml_backend_load_all_from_path` then picks one of 14 `libggml-cpu-<arch>.so` variants (17.8 MB for all of them, about 1.2 MB each), plus `libggml-vulkan.so` at 43 MB. The minimal Linux set is 23 MB for CPU (8.4 MB as .tar.gz) and 66 MB with Vulkan (22 MB as .tar.gz). Windows: core DLLs + libomp 4.8 MB, CPU variants 18.4 MB, `ggml-vulkan.dll` 44 MB. The model is a separate 3.0 GB download. Linux runtime deps are only system `libstdc++`, `libgomp`, and `libvulkan.so.1` for Vulkan.
- **yzma notes.** `go run github.com/hybridgroup/yzma@v1.28.0 install --lib DIR -p cpu|vulkan [--os windows]` fetches the official ggml-org release tarball, checked by SHA-256 (release pinned per yzma tag: v1.28.0 ↔ b11146). The C API maps 1:1: `Tokenize`, `BatchInit/Add`, `Decode`, `GetLogitsIth`, `MemoryClear`, `MemorySeqRm`, `StateSeqGet/SetData`. The context was set up like SemIf: `n_ctx` 4160, `n_seq_max` 1, `n_outputs_max` 1, decode chunk 512. Nothing blocked, and llama-server was not needed.

**Windows (not run here)**
- `CGO_ENABLED=0 GOOS=windows go build` works, since purego and ffi also cover Windows. yzma lists Windows amd64 with CPU, Vulkan, CUDA and others.
- Ship `llama.dll`, `ggml.dll`, `ggml-base.dll`, `ggml-cpu-*.dll`, `libomp.dll`, and optionally `ggml-vulkan.dll`, taken from the `win-cpu-x64` / `win-vulkan-x64` release, next to the exe or in a cache dir. Point `llama.Load` at that dir.
- The DLLs import `MSVCP140.dll`, `VCRUNTIME140.dll`, `VCRUNTIME140_1.dll` and the `api-ms-win-crt-*` UCRT libraries. The UCRT ships with Windows 10+, but the VC++ 2015–2022 runtime may be missing on a clean machine. Either bundle those three DLLs app-local, which the redist licence permits, or have the user install the VC++ redist. `vulkan-1.dll` comes with the GPU driver. Without it, loading the Vulkan backend fails and the app should fall back to CPU.
- Still unverified: DLL search path behaviour, which may need `SetDllDirectory` or the libs next to the exe; Defender/SmartScreen on unsigned downloaded DLLs; Windows CPU and Vulkan latency; and whether the Windows build's p values match these.
