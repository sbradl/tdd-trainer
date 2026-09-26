# Result readers and fail reason

Status: done
Blocked by: 01
Spec: ../spec.md (§4)

## What

JUnit XML and .NET TRX readers next to go-json; normalised result model (test ID = file+classname+name, state, message, error-vs-failure); build broken vs `<error>` vs `<failure>` classification; exit-code-only fallback with warning.

## Done when

Fixture outputs from pytest, jest, vitest, busted, ExUnit+junit_formatter, dotnet TRX and go test -json parse into the same model; classification unit-tested.

## Notes from implementation

- Real fixtures in `internal/results/testdata/` (busted written from its junit handler source; not installed locally).
- `<failure>` does not mean assertion: pytest reports a NameError inside a test as `<failure>`, and so do jest, ExUnit and TRX for any exception. Exact rule used: `<error>` → wrong reason; a `type` attribute (vitest) → AssertionError = assertion, else wrong; everything else → undecided → `red-check` gate.
- busted puts `:<line>` into classname; stripped from the test ID so moving a test does not make it new.
- Result file is deleted before each run (ExUnit leaves a stale report on compile errors). Results configured + none written + non-zero exit → build broken.
- `results.path` empty → results read from stdout (busted `-o junit` default).
