# Stack and distribution

Type: grilling
Status: resolved
Blocked by: 01, 07, 14
Map: ../map.md

## Question

What is the implementation stack and how is it shipped to Linux and Windows with near-zero setup? Go single binary vs Python vs hybrid; how the local model and runtime are obtained (bundled, first-run download); how the optional cloud judge is configured.

## Answer

- **Stack**: Go single binary using yzma (purego llama.cpp bindings), `CGO_ENABLED=0`, cross-compiled for Linux and Windows (amd64). Proven by [Go scoring spike](14-go-scoring-spike.md).
- **Delivery**: the release archive per OS bundles the llama.cpp shared libs (pinned to the yzma tag, e.g. v1.28.0 ↔ b11146; CPU variants + Vulkan backend). Only the model (Qwen3.5-4B Q4_K_M GGUF, 3.0 GB, Apache-2.0) is downloaded, by `tddt setup` into the user cache dir, SHA-256 checked, with progress; a flag or env var points at an existing file for offline installs.
- **Backend**: Vulkan automatically when it loads, otherwise CPU. Reuse the model state across the gates of one step (prefill the shared part of the prompt once).
- **Thresholds**: keep 0.8 (step-size 0.45). CI runs the judge fixtures on CPU and Vulkan, with the fixtures re-baselined in state-reuse mode. Calibrating per backend waits until drift proves a problem.
- **Windows**: ship `MSVCP140.dll`, `VCRUNTIME140.dll` and `VCRUNTIME140_1.dll` next to the exe (the redist licence permits this); `vulkan-1.dll` comes with the GPU driver, with CPU as fallback. No code signing in v1; the docs explain the SmartScreen warning.
- **Channels**: GitHub releases (Linux + Windows archives) and `go install`. `go install` users get the libs via `tddt setup` too. Package managers (scoop, winget, AUR, Homebrew) are out of scope for v1.
- Still unverified until implementation: Windows DLL search path, Windows latency, and whether Windows p values match Linux.
