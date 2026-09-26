# SemIf feasibility as the local Judge

Research for [issue 01](../issues/01-semif-feasibility.md). Retrieved 2026-09-26. SemIf commit `23cf1f3` (2026-09-24), cloned from https://github.com/TheoLeeCJ/SemIf. Timings marked **measured** were taken on the user's laptop (Ryzen AI 7 350, 16 threads, 30 GB RAM, Radeon 860M iGPU, Manjaro) using llama.cpp release `b11200`. They may have competed for CPU with the user's own `--regress` run (load average about 3 at the time), so read them as upper bounds.

## TL;DR

- **SemIf is a method plus a Python research harness. It is not a runtime to embed.** The method: build a chat prompt that lists the options as letters A–P, run one forward pass, read the logits of the letter tokens at the last position, and softmax over them. No text is generated ([docs/METHOD.md](https://github.com/TheoLeeCJ/SemIf/blob/main/docs/METHOD.md), [src/semif_phase1/core.py](https://github.com/TheoLeeCJ/SemIf/blob/main/src/semif_phase1/core.py)).
- **It works without CUDA.** The `llamacpp` backend scores GGUF files on CPU (`n_gpu_layers = 0`) ([llamacpp_backend.py](https://github.com/TheoLeeCJ/SemIf/blob/main/src/semif_phase1/llamacpp_backend.py)). A WebGPU browser demo (wllama) also exists. SemIf never mentions Windows, but its CPU path is plain llama-cpp-python, which ships a Windows CPU wheel.
- **For a Go single binary, reimplement the method and do not ship SemIf.** llama.cpp exposes everything needed: full logits over its C API, and top-N logprobs over `llama-server` HTTP. The Python stack adds about 1 GB of venv (711 MB of it is torch, which the llamacpp path never imports) and a C toolchain on Linux.
- **Model choice decides the result.** SemIf's own quality ladder gives Qwen3.5-4B a balanced accuracy of 0.81, MiniCPM5-2B 0.69, and Qwen3-0.6B 0.44, which is below chance for its task mix. Only the 4B is credible as a judge. On CPU it is slow: **measured** about 5.5 s for a roughly 370-token step and about 28 s for a roughly 1.9k-token step. With the Vulkan iGPU build this drops to about 1.2 s and about 6 s.

## 1. Platforms and backends (primary: repo)

| Backend | Hardware | OS evidence | Source |
|---|---|---|---|
| torch (default) | CUDA with 1 GPU; CPU float32 "much slower" reference | Linux (dev box has an RTX 3090) | README Quick start; `core.resolve_device` |
| `--backend llamacpp` | CPU only; GGUF via `llama-cpp-python==0.3.35`; `--llama-threads` | Linux documented. Windows: PyPI ships only an sdist (75 MB, needs a compiler), but abetlen's index has `llama_cpp_python-0.3.35-py3-none-win_amd64.whl` (CPU) | README; [pyproject.toml](https://github.com/TheoLeeCJ/SemIf/blob/main/pyproject.toml); https://pypi.org/project/llama-cpp-python/0.3.35/; https://abetlen.github.io/llama-cpp-python/whl/cpu/llama-cpp-python/ |
| MLX / MPS | Apple Silicon | macOS only | [docs/MLX.md](https://github.com/TheoLeeCJ/SemIf/blob/main/docs/MLX.md), [docs/APPLE_SILICON.md](https://github.com/TheoLeeCJ/SemIf/blob/main/docs/APPLE_SILICON.md) |
| WebGPU demo | Browser, wllama 3.6.1 (llama.cpp to WASM/WebGPU), `n_ctx 2048` | any WebGPU browser | [webgpu-demo/README.md](https://github.com/TheoLeeCJ/SemIf/blob/main/webgpu-demo/README.md), `worker.js` |
| EXL3 bridge (27B) | CUDA | — | `exl3-bridge/` |

Notes on the llamacpp backend:
- **Tokenizer.** The prompt is built with the HF `transformers` tokenizer and its chat template, with `enable_thinking=False`. Every prompt is then re-tokenized through the GGUF vocabulary, and the backend **fails if the two tokenizations differ**. It also checks that the letters A–P are single tokens that are identical in both ([llamacpp_backend.py `_verify_vocabulary`, `encode_verified`](https://github.com/TheoLeeCJ/SemIf/blob/main/src/semif_phase1/llamacpp_backend.py)). This means llama.cpp's own tokenizer is sufficient; the reference tokenizer is only there for auditing.
- **Readout.** `llama_get_logits_ith(ctx, -1)` returns the full vocabulary logits. The backend keeps only the letter-slot logits and softmaxes over them. It also reports `allowed_token_mass`, the probability mass the model put on the allowed letters, which is useful as a "model is confused" signal.
- **Shared state across criteria.** Qwen3.5 uses a hybrid linear-attention memory that "supports neither sequence copies nor partial tail removal". SemIf therefore saves the state after prefill once (`llama_state_seq_get_data`) and restores it for each criterion (`llama_state_seq_set_data`). This is the path to use when one Step is judged on several criteria.
- **torch.** Every `import torch` in `src/` is inside a function on the torch, shared, or reranker path. The llamacpp path imports only `transformers` for the tokenizer. **Measured:** a venv with only `transformers==5.17.0 tokenizers jinja2` (156 MB) loads the pinned Qwen3.5-4B tokenizer and chat template with `torch` never imported. torch is required only because `pyproject.toml` lists it as a hard dependency.

## 2. Models, sizes, quality (primary: repo manifests and results)

| Model (GGUF used by SemIf) | File | SemIf authored balanced acc. | Perturbation BA | TypeSafe agreement |
|---|---:|---:|---:|---:|
| Qwen3-0.6B Q8_0 | 639 MB | 0.440 | 0.528 | 0.407 |
| MiniCPM5-2B Q4_K_M | 1.56 GB | 0.686 | 0.693 | 0.637 |
| Qwen3.5-4B Q4_K_M (bartowski) | 3.01 GB | 0.813 | 0.766 | 0.845 |
| Qwen3.8-27B EXL3 5 bpw (CUDA) | — | 0.958 | — | — |

Source: README "Browser model ladder" and [manifests/models.json](https://github.com/TheoLeeCJ/SemIf/blob/main/manifests/models.json). The quality numbers come from **BF16** weights; SemIf does not report quality for the quantized GGUFs ("does not establish full quantized quality"). Calibration by temperature scaling helps ([docs/CALIBRATION.md](https://github.com/TheoLeeCJ/SemIf/blob/main/docs/CALIBRATION.md)). Project code is MIT; model licences come from upstream.

## 3. Latency

**Published by SemIf** (none of it on CPU):
- RTX 3090, BF16 4B: 21 binary criteria in 1.02 s direct. Shared-state throughput is 2.3 decisions/s fresh, 10.8 with serial prefix reuse, and 20.0 with parallel suffixes, on states of about 8k characters.
- Browser, WebGPU, RTX 3090, direct readout of one decision: 0.70 s (0.6B), 1.51 s (2B), 3.27 s (4B). Load time 9–21 s from a local SSD ([results/raw/browser-model-ladder.json](https://github.com/TheoLeeCJ/SemIf/blob/main/results/raw/browser-model-ladder.json)).
- **No CPU latency is published.**

**Measured on this laptop.** Setup: `llama-server` b11200, one decision per request, `/v1/chat/completions` with `max_tokens:1`, `logprobs`, `top_logprobs:20`, and thinking off. Prompts were a Go FizzBuzz red step (test, runner output and diff): "small" is about 350 tokens and "big" is about 1.9k tokens (the same step plus 6.5 kB of extra source). Model load took 1–2.5 s from warm page cache.

| Model | Backend | small, cold | small, same-state next criterion | big, cold | big, next criterion | RSS |
|---|---|---:|---:|---:|---:|---:|
| Qwen3-0.6B Q8 | CPU 8 threads | 0.7 s | 0.2 s | 4.5 s | 0.46 s | 1.6 GB |
| Qwen3.5-4B Q4_K_M | CPU 8 threads | **5.4 s** | 5.1 s (no reuse) | **28 s** | 7.5 s (partial reuse) | 4.6–4.8 GB |
| Qwen3-0.6B Q8 | Vulkan (Radeon 860M iGPU) | 0.15 s | 0.05 s | 0.95 s | 0.07 s | — |
| Qwen3.5-4B Q4_K_M | Vulkan iGPU | 1.25 s | 1.15 s | 6.1 s | 1.9 s | — |

- The 4B model on CPU prefills at only about 68 tok/s, versus about 500 tok/s for the 0.6B model. On the iGPU the 4B reaches about 300 tok/s.
- `llama-server`'s `cache_prompt` barely helps Qwen3.5 because of the hybrid memory described above. SemIf's save/restore of sequence state avoids re-prefilling, but that is only reachable through the C API, not over HTTP.
- User's existing setup (`~/.local/share/semif`, [tdd-judge SKILL.md](/home/sbradl/Projects/claude-skills/.claude/skills/tdd-judge/SKILL.md)): "about 4 s per yes/no gate, 8 s for `tpp`". That fits the small-prompt CPU row. Its parts are clipped to 6000 characters each (`MAX_PART_CHARS`). Accuracy and timing from `--regress` over its 41 fixtures are pending; the user is running it.

Takeaway: with the 4B model on CPU, a verdict arrives about 5–30 s after a Step. That is acceptable only if judging runs asynchronously, keeps the state small (diff plus failing assertion, not whole files), and shows a heuristic verdict first. A GPU through Vulkan cuts latency 4–5×, and llama.cpp publishes Vulkan builds for both Linux and Windows.

## 4. Driving it from Go

| Option | How | Pros | Cons |
|---|---|---|---|
| **A. Spawn `llama-server`** | Go downloads or extracts the official release zip (CPU: `ubuntu-x64` 17 MB, `win-cpu-x64` 19 MB; Vulkan: 31/33 MB) and runs `llama-server -m model.gguf --port N`. Requests go to `/v1/chat/completions` with `max_tokens:1, logprobs:true, top_logprobs:K`, or to `/completion` with `n_probs`. Both apply the GGUF's own chat template. **Measured working**; the top logprobs contained all option letters. | Pure Go, no cgo. Official binaries for both OSes. GPU backends come for free. | Only top-K logprobs, so an option letter can be missing (treat missing as ≤ the smallest shown logprob, or add a grammar or `logit_bias`). No sequence-state save/restore for Qwen3.5 over HTTP (slot save/restore exists but is per slot). One extra process to manage. |
| **B. yzma (purego, no cgo)** | https://github.com/hybridgroup/yzma: "uses the purego and ffi packages so CGo is not needed". Linux, macOS and Windows. Downloads llama.cpp prebuilt shared libraries. Exposes `Decode`, `GetLogitsIth`, `Tokenize`, `StateSeqGetData/SetData`, `MemoryClear` ([pkg.go.dev](https://pkg.go.dev/github.com/hybridgroup/yzma/pkg/llama)). | Gives exactly SemIf's readout: full logits and state save/restore for multi-criterion Steps. Porting `llamacpp_backend.py` is about 150 lines. | Libraries must match llama.cpp versions. Chat template must be rendered in Go (Qwen ChatML with the empty `<think></think>` block is trivial). Younger project. |
| C. cgo bindings (e.g. go-skynet/go-llama.cpp) | Static link | One file | cgo breaks easy cross-compiling to Windows; maintenance unverified. |
| D. Ollama as a dependency | `/api/chat` with `logprobs:true, top_logprobs≤20, options.num_predict:1, think:false` (since v0.12.11). **Measured working** (v0.33.3, 0.6B). | Many learners already have it. | An extra install for everyone else. Top-20 cap: a PR to request specific tokens was closed on 2026-09-22 in favour of raising the cap ([ollama#18580](https://github.com/ollama/ollama/pull/18580)). No state reuse control. |
| E. Keep SemIf Python | Go shells out to `semif-score` (as the user's `judge.py` does) | Reuses the fixtures and prompts as they are | Setup needs uv, Python 3.12, and cmake plus a C compiler (Linux), about 1 GB of venv (torch 711 MB unused), and the HF tokenizer cached online once. This is not "very easy setup". |

Bundling sketch for option A or B:
- A single Go binary, with the llama.cpp CPU or Vulkan libraries or `llama-server` either embedded (about 20–35 MB per OS) or fetched on first run, checked by SHA256.
- The GGUF (3.0 GB for the 4B) fetched on first run from a pinned HF revision URL into the user cache dir.
- The Judge picks Vulkan when a device is present and falls back to CPU otherwise.
- Tokenizer parity is not a problem: SemIf itself asserts that GGUF tokenization equals the HF tokenizer's for Qwen3.5-4B.

## 5. Alternatives offering the same "score declared options" style

- **llama.cpp directly** (C API or `llama-server` `n_probs`/`logprobs`, `logit_bias`, `grammar`): https://github.com/ggml-org/llama.cpp/blob/master/tools/server/README.md. This is the substrate SemIf's CPU path already uses.
- **Ollama logprobs** (v0.12.11+): https://docs.ollama.com/api/generate.
- **wllama** (llama.cpp in the browser) is what SemIf's WebGPU demo uses. It is not relevant for a terminal UI.
- **Reranker readout** (Qwen3-Reranker-4B, yes/no log-odds per option). SemIf found it worse for general decisions (authored BA 0.625 against 0.813) and CUDA-only in its harness.

## Open points for later tickets

- Quality of the quantized GGUF on TDD-specific criteria is unknown. See the user's `--regress` results and [07-judge-eval](../issues/07-judge-eval.md).
- MiniCPM5-2B was not measured on CPU. It might be the latency/quality middle ground on machines without a GPU.
- Windows latency was not measured. The same llama.cpp CPU and Vulkan builds exist for it.
