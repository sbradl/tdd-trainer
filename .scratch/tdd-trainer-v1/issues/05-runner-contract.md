# Language-agnostic runner contract

Type: grilling
Status: resolved
Blocked by: none
Map: ../map.md

## Question

What does the per-project config look like (test command, test-file glob, anything else)? How does the coach learn from arbitrary runner output which test failed and why (exit code only, structured formats like JUnit XML / go test -json / TAP, judge-read raw output)? What minimum does it need to decide fail reason? Validate against Go, Python, TypeScript runners.

Constraints from [Step inference model](04-step-inference.md): the contract must distinguish "build broken" from assertion failure, identify each failing test so newly failing tests can be told apart, and cope with slow suites since the coach runs tests on every debounced save (run cancellation, maybe running only affected tests).

See [Prior art](02-prior-art.md): TCR per-language YAML config + JUnit XML parser (Go, MIT) and cyber-dojo/TCR build-vs-assertion split are reuse candidates.

## Answer

- **Result readers**: JUnit XML (the common format), `go test -json`, and .NET TRX, so Go and .NET need no extra package. If no result file appears, fall back to exit code only and warn that verdicts will be weaker.
- **Language coverage (checked 2026-09-26)**:
  - Go: `go test -json` built in.
  - .NET: TRX built in (JunitXml.TestLogger optional).
  - Lua/busted: `-o junit` built in; tells `<error>` from `<failure>`.
  - Elixir: needs the `junit_formatter` hex dependency plus `ExUnit.configure formatters: [JUnitFormatter, ExUnit.CLIFormatter]`; `tddt init` prints these lines.
  - Python: pytest `--junitxml`.
  - TypeScript: jest-junit or vitest's junit reporter.
- **Build broken vs assertion**, checked in this order, deterministic first:
  1. Optional `build` command (`go vet`/`go build`, `dotnet build`, `mix compile`, `tsc --noEmit`); if it fails, the state is build broken.
  2. JUnit `<error>` counts as the wrong reason, `<failure>` as an assertion (only pytest and busted make this split).
  3. The rest goes to the judge's red-check gate.
- **Config**: `.tddtrainer.yml` in the project root. Fields: `test` (command, per-OS overrides), `results` (path + format), optional `build`, `tests` and `sources` globs, `ignore`. Built-in presets: Go, Python, TypeScript (jest | vitest), .NET (xUnit | NUnit | MSTest), Lua (busted), Elixir. `tddt init` picks one; the presets later double as auto-detection.
- **Slow suites**: always run the whole suite; a newer save cancels the run in progress; warn when a run takes longer than a limit (default 5 s). Running only affected tests is out of scope for v1.
- **Test identity**: file + classname + name from the results. New tests = IDs not seen in the previous run, found by comparing runs rather than parsing code. A test that disappears while another appears in the same green step counts as a rename, i.e. refactoring the tests.
- **Test vs source files**: decided by the `tests`/`sources` globs; files matching neither are ignored.
- Sources: busted junit handler https://github.com/lunarmodules/busted/blob/master/busted/outputHandlers/junit.lua ; https://github.com/victorolinasc/junit-formatter ; https://github.com/spekt/junit.testlogger
