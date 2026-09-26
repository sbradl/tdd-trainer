# Presets, init and auto-detection

Status: ready-for-agent
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
