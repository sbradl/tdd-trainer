# TDD Trainer v1

Label: wayfinder:map

## Destination

Implementation-ready v1 spec for a cross-platform (Linux + Windows) TDD coach: watches a project dir live, infers red/green/refactor steps, judges test size, fail reason, simplest code (TPP) and refactor quality; language-agnostic; judging runs locally. Plan, don't build.

## Notes

- Domain: TDD coaching. Vocabulary in `CONTEXT.md` (Learner, Session, Step, Fail reason, Transformation, Judge, Verdict) — use it.
- Skills: `grilling` + `domain-modeling` for grilling tickets; `research` for research tickets; `prototype` for prototype tickets.
- Standing decisions from charting:
  - Learner = anyone wanting to check they do TDD correctly.
  - Advisory only (verdict + short explanation/hint per step, session report at end). No gatekeeping.
  - Live file watcher; steps inferred from test results + diff, learner can override phase.
  - Language-agnostic via per-project config (test command, test-file glob); auto-detect later.
  - Terminal UI first (split pane), editor-agnostic.
  - Judge is local only (SemIf-style option scoring, https://github.com/TheoLeeCJ/SemIf); no cloud judge in v1.
  - Very easy setup: Go single binary + `tddt setup` for the model (decided in Stack and distribution).
  - Reference learner languages (presets + evals): Go, Python, TypeScript, .NET, Lua, Elixir.
- Tracker: local markdown. Research findings live in `research/` next to this map.

## Decisions so far

<!-- one line per closed ticket: [title](issues/NN-slug.md): gist -->
- [Step inference model](issues/04-step-inference.md): coach runs tests on debounced save; steps = saves between test-state transitions; shadow git in `.tddtrainer/`; compile error = red in progress; anomalies get advisory verdicts
- [Transformation Priority Premise: canonical list and detection](issues/03-tpp-list.md): no canonical list; use Martin's 2021 8-item list (Clean Craftsmanship) as default, with configurable order (recursion vs iteration varies by language); no existing TPP detector, so build one (tree-sitter/GumTree AST rules + LLM judge); also flag multi-transformation green steps
- [Prior art: TDD coaching tools](issues/02-prior-art.md): LLM TDD judges (TDD Guard/Probity) target AI agents, block, cloud-only; human step classifiers (Zorro, WatchDog) dead/Java; cyber-dojo/TCR only pass/fail; none judge TPP. Gap confirmed; reuse their rules text, amber-vs-red fail split, TCR-style per-language YAML
- [SemIf feasibility](issues/01-semif-feasibility.md): don't ship SemIf's Python stack; reimplement its option-logit readout in Go on llama.cpp (llama-server logprobs or yzma purego). CPU and Vulkan builds cover Linux and Windows. Qwen3.5-4B Q4 (3 GB) is the only judge-grade model: about 5–28 s per verdict on CPU, 1–6 s on Vulkan iGPU, so judge asynchronously
- [Language-agnostic runner contract](issues/05-runner-contract.md): `.tddtrainer.yml` + per-language presets; read JUnit XML / `go test -json` / TRX; build cmd → build broken, `<error>` vs `<failure>`, judge for leftovers; tests identified by comparing runs
- [Heuristic vs judge split](issues/06-heuristic-vs-judge.md): exact checks first, judge for leftovers; TPP = 2021 8-item list, judge-only detection; reuse tdd-judge gates + new `one-behaviour`, `refactor-effect`; missed-refactor hint; `uncertain` silent live
- [Judge eval prototype](issues/07-judge-eval.md): local Qwen3.5-4B 66/88 confident+correct, 0 confident wrong across 6 languages; ~7 s/gate warm → model stays loaded, gates run in the background, red-check first; weak gates (multi, structural, tpp variable/list) tuned during implementation
- [Cloud judge options](issues/11-cloud-judge-options.md): OpenAI (effort none) + OpenAI-compatible hosts (Together Qwen3.5-9B, self-hosted) give top_logprobs → same option readout as local; Anthropic enum-only (no logprobs), Gemini logprobs unreliable + free tier trains → defer; cents/session, no paid API trains; build OpenAI-compatible HTTP backend first, Anthropic second, pick model via the 22 uncertain fixtures
- [Go scoring spike](issues/14-go-scoring-spike.md): a pure-Go port via yzma (llama.cpp, no cgo) matches SemIf's prompts token for token; 86/88 verdicts agree (68/88 confident+correct); Vulkan iGPU ~1 s/gate vs 4–8 s on CPU; reusing state across one step's gates works (2–7× faster); ships as a 4 MB binary + 23 MB (CPU) / 66 MB (Vulkan) libs + a 3 GB model; Windows needs the VC++ runtime
- [Stack and distribution](issues/08-stack-distribution.md): Go + yzma, no cgo; release archive bundles llama.cpp libs, `tddt setup` downloads the 3 GB model (Apache-2.0); Vulkan automatically else CPU; state reuse; CI runs fixtures on both backends; VC++ DLLs next to the exe on Windows; GitHub releases + `go install`
- [Verdict and terminal UX](issues/09-verdict-ux.md): default Quiet coach (status line + hint/warning cards, cycles collapse to summaries), Dashboard view on a toggle; OKs only counted, uncertain only in report, no bell/notifications
- [Session report](issues/10-session-report.md): Markdown in `.tddtrainer/reports/` + terminal summary on quit; per-cycle verdicts, TPP path, anomalies, uncertain, top-3 tips; no scores/trends
- [Preset auto-detection](issues/13-preset-autodetect.md): `tddt init` suggests a preset from marker files (confirm to write config); pick on multiple matches, blank or preset list on none; runs automatically when config is missing

## Handoff

Destination reached (2026-09-26): [spec](../tdd-trainer/spec.md) + 12 implementation slices in [../tdd-trainer/issues/](../tdd-trainer/issues/).

## Not yet specified

(none)


## Out of scope

- Team/trainer dashboards, sharing sessions.
- Scores, gamification, trends across sessions.
- Gatekeeping / TCR (blocking or reverting code).
- Editor plugins (v1).
- Cloud judge (v1): local judge only; see [Cloud judge and privacy](issues/12-cloud-judge-privacy.md) and the research in [Cloud judge options](issues/11-cloud-judge-options.md).
- Package managers (scoop, winget, AUR, Homebrew) and code signing (v1), see [Stack and distribution](issues/08-stack-distribution.md).
- Syntax-tree (tree-sitter) TPP detection rules (v1): judge-only detection, see [Heuristic vs judge split](issues/06-heuristic-vs-judge.md).
