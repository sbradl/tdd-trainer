# Changelog

## Upcoming version

Nothing yet.

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
