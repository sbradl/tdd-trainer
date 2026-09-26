# Judge core (yzma backend)

Status: done
Blocked by: 01
Spec: ../spec.md (§7, §10)

## What

Port the spike (branch spike/go-scoring) into a judge package: lib loading (Vulkan else CPU), model load, ChatML prompt + Python-compatible JSON encoder, option-letter logit readout, thresholds, two-probe review gates, state reuse per step. `tddt judge --regress` over the fixture set (import from prototype-judge-eval/fixtures), re-baselined in reuse mode.

## Done when

`tddt judge --regress` runs on CPU and Vulkan with ≥ the spike's scores (68/88 CPU) and 0 confident-wrong; prompts token-identical to the spike's reference dump.

## Notes from implementation

- `internal/judge`: gates/prompts ported from the spike; prompt text equals the Python reference for all 98 probes (`testdata/ref.jsonl`, unit test), token IDs equal the HF reference (test runs when model + libs are installed).
- State reuse groups gates by identical evidence text (the prefix contains the evidence, so only gates seeing the same parts can share it). Every score uses the prefix/suffix split, also for a single gate, so results don't depend on grouping.
- Fixtures embedded in the binary (`internal/judge/fixtures`); `tddt judge --regress [--cpu] [--gate g]` exits non-zero on any confident-wrong.
- Re-baselined in reuse mode (`testdata/baseline-{cpu,gpu}.txt`): CPU 69/88, GPU (Vulkan iGPU) 69/88, 0 confident-wrong both (spike: 68 CPU / 67 Vulkan). CPU here ~1.8 s/gate.
- Backend: the Vulkan lib build also contains the CPU backends; GPU offload is used when `SupportsGpuOffload()`, `--cpu` forces CPU. One lib folder can serve both → slice 11 may ship only the Vulkan build.
- Lib/model lookup: `$TDDT_LIB` / lib next to exe / `<UserCacheDir>/tddt/lib`; `$TDDT_MODEL` / `<UserCacheDir>/tddt/models/Qwen_Qwen3.5-4B-Q4_K_M.gguf`.
- CI runs `go test -short` (model tests skipped).
