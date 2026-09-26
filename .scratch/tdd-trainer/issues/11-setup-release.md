# Setup and release packaging

Status: done (Windows unverified)
Blocked by: 07
Spec: ../spec.md (§10)

## What

`tddt setup`: model download with SHA-256 + progress into user cache dir, offline path flag/env, lib fetch for go-install users. Release pipeline: per-OS archives with pinned llama.cpp libs, Windows VC++ runtime DLLs app-local; CI regress on CPU; Windows build smoke test; README incl. SmartScreen note.

## Done when

Fresh Linux and Windows machines go from download to a running session with `tddt setup` only; Vulkan falls back to CPU when unavailable.

## Notes from implementation

- `tddt setup [--model-file F] [--lib-dir D] [--skip-model] [--cpu]`: installs llama.cpp b11146 **Vulkan** build (contains the CPU backends) unless libs are found, downloads the model (resumable, progress, SHA-256 pinned: bartowski/Qwen_Qwen3.5-4B-GGUF @ 4168f45, 13c16f42…), links/copies an offline model file into the cache, then self-checks one fixture and prints the backend. Own downloader (stdlib), not yzma's (would add ~70 packages).
- Libs trimmed to what libllama needs (tool `*-impl`, llama-common, mtmd, rpc skipped): Linux 64 MB. Runtime deps: libgomp, libstdc++, (libvulkan for GPU).
- Verified on Linux with an empty cache: setup → GPU self-check; CPU fallback verified with no Vulkan driver and with ggml-vulkan missing (5.2 s/gate).
- Lib lookup order: `$TDDT_LIB`, `<exe>/lib`, `<exe dir>` (Windows archive is flat), `<cache>/tddt/lib`. On Windows the lib dir is prepended to PATH before loading (dependent DLLs).
- Release workflow (`.github/workflows/release.yml`, tags `v*`): Linux tar.gz (tddt + lib/), Windows zip (flat, + msvcp140/vcruntime140/vcruntime140_1 from the runner), SHA256SUMS, GitHub release. CI: `windows-smoke` (tests + `--once` on a kata) and `regress` (CPU, model cached).
- **Not verified**: anything on real Windows (DLL search, VC++ runtime, latency, p values), the GitHub workflows themselves (no remote yet). No project LICENSE file exists; archives ship llama.cpp's LICENSE only.
- README written (install, setup, offline, SmartScreen, keys, config).
