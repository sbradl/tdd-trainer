# Judge core (yzma backend)

Status: ready-for-agent
Blocked by: 01
Spec: ../spec.md (§7, §10)

## What

Port the spike (branch spike/go-scoring) into a judge package: lib loading (Vulkan else CPU), model load, ChatML prompt + Python-compatible JSON encoder, option-letter logit readout, thresholds, two-probe review gates, state reuse per step. `tddt judge --regress` over the fixture set (import from prototype-judge-eval/fixtures), re-baselined in reuse mode.

## Done when

`tddt judge --regress` runs on CPU and Vulkan with ≥ the spike's scores (68/88 CPU) and 0 confident-wrong; prompts token-identical to the spike's reference dump.
