# Changelog

## Upcoming version

### Sessions

- **`tddt --resume` continues the last session** after a restart (say, to update `tddt`). Step numbers go on, `tddt show` and the report cover the whole session, and the time `tddt` wasn't running doesn't count. If you were in a Red, the Green that follows is judged as usual; after a Green, its open hints come back and update as you refactor. Changes made while `tddt` was off count like a save.

### Coaching

- **Fewer false hints, tuned on longer katas** (bowling, Game of Life, Mars Rover, word wrap, tennis, prime factors):
  - A test that sets up a scenario with several calls (say, three rolls) and checks it once no longer gets "checks more than one behaviour".
  - A first fake-it step (`return "Love-All"`) or a short function with one `if` no longer gets a *Refactor now* hint about magic strings or long functions.
  - **Special cases are counted.** One branch for a single test value (`if n == 4 { return "IV" }`) is a fake-it step for the first example of a new rule: it gets a note to generalise with the next example, not a hint, and *Refactor now* doesn't ask you to refactor it away (that would change behaviour without a test). A Green that adds another one (`n == 5`, `n == 6`, or `commands == "R"` then `"RR"`) gets "Another branch for a single test value: the code now special-cases 3 test values", even when the judge isn't sure. Computed conditions (`n % 3 == 0`) and the zero/empty guard don't count.
  - Returning a parameter instead of a literal (`return cells`, `[]int{n}`) is recognised as Constant → Variable.

- **Green hints update as you refactor.** After *A simpler change would have done* or *The code special-cases the test's inputs*, fixing the code while the tests are green now re-checks the hint: it turns into *Resolved*, or says the problem is still there. Before, both stayed until the end of the session.

### Fixes

- **A build broken while refactoring is no Red.** If only code changed, the card says "The build broke while refactoring: make it compile again" instead of asking for a stub. Once it compiles, a test that fails now is "a test that passed before is failing now", not a Red in progress that never goes away.
- **Fixing a test you broke goes on where you were.** Broken while refactoring, getting back to green continues the refactoring; before, it counted as a new Green that judged your fix as if it were new code, and the hints of the real Green stopped updating. Either way, the warning "A test that passed before is failing now" turns into *Resolved* once all tests pass again, instead of staying for the rest of the cycle.
- **Starting in a fresh kata folder works.** With only `go.mod` (or no tests yet), some runners fail, and the session started as "Red in progress", so the first cycle was never judged. Now a folder without tests is an empty, green start.
- **A typo or a missing function in a new test is "Red in progress" in Python and Elixir.** A `NameError`, `ImportError` or Elixir `UndefinedFunctionError` completed the Red at once and then warned about the wrong reason; fixing the typo was flagged as "a test was changed while making it pass". Now these are recognised exactly, and the Red completes when the test fails on its assertion. A crash inside the code under test is still left to the judge.
- **"Small test" ignores imports and blank lines**, so a first table-driven Go test no longer gets a size hint for its `package` and `import` lines.

## 2026-09-26.2

### Coaching

- **Next-test hints.** Stuck on which test to write next? Press `n` while the tests are green. The first press names the kind of test ("Try an edge case."), the second the case, and the transformation it will probably need. Hints follow the ZOMBIES checklist: zero/empty, one, many, boundaries, errors. Right after a Green that faked its result with a constant, the hint is to triangulate with a second example. The judge looks at your tests only when you ask, ahead of other checks, so the first press takes a few seconds. When it isn't sure, it says so instead of guessing.
- **`tddt next`** prints the same hint for the code as it is now, without a running session.
- **The test code is reviewed too.** Right after each Green, *Refactor tests now* points out test bodies that repeat the same steps, names that don't say which behaviour they check, expected values computed instead of written down, and failures that don't say what was expected and what came out. It updates as you refactor the tests, and the next Red notes if you skipped it.
- **Faster refactor review.** The domain-design lens is gone: it asks design questions above the level of one TDD cycle. The remaining questions are shorter, so the code review after each Green takes about a third less time; the new test review uses about that much again. The next-test hint's second stage is faster too.
- **Refactor hints name the file**, as in "Worth refactoring roman.go now…", so it's clear they are about the production code, not the tests.

