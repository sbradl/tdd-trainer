# Session report

Type: grilling
Status: resolved
Blocked by: 09
Map: ../map.md

## Question

What does the end-of-session report contain (steps, verdicts incl. silent 'uncertain' ones, anomalies, missed refactors, the TPP path taken), in what format (terminal, Markdown, HTML), and where is it saved? Is there a history across sessions?

## Answer

- **Contents**:
  - header: duration, number of cycles, tests added, share of clean cycles
  - one table per cycle: Red / Green / Refactor steps with their verdicts
  - the order of transformations used
  - anomalies and missed refactors
  - a section listing `uncertain` verdicts
  - the 3 most frequent hints, as focus tips

  No score (out of scope).
- **Format**: a Markdown file, plus a short terminal summary on quit. HTML later if wanted.
- **When**: written on quit (`q` / Ctrl-C) and on demand via a key. A session = one run of the coach.
- **Where**: `.tddtrainer/reports/<timestamp>.md` in the project. Diff snippets only for steps that got a hint or warning; full diffs via `tddt show <step>` from the shadow repo. `.tddtrainer/` holds its own `.gitignore` containing `*`, so the learner's repo ignores it without the learner's files being touched.
- **History**: reports are kept per project; no comparison across sessions or trends in v1.
