# Result readers and fail reason

Status: ready-for-agent
Blocked by: 01
Spec: ../spec.md (§4)

## What

JUnit XML and .NET TRX readers next to go-json; normalised result model (test ID = file+classname+name, state, message, error-vs-failure); build broken vs `<error>` vs `<failure>` classification; exit-code-only fallback with warning.

## Done when

Fixture outputs from pytest, jest, vitest, busted, ExUnit+junit_formatter, dotnet TRX and go test -json parse into the same model; classification unit-tested.
