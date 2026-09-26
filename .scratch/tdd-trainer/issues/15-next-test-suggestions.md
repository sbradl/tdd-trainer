# Next-test suggestions: a concrete test, generated and ranked

Status: idea
Blocked by: 14
Spec: ../spec.md

## What

A 3rd reveal stage for `n`: a concrete next test, e.g. `Roman(4) == "IV"`. Generate candidates, then rank them with scoring gates.

- **Engine:** add sampling to `judge.Engine` (llama.cpp sampler via yzma). Short, capped output (~40 tokens per candidate) that reuses the cached prompt prefix. Scoring stays as it is.
- **Constrained by #14:** the prompt includes the category and case #14 found ("empty input", "boundary"). Candidates stay on topic and the 4B model has less to invent.
- **Generate** 3–5 candidates as one-line test sketches (inputs → expected), not full test code. No language syntax to get wrong.
- **Rank** each candidate with gates:
  - "Would this test fail against the current source?" (yes required);
  - "Which transformation would making it pass need?" (`TPPOptions`): pick the smallest;
  - duplicate of an existing test? (exact where possible, else a gate).
- **Show** only the top candidate, with its reason: "fails now, needs Unconditional → Selection".
- **Opt-in:** `--suggest` flag or config key, since it's slower and less reliable than #14.
- **Latency:** runs in the background after the 2nd `n`, so the 3rd is instant; the dashboard shows it as pending.

## Done when

- Kata fixtures: given a kata state, the top candidate fails now and needs the smallest possible transformation in ≥80% of fixtures, and is never an existing test.
- GPU ≤3s, CPU ≤15s for the whole suggestion, measured in `tddt judge --regress`.
- No confident candidate → stage 3 says so and keeps the #14 hint.

## Open

- Scoring candidates against the source: the "would it fail" gate may be weak without running anything. Could generate real test code and run it in a snapshot copy instead (exact, but language-specific and slow).
- Is 4B enough for generation, or does this need a bigger optional model?
