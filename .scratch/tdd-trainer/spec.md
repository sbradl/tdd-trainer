# TDD Trainer v1: spec

Status: ready for implementation (2026-09-26)
Source of every decision: the wayfinder map [TDD Trainer v1](../tdd-trainer-v1/map.md). Each section links the ticket that holds the detail. Vocabulary: [CONTEXT.md](../../CONTEXT.md).

## 1. What it is

`tddt` is a cross-platform (Linux + Windows, amd64) terminal coach for a **Learner** practising TDD. It watches a project directory, runs the tests on every save, infers **Red / Green / Refactor steps**, and has a local **Judge** give advisory **Verdicts**:
- is the new test small (one behaviour)?
- does it fail for the right reason?
- was the Green change the simplest one, following the Transformation Priority Premise?
- did the Refactor keep behaviour and improve the code?

It never blocks or reverts. At the end of a **Session** it writes a report.

Works in any language via config. Reference presets: Go, Python, TypeScript (jest | vitest), .NET (xUnit | NUnit | MSTest), Lua (busted), Elixir.

### Out of scope for v1
- team/trainer dashboards
- scores, gamification, trends across sessions
- gatekeeping / TCR
- editor plugins
- cloud judge
- syntax-tree (tree-sitter) TPP rules
- running only affected tests
- package managers and code signing
- HTML report

## 2. Architecture

```
tddt (single Go binary, CGO_ENABLED=0)
├── cli          init | setup | (run) | show <step> | judge --regress
├── config       .tddtrainer.yml + built-in presets + marker-file detection
├── watcher      fsnotify, debounce, ignore globs
├── runner       build cmd → test cmd (cancellable) → result readers → Test state
├── snapshots    shadow git repo in .tddtrainer/ (go-git), one commit per test run
├── steps        Test-state transition machine → Steps, Anomalies, Baseline, overrides
├── judge        yzma/llama.cpp backend, option scoring, gates, scheduler
├── tui          Quiet coach view (default) + Dashboard view
└── report       Markdown session report
```

Data flow: save → debounce → build/test run → Test state + snapshot → step machine → (step complete) → exact checks → judge gate queue → verdicts → TUI + report.

## 3. Config and presets
Detail: [Language-agnostic runner contract](../tdd-trainer-v1/issues/05-runner-contract.md), [Preset auto-detection](../tdd-trainer-v1/issues/13-preset-autodetect.md)

`.tddtrainer.yml` in the project root:

```yaml
preset: go                     # informational
build: go vet ./...            # optional; failure = build broken
test:
  cmd: go test -json ./...
  windows: go test -json ./... # optional per-OS override
results: { format: go-json }   # junit-xml | go-json | trx, + path for file formats
tests:   ["**/*_test.go"]
sources: ["**/*.go"]
ignore:  [".tddtrainer/**", "vendor/**"]
tpp_order: iteration-first     # or recursion-first (Elixir preset)
slow_run_warning: 5s
```

- `tddt init` detects presets from marker files. If there is no match, it offers all presets or a blank config; if there are several (monorepo), the learner picks one and the config is rooted there. It writes the file after confirmation.
- Marker files: `go.mod` → Go; `*.csproj`/`*.sln` → .NET (framework from package refs); `mix.exs` → Elixir; `package.json` → TypeScript (jest or vitest from its deps); `pyproject.toml`/`pytest.ini` → Python; `.busted`/`*.rockspec` → Lua.
- Running `tddt` without a config starts `init` automatically.
- Elixir needs `junit_formatter` plus `ExUnit.configure formatters: [JUnitFormatter, ExUnit.CLIFormatter]`; `init` prints those lines.
- Files matching neither `tests` nor `sources` are ignored for step logic.

## 4. Runner and test state
Detail: [runner contract](../tdd-trainer-v1/issues/05-runner-contract.md), [Step inference model](../tdd-trainer-v1/issues/04-step-inference.md)

- Tests run on every save, debounced (~300–500 ms). A newer save cancels the run in flight. The full suite always runs. If a run takes longer than `slow_run_warning`, the TUI shows a warning.
- Order: `build` (if set) → `test` → read results.
- Result readers: JUnit XML, `go test -json` and .NET TRX. If no result file appears, the coach falls back to the exit code only and shows a one-time warning that verdicts will be weaker.
- **Test state** = the set of failing test IDs, or `build broken`. A test ID is file + classname + name.
- Fail reason, decided exactly first: a failing build command → build broken; JUnit `<error>` → wrong reason; `<failure>` → assertion. Only pytest and busted mark `<error>` separately. Anything the exact checks can't decide goes to the `red-check` gate.