### Fixes

- **Filling in an existing test now starts a Red.** If you write an empty test first and give it its body later, the Red completes once the test fails on its assertion. Before, it stayed in "Red in progress" forever. A passing test you change so that it fails now counts as a Red too, not as a broken test, as long as you changed only test files.
- **Refactoring tests no longer looks like a new test.** If a test refactoring broke the build for a moment (say, a half-written helper), fixing it gave the warning "a new or changed test passed without failing first". Now it counts as a refactoring.
- **Refactor now updates after every refactoring.** When the judge wasn't sure your refactoring removed the problem, the hint stayed as it was, as if nothing had happened. Now the review that raised the hint looks again at the code as it is, and the hint says *Resolved*, *Still there* or *Probably resolved*. If the refactoring removed the problem but left another one (say, a placeholder string), the hint says so and follows the new problem, instead of claiming the old one is still there.

### Reports

- The focus tips say how often you asked for the next test, and for which cases most often. A new section lists each hint.

## 2026-09-26

The first release. Everything below is new.

### Coaching

- **Watches while you work.** `tddt` runs your tests on every save and works out whether you just finished a Red, Green or Refactor step. A newer save cancels a test run still in progress.
- **Checks every step:**
  - Red: one new test, small, failing on its assertion.
  - Green: the simplest change (Transformation Priority Premise), one transformation at a time, no special-casing of test inputs.
  - Refactor: tests stayed green, behaviour unchanged, design effect.
- **Refactor now.** Right after each Green, the new code is reviewed for things worth cleaning up, and the hint names what, for example "repeated conditionals". Every green save while you refactor re-checks it, until it says *Resolved*. If you start the next test without cleaning up, the report counts it as a missed refactor.
- **Red in progress.** A compile error counts as failing: the coach asks for the smallest stub, and the Red is complete once the test fails on its assertion.
- **Rhythm warnings:** several new tests at once, a test that passes without failing first, code written without a failing test, a test edited while making it pass, an existing test breaking.
- **Advisory only.** Nothing is ever blocked or reverted. When the judge isn't sure, it stays quiet during the session and lists the verdict in the report.

### Terminal UI

- **Quiet coach** (default): one status line, and cards only for hints and warnings. Finished cycles fold into one summary line.
- **Dashboard** (Tab): the cycle strip, the test state, and every check of the current step, including pending ones with their expected time.
- **Keys:** `r`/`g`/`f` tell the coach which phase you are in, `b` resets the baseline, `w` writes the report, `q` quits.
- Plain line output when stdout is not a terminal.

### Reports

- A Markdown **session report** in `.tddtrainer/reports/`, written on quit or with `w`. It contains:
  - focus tips;
  - a table per cycle;
  - your transformation path;
  - anomalies and missed refactors;
  - uncertain verdicts;
  - a legend of the checks.
- **`tddt show <step>`** prints any step of the last session with its verdicts and full diff.

### Languages

- Presets with auto-detection for Go, Python (pytest), TypeScript/JavaScript (jest, vitest), .NET (xUnit, NUnit, MSTest), Lua (busted) and Elixir (ExUnit). `tddt init` writes the config; `tddt` starts it automatically when there is none.
- Reads JUnit XML, `go test -json` and .NET TRX results. Without any of them it falls back to the exit code.

### The judge

- Runs **locally**; your code never leaves your machine. It uses Qwen3.5-4B through llama.cpp, on a GPU via Vulkan when available, otherwise on the CPU.
- **`tddt setup`** downloads the pinned llama.cpp libraries and the model, SHA-256 checked, with resume and an offline option (`--model-file`), then runs a self-check.
- **`tddt judge --regress`** checks the judge against its labelled fixtures: 92 of 101 answered confidently and correctly, none confidently wrong, on both CPU and GPU.

### Platforms

- Linux and Windows, amd64. Windows is not verified on real hardware yet.
