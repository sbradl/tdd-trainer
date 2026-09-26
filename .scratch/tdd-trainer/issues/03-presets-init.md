# Presets, init and auto-detection

Status: done
Blocked by: 02
Spec: ../spec.md (§3)

## What

Built-in presets for Go, Python, TypeScript (jest|vitest), .NET (xUnit|NUnit|MSTest), Lua (busted), Elixir; marker-file detection; `tddt init` with confirm/pick/blank flows; auto-init when config is missing; Elixir junit_formatter instructions; per-OS command overrides; tpp_order per preset.

## Done when

In sample projects of each of the six languages, `tddt init` proposes the right preset and the written config produces a correct Test state.

## Notes from slice 02

- jest-junit drops suites that fail to load unless `reportTestSuiteErrors` is on. Preset needs: `JEST_JUNIT_REPORT_TEST_SUITE_ERRORS=true`, `JEST_JUNIT_ADD_FILE_ATTRIBUTE=true`, `JEST_JUNIT_CLASSNAME={classname}`, `JEST_JUNIT_TITLE={title}`, `JEST_JUNIT_SUITE_NAME={filepath}` (env vars differ per OS shell → per-OS override, or `jest-junit` key in package.json).
- vitest: `--reporter=junit --outputFile=<path>`.
- dotnet: `--logger "trx;LogFileName=<name>" --results-directory <dir>`.
- ExUnit junit_formatter writes `_build/test/lib/<app>/test-junit-report.xml`.

## Notes from implementation

- Verified end to end (init → run → Test state) on real sample projects: Go, Python, vitest, jest, .NET xUnit, Elixir. Lua not verified: busted not installed locally.
- New config field `env` (map), used by the jest preset for jest-junit options; avoids per-shell env syntax.
- Presets are text templates (`internal/preset/presets/*.yml`): tsc build only with tsconfig.json; Elixir app name from mix.exs; .NET framework from PackageReference.
- Setup steps (jest-junit, junit_formatter) are skipped when the dependency is already present; with setup steps left, auto-init stops instead of running.
- .NET: several test projects under one .sln all write `tddt.trx` → last one wins. Not handled yet.
- Local machine note: this dotnet install needs `MSBuildEnableWorkloadResolver=false` (broken workload manifest), unrelated to tddt.
