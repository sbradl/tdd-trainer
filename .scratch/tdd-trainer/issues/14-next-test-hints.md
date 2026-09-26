# Next-test hints: which kind of test to write next

Status: idea
Blocked by: —
Spec: ../spec.md

## What

On demand (key `n`, "what next?"), tell the learner which kind of test to write next. Scoring only, no text generation: gates pick a category, and the hint text is static per category.

- **Coverage gates (ZOMBIES):** one yes/no gate per case: Zero/empty, One, Many, Boundaries, Interface, Exceptions/errors. Question: "Does any test cover the <case> case?" Evidence: `PartTest` + `PartSource`. The first uncovered case in ZOMBIES order becomes the hint. Two-probe `Clean` pattern as in the review lenses, so "covered" needs both probes to agree.
- **Next-transformation gate:** "Which is the smallest transformation a new test could force in the current source?" over `TPPOptions`. Used to phrase the hint ("a test that needs one new condition").
- **Exact signals, no model:**
  - last Green was Nil → Constant (fake-it) → "Triangulate: add a second example that forces you to generalise";
  - last Green applied several transformations or `step-size` was complex → "Next time pick a test closer to the current code".
- **Staged reveal:** 1st `n`: category ("an edge case"); 2nd `n`: the case ("empty input"). #15 adds a 3rd stage.
- **When:** only on request. Suggested moment: all green, no open *Refactor now*. During Red/Green, `n` says to finish the current step first.
- **Report:** hint requests per cycle and category ("asked 4×, mostly boundaries") as a focus tip.
- **Plain output:** `tddt next [dir]` prints the same hint for the last session state.

## Done when

- The gates have labelled fixtures in at least 3 languages (take sequences from the roman, fizzbuzz and bowling katas) and pass `tddt judge --regress` with 0 confidently wrong.
- `n` works in both views and in `tddt next`. Staged reveal and the report entry are covered by tests.
- An uncertain verdict never becomes a hint. Then fall back to the exact signals, or to "No clear gap. Is there a case you haven't tried?"

## Open

- ZOMBIES order vs. TPP order when they disagree (e.g. "many" is uncovered but the smallest transformation is a boundary).
- "Interface" doesn't fit well as a coverage question; maybe drop it.
- Recognising "done": all cases covered → "Looks complete. Refactor or stop."