## 5. Snapshots
- A shadow git repo in `.tddtrainer/` has its own git dir and uses the project as its work tree, via go-git. It never touches the learner's repo and doesn't need a system git.
- One snapshot per test run. Steps reference snapshot ranges, which feed the judge's diffs, `tddt show <step>` and the report.
- `.tddtrainer/.gitignore` contains `*`.

## 6. Step inference
Detail: [Step inference model](../tdd-trainer-v1/issues/04-step-inference.md)

A **Step** is every save between two Test-state transitions, judged when the transition happens:

| From | To | Step |
|---|---|---|
| all green | exactly one new failing test, failing on an assertion | Red |
| all green / Red | build broken or new test erroring | Red in progress (not complete) |
| failing | all green | Green |
| green | green with code changed | Refactor (accumulates until the next Red starts) |

- **Anomalies** get an advisory verdict and never block:
  - several new tests in one step
  - a new test that passes immediately
  - production code changed without a failing test
  - a test edited during Green
  - a test that goes red during Refactor
- **Baseline**: the first snapshot. A session that starts red is treated as Red in progress and gets a warning.
- **Overrides**: hotkeys `r`/`g`/`f` set the phase; `b` resets the baseline.
- Tests may be restructured while green; that is judged like a production-code Refactor. A test that disappears while another appears in the same green step counts as a rename.
- New tests are counted by comparing runs, not by parsing code.

## 7. Judge
Detail: [Heuristic vs judge split](../tdd-trainer-v1/issues/06-heuristic-vs-judge.md), [Judge eval prototype](../tdd-trainer-v1/issues/07-judge-eval.md), [Go scoring spike](../tdd-trainer-v1/issues/14-go-scoring-spike.md), [TPP research](../tdd-trainer-v1/issues/03-tpp-list.md)

**Principle**: exact checks first; the judge decides only what they can't.

**Method** (SemIf direct mode, ported):
- The prompt is the Qwen ChatML template with an empty `<think>` block. The user turn is JSON `{evidence, question, options}`, with options lettered A–P, encoded exactly like Python `json.dumps(ensure_ascii=False)`.
- One forward pass; softmax over the logits of the option letters. No text is generated.
- Model: Qwen3.5-4B Q4_K_M GGUF (3.0 GB, Apache-2.0).
- The model stays loaded for the whole session.
- Reuse the model state within a step: prefill the shared evidence once, then per gate restore it and decode only the question and options.
- Evidence parts are labelled `Test`, `Test-runner output`, `Current source` and `Source diff`, each clipped to 6000 characters, keeping the head and tail.
- **Verdict** = the top answer if p ≥ 0.8 (`step-size` ≥ 0.45), otherwise `uncertain`.
- **Review-lens gates** ask twice (detect / clean) and answer "no" only when both agree.

**TPP list** (*Clean Craftsmanship*, 2021), in order:
1. {}→Nil
2. Nil→Constant
3. Constant→Variable
4. Unconditional→Selection
5. Value→List
6. Selection→Iteration
7. Statement→Recursion
8. Value→Mutated Value

Recursion vs iteration order is configurable per preset. **Step-size bands**: constant = items 1–2, simple = 3–4, complex = 5–8.

**Gates**:

