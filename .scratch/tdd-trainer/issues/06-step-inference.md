# Step inference engine

Status: done
Blocked by: 02, 05
Spec: ../spec.md (§6)

## What

Pure state machine over (Test state, snapshot) events → Steps (Red, Red in progress, Green, Refactor), Anomalies, Baseline, rename detection, override API (r/g/f/b). Table-driven tests for every transition and anomaly in the spec.

## Done when

All transitions and anomalies in spec §6 covered by tests; replaying the verdict-ux prototype's script yields the same step sequence.

## Notes from implementation

- `internal/steps.Machine`: pure; phases Refactor (green) / Red in progress / Green (making it pass). Table tests for every §6 transition + anomaly; replay of the verdict-ux script gives the same sequence.
- `TestState.Tests` (all reported IDs) added so new vs previously passing failing tests can be told apart; Go parents with failing subtests stay known.
- Decisions made while implementing (spec was silent):
  - An existing test going red while *making a test pass* is also reported (once per Green) as an anomaly — matches the prototype's last step. Compile errors (build broken) during Green are not.
  - Build broken while refactoring, fixed without new tests → still Refactor, no anomaly.
  - A session that starts red and then goes green judges nothing (began mid-cycle); if its failing test later fails on an assertion it becomes a Red.
  - Test-file change while green without a new test → (test) Refactor, per spec.
  - Refactor step is emitted when the next Red completes and spans to the last all-green run.
- Overrides/baseline reset exist as API (`Override`, `ResetBaseline`); hotkeys come with the TUI (slice 09).
