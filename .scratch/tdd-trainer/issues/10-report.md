# Session report and tddt show

Status: done
Blocked by: 08
Spec: ../spec.md (§9)

## What

Markdown report in .tddtrainer/reports/ on quit and on demand; terminal summary; `tddt show <step>` prints the step's full diff and verdicts.

## Done when

Report contains every section in spec §9; uncertain verdicts appear only there.

## Notes from implementation

- `internal/report`: Markdown with header table (duration, cycles, tests added, clean share), focus tips (top 3 hints/warnings), per-cycle tables, transformation path, anomalies + missed refactors, uncertain section (the only place uncertain verdicts appear), diff snippets only for flagged steps (missed refactor → the Green before it). Tests check every section.
- Written on quit (TUI q/Ctrl-C and plain-mode Ctrl-C) and on `w`; one file per session `.tddtrainer/reports/<start>.md`, rewritten on each write; terminal summary on quit; pending gates at quit are noted.
- `tddt show STEP [dir]` reads the newest session file `.tddtrainer/sessions/<start>.json` (saved atomically after each step/judge verdict) and prints verdicts incl. uncertain + the full diff from the shadow repo.