| Criterion | Exact checks | Gates (evidence) |
|---|---|---|
| Test small | exactly 1 new test ID; added test lines ≤ ~15 | `one-behaviour` (test) |
| Right fail reason | build cmd, `<error>` vs `<failure>` | `red-check` (test, runner output) for leftovers only |
| Simplest code | none | `tpp` (diff); `step-size` (test, Red's runner output, source before Green): an applied band above the predicted band → hint; `multi` (diff): 2+ transformations → "test probably missing"; `cheating` (test, diff) |
| Refactor | stayed green; no new test IDs | `structural` (diff); `refactor-effect` (diff): improves / neutral / worsens |
| Missed refactor | a Green followed directly by a new Red | review lenses (Green's diff): `review-smells`, `review-clean-code`, `review-ddd`, `review-pragmatic`, `review-philosophy` |

**Scheduling**:
- One judge worker runs gates in the background.
- Priority: `red-check` first, then the Green gates, then the Refactor gates, and review lenses last.
- Pending gates of older steps still complete.
- Expected latency with the model loaded: about 1 s per gate on a Vulkan iGPU, 3–8 s on CPU.

**Backends**: use Vulkan automatically when `ggml-vulkan` loads, otherwise CPU.

**Measured baseline** (88 fixtures in 6 languages):
- Python reference: 66/88 confident and correct, 0 confident and wrong.
- Go port: CPU 68/88, Vulkan 67/88.
- Weak gates: `multi`, `structural`, `tpp` for variable and list. Tune them during implementation against the fixtures.
- Fixtures: branch `prototype/judge-eval` (`prototype-judge-eval/fixtures`), named `<gate>.<expected>.<tag>/` containing `test.txt`, `transcript.txt`, `source.txt` and `diff.txt`. They become the product's regression suite (`tddt judge --regress`). Any change to a question or option must be checked against it. Re-baseline the fixtures in state-reuse mode.

## 8. TUI
Detail: [Verdict and terminal UX](../tdd-trainer-v1/issues/09-verdict-ux.md); prototype on branch `prototype/verdict-ux`

- **Quiet coach** (default):
  - one status line: phase, test summary, OK count, "judging N" spinner
  - hints and warnings, anomalies included, appear as cards as soon as they arrive
  - a closed cycle collapses into one summary line
- **Dashboard** (toggle, e.g. Tab):
  - phase banner and cycle strip
  - the current step's gate table, with pending gates and their ETA
  - hotkey bar
- OK verdicts appear only as a counter. `uncertain` appears only in the report, greyed in the Dashboard.
- No bell and no desktop notifications.
- Keys: `r`/`g`/`f`/`b` overrides (a toast confirms them), report-now key, `q` quit.

## 9. Session report
Detail: [Session report](../tdd-trainer-v1/issues/10-session-report.md)

- Markdown file at `.tddtrainer/reports/<timestamp>.md`, plus a short summary in the terminal on quit. Written on quit (`q`/Ctrl-C) and on demand.
- Contents:
  - header: duration, cycles, tests added, share of clean cycles
  - per-cycle table of steps and verdicts
  - the TPP path
  - anomalies and missed refactors
  - an `uncertain` section
  - the top 3 most frequent hints as focus tips
- Diff snippets only for steps that got a hint or warning; full diffs via `tddt show <step>`.
- No scores and no cross-session trends.

## 10. Setup and distribution
Detail: [Stack and distribution](../tdd-trainer-v1/issues/08-stack-distribution.md)

- Go single binary using yzma v1.28.0 (purego, no cgo), cross-compiled for linux/amd64 and windows/amd64.
- The release archive per OS bundles the llama.cpp shared libraries pinned to the yzma tag (b11146): CPU variants and the Vulkan backend. Sizes:
  - Linux: 23 MB (CPU only), 66 MB with Vulkan
  - Windows: similar, plus `libomp.dll`, and `MSVCP140.dll`, `VCRUNTIME140.dll` and `VCRUNTIME140_1.dll` placed next to the exe.
- `tddt setup` downloads the model into the user cache dir, checks its SHA-256 and shows progress. A flag or env var points at an existing file for offline installs. `go install` users get the libraries through `tddt setup` too.
- Channels: GitHub releases and `go install`. No code signing; the docs explain the SmartScreen warning.
- CI:
  - unit tests
  - `tddt judge --regress` on CPU. Vulkan regress needs a GPU runner or a local run before each release.
  - a Windows build smoke test

## 11. Risks and unverified
- Windows is not yet verified: DLL search path, latency, and whether p values match Linux.
- p drifts by about ±0.05 across builds and backends, which matters near the 0.8 cutoff. Watch the per-backend regress results; calibrate only if needed.
- Anomaly noise: in the WatchDog study, strict TDD appeared in only 2.2% of real sessions. The Quiet coach keeps this in check; watch it in real use.
- CPU-only machines: 3–8 s per gate, and a green step takes about 20–30 s before all its verdicts are in.

## 12. Implementation slices
See [issues/](issues/), in tracer-bullet order; each slice ends in something usable:

1. Walking skeleton
2. Result readers
3. Presets and init
4. Watcher
5. Snapshots
6. Step inference
7. Judge core
8. Gates and scheduler
9. TUI
10. Report
11. Setup and release
12. Judge tuning
