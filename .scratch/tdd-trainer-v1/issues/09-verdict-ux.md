# Verdict and terminal UX

Type: prototype
Status: resolved
Blocked by: none
Map: ../map.md

## Question

What does the coach look like live in a terminal split pane? How are steps, verdicts (arriving in the background, ~7 s per gate), hints, anomalies and phase-override hotkeys shown? How noisy is it? WatchDog found strict TDD in only 2.2% of real sessions (see [Prior art](02-prior-art.md)), so the anomaly verdicts from [Step inference model](04-step-inference.md) risk firing on almost every step. Decide the noise policy: what's shown immediately, what's batched, what's saved for the report.

## Prototype

Ready to react to (built while the user was away; not resolved, needs the user's reaction): branch `prototype/verdict-ux`, `cd prototype-verdict-ux && go run .`. Three layouts: A Timeline, B Dashboard, C Quiet coach (see its README).

## Answer

- **Default layout: C · Quiet coach.** One status line (phase, test summary, count of OK verdicts, "judging N" spinner). Only hints and warnings, anomalies included, become cards; a closed Red→Green(→Refactor) cycle collapses into one summary line.
- **Alternative view: B · Dashboard**, toggled by a key (proposed: Tab). It has the phase banner, the cycle strip, the current step's gate table with pending gates and their ETA, and the hotkey bar. Layout A (Timeline) is dropped.
- **Noise policy**: hints and warnings are shown as soon as they arrive; OK verdicts only as a counter; `uncertain` only in the session report (and greyed in the B view). No bell or desktop notification: the pane only.
- Phase-override hotkeys `r`/`g`/`f`/`b` as in [Step inference model](04-step-inference.md); a short toast confirms them.
- Prototype: branch `prototype/verdict-ux`, `prototype-verdict-ux/`.
