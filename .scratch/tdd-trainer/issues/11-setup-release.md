# Setup and release packaging

Status: ready-for-agent
Blocked by: 07
Spec: ../spec.md (§10)

## What

`tddt setup`: model download with SHA-256 + progress into user cache dir, offline path flag/env, lib fetch for go-install users. Release pipeline: per-OS archives with pinned llama.cpp libs, Windows VC++ runtime DLLs app-local; CI regress on CPU; Windows build smoke test; README incl. SmartScreen note.

## Done when

Fresh Linux and Windows machines go from download to a running session with `tddt setup` only; Vulkan falls back to CPU when unavailable.
