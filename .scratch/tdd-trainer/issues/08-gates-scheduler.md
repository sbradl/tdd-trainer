# Gates and scheduler

Status: done
Blocked by: 06, 07
Spec: ../spec.md (§7)

## What

Map completed steps to exact checks + gates with the right evidence (incl. step-size's Red transcript + pre-Green source); band comparison, multi, missed-refactor via review lenses; background worker with priority (red-check → Green → Refactor → lenses); verdict model (gate, level ok/hint/warn/uncertain, text, p).

## Done when

Replaying recorded kata sessions yields the expected verdicts; UI thread never blocks on the judge.

## Notes from implementation

- `internal/coach`: exact checks emitted synchronously in `Step`, judge jobs in a priority heap (red-check → Red → Green → Refactor → lenses, FIFO within), one worker; nothing dropped.
- Evidence mirrors the fixtures: Test = added test lines of the step, Test-runner output = the new tests' failure messages, Current source = source files at the Red's end, Source diff = source patches. Refactor of tests only → test diff judged.
- step-size compares its band with the tpp answer's band (constant 1–2, simple 3–4, complex 5–8); either uncertain → uncertain.
- Missed refactor = one verdict combining the 5 lenses over the previous Green's diff.
- Judge loads in the background (`judge.Lazy`); without libs/model the coach runs exact checks only and says so.
- Replay test drives a real Go kata through snapshot store + step machine + coach with a scripted scorer and checks verdict order, levels and evidence; a blocked judge never blocks `Step`.
- Live run on a Go kata with the real model works (GPU); noisy lens hint on a trivial `return "I"` → slice 12.
