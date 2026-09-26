# Step inference engine

Status: ready-for-agent
Blocked by: 02, 05
Spec: ../spec.md (§6)

## What

Pure state machine over (Test state, snapshot) events → Steps (Red, Red in progress, Green, Refactor), Anomalies, Baseline, rename detection, override API (r/g/f/b). Table-driven tests for every transition and anomaly in the spec.

## Done when

All transitions and anomalies in spec §6 covered by tests; replaying the verdict-ux prototype's script yields the same step sequence.
