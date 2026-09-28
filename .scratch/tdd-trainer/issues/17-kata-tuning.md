# Tune on longer katas

Status: done
Blocked by: 12

## What

Run scripted sessions of common, longer katas through `tddt` (plain output, real judge), with typical learner mistakes, and turn the misses into fixtures and fixes.

## Notes from implementation

Sessions (18-20 saves each): Go bowling, Python Game of Life, C# Mars Rover, Elixir word wrap, Python tennis, Go prime factors (table-driven). Evidence was dumped per judge job with a temporary hook, so the new fixtures hold exactly what the gates saw live.

Fixed in code:

- Empty kata folder (only `go.mod`, no tests): `go vet` fails, so the baseline started as Red in progress and the first cycle was never judged. A baseline without test files is now green.
- pytest `NameError`/`ImportError`/`NotImplementedError` and ExUnit `UndefinedFunctionError` were undecided (no JUnit type), so a typo'd test completed the Red, got a wrong-reason warning, and fixing the typo counted as "test edited in Green". The JUnit reader now types them from the failure text. Other exceptions (a crash in the code under test) stay undecided: classifying them as wrong made the next Green a "passed without failing" anomaly.
- Test size counted package/import/blank lines (first table-driven Go test: 23 lines).

Tuned (lab, CPU + GPU regress, no confident-wrong):

- cheating: exact-value examples in the question: cs-rover-rr and ts-roman-table up.
- cheating, from a live session (`if n == 4 { return "IV" }` flagged): faking the first example of a new rule is a TDD step; a second special case of the same rule is cheating. The 4B model can't tell them apart (5 framings, incl. counting). The coach now counts branches on exact values in the production code after the Green (`exactValueBranches`): with at most one, a "yes" becomes an OK-level fake-it note. First-example fixtures (go-roman-iv, cs-rover-r, py-tennis-fifteen, go-minus-one) are kept out of the gate suite, since the gate answers yes by design; `cheating.yes.go-roman-ix` (second case) is uncertain (0.72).
- one-behaviour: "several calls that only set up one scenario followed by one check are one behaviour": bowling spare/strike confident-wrong → correct. Cost: `one-behaviour.no.go-add-and-remove` ok → uncertain (0.57); 4 variants tried, none kept it.
- review lenses (all four): "clearly shows" and "a literal returned to pass a first test and a short function with one condition are no": 10/3 → 15/0 on lens fixtures.
- tpp variable option: "a function now returns its parameter instead of a fixed literal": 19 → 21/22.

Not fixed:

- New gate "beyond-test" (Green adds behaviour no test asks for, e.g. the whole tennis endgame for one advantage test): 4 framings, best 2/9 confident. The 4B model can't do it; fixtures kept outside the repo.
- refactor-effect: replacing Roll's spare flags with a rolls array (the classic bowling refactoring) is judged "worsens" p≈0.83; a floor can't separate it from the true "worsens" fixture (0.80 on CPU). Not added as a fixture.
- step-size: "block is stable" (Game of Life) needs `return cells`, judged "complex" p≈0.90; the model reasons about the whole problem. Low impact (the verdict is OK-level "check it generalises"). Not added as a fixture.
- multi on large Greens stays uncertain.
- A refactoring at the very end of a session is never judged: a Refactor step only closes when the next Red starts.
