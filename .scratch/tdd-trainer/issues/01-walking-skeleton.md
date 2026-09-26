# Walking skeleton

Status: ready-for-agent
Blocked by: none
Spec: ../spec.md (§2, §3)

## What

Go module + `tddt` CLI. Loads a hand-written `.tddtrainer.yml` (Go preset shape), runs `build` then `test` once, reads `go test -json`, prints the Test state (failing test IDs or build broken). Cross-compiles for linux/amd64 and windows/amd64 with CGO_ENABLED=0.

## Done when

`tddt` in a Go kata dir prints the correct Test state for green, red and build-broken; both GOOS builds succeed in CI.
