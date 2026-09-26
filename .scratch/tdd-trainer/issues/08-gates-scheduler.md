# Gates and scheduler

Status: ready-for-agent
Blocked by: 06, 07
Spec: ../spec.md (§7)

## What

Map completed steps to exact checks + gates with the right evidence (incl. step-size's Red transcript + pre-Green source); band comparison, multi, missed-refactor via review lenses; background worker with priority (red-check → Green → Refactor → lenses); verdict model (gate, level ok/hint/warn/uncertain, text, p).

## Done when

Replaying recorded kata sessions yields the expected verdicts; UI thread never blocks on the judge.
