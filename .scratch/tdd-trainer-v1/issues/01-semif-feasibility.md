# SemIf feasibility

Type: research
Status: resolved
Blocked by: none
Map: ../map.md

## Question

Can SemIf (https://github.com/TheoLeeCJ/SemIf) run on Linux and Windows without a CUDA GPU (CPU, GGUF/llama.cpp, WebGPU)? What are install footprint, model sizes, and per-judgement latency on a typical laptop? Can a Go binary drive it (subprocess, HTTP, embedded runtime), and what would bundling look like? What alternatives offer the same 'score declared options' style locally (e.g. llama.cpp logit access, Ollama logprobs)?

## Answer

Gist: SemIf is a method plus a Python research harness, not a runtime to embed. For the Go single binary, reimplement the method on llama.cpp and don't ship SemIf's Python.

- The method: list the options as letters, run one forward pass, and softmax the letter-token logits. No generation.
- CPU without CUDA works: the `llamacpp` backend runs GGUF on CPU, and a WebGPU browser demo also exists. SemIf never mentions Windows, but llama-cpp-python has a win_amd64 CPU wheel, and official llama.cpp CPU and Vulkan builds exist for Linux and Windows.
- Install footprint of the Python path: about 1 GB of venv (torch 711 MB, which the llamacpp path never imports; a tokenizer-only venv of 156 MB was verified) plus cmake and a C compiler on Linux. The llama.cpp route needs 17–33 MB of binaries.
- Model sizes: 0.6B Q8 is 639 MB, MiniCPM5-2B Q4 is 1.56 GB, Qwen3.5-4B Q4_K_M is 3.01 GB. SemIf's balanced accuracy for them is 0.44 / 0.69 / 0.81, so only the 4B is judge-grade.
- Latency, measured on this laptop's CPU with the 4B: about 5.4 s for a step of about 350 tokens and about 28 s for about 1.9k tokens. With the Vulkan iGPU: 1.2 s and 6 s. So judge asynchronously and keep the state small.
- Go options:
  - Spawn `llama-server` and use `logprobs`/`top_logprobs` (measured working).
  - yzma (purego, no cgo) for full logits plus the sequence-state save/restore that Qwen3.5 needs to reuse the state across several criteria.
  - Ollama logprobs, capped at the top 20 tokens.
- Accuracy (user's `claude-skills/tdd-judge --regress`, 41 fixtures, Qwen3.5-4B Q4 CPU, Ryzen AI 7 350, 2026-09-26): 33/41 confident and correct. 7 abstained (`uncertain`): cheating.yes, red-check notimpl, tpp scalar, tpp go-new-file, structural.yes, review-ddd, review-philosophy. 1 confident wrong: step-size word-frequency (complex→simple, p=0.52; that gate uses a 0.45 threshold). No confident wrong answers at the 0.8 threshold. 402 s wall clock for the whole run, about 10 s per case, including model load and two-probe review gates.

Details: [../research/semif-feasibility.md](../research/semif-feasibility.md)
