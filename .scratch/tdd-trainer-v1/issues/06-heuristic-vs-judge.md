# Heuristic vs judge split

Type: grilling
Status: resolved
Blocked by: 03
Map: ../map.md

## Question

For each criterion (test small enough, fails for the right reason, simplest code / higher TPP transformation chosen, refactoring improves code, tests stayed green during refactor), which parts are decided deterministically by heuristics and which need the judge? What input does the judge get per criterion?

See [Prior art](02-prior-art.md): TDD Guard/Probity rule prompts (MIT), Probity AST new-test count; and [TPP research](03-tpp-list.md).

## Answer

Principle: exact checks first, the judge only where they can't decide. Gates run in the background; an `uncertain` verdict stays silent live and appears only in the session report.

| Criterion | Exact checks | Judge gates (input) |
|---|---|---|
| Test small enough | exactly 1 new test ID (from comparing runs); added test lines ≤ limit (default ~15). No assertion counting. | new `one-behaviour` gate (test) |
| Fails for right reason | build command → build broken; JUnit `<error>` vs `<failure>` (see runner contract) | `red-check` (test, runner output) for leftovers only |
| Simplest code | none | `tpp` (diff); `step-size` (test, runner output, source) — the transformation used falling in a higher band than predicted → "a simpler change would have done"; 2+ transformations in one diff → "a test is probably missing"; `cheating` (test, diff) |
| Refactor improves | tests stayed green; no new test IDs; renames allowed | `structural` (diff); new `refactor-effect` gate (diff): improves / neutral / worsens |
| Missed refactor | Green step followed directly by a new Red | review-lens gates (diff/source of the Green step); if they find something → hint "consider refactoring first" |
| Stayed green during refactor | fully exact (test state) | — |

- **TPP list: the newest (Clean Craftsmanship 2021), 8 items**: {}→Nil, Nil→Constant, Constant→Variable, Unconditional→Selection, Value→List, Selection→Iteration, Statement→Recursion, Value→Mutated Value. Recursion vs iteration order configurable per preset (e.g. Elixir recursion first; Go/.NET iteration first).
- **TPP detection**: judge only in v1. Syntax-tree rules (tree-sitter) are out of scope for v1 (cgo, per-language rules).
- **Starting point**: gate questions, options and fixtures from the user's `claude-skills/.claude/skills/tdd-judge` (33/41 confident and correct). Consequence of the 8-item list: the `tpp` options (currently 12 ids) and `step-size` bands must be remapped, and the `tpp` fixtures relabelled (e.g. `constant-plus` → Nil→Constant). Proposed bands: constant = items 1–2, simple = 3–4, complex = 5–8.
- New gates to write and add fixtures for: `one-behaviour`, `refactor-effect`.
