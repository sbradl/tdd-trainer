# Prior art: TDD coaching tools

Research for [issue 02](../issues/02-prior-art.md). Retrieved 2026-09-26. Sources are repos/source code unless noted.

## Summary table

| Tool | Judges | How | Step detection | Languages | Setup | Mode | Status |
|---|---|---|---|---|---|---|---|
| TDD Guard | test-first, one test at a time, minimal impl, fail reason, refactor on green (+lint) | LLM (Claude, default Sonnet) per edit | Claude Code hook on each file write + last test run captured by per-framework reporter | 9+ via reporters (Vitest, Jest, pytest, Go, Rust, PHPUnit, RSpec, Minitest, Storybook; dotnet/junit5 reporters in repo) | Claude Code plugin + install a reporter per project; Node 22 | Blocking (AI agent) | Maintained, superseded by Probity |
| Probity | same TDD rule set + missed refactor at green→red boundary | LLM via agent vendor SDK; optional deterministic fast-path (AST count of new tests) | Reads agent session transcript (tool calls + outputs) — no reporters | Any (transcript); AST test counting for TS/JS/Py/C#/PHP/Ruby | npm dev-dep + config file + per-agent hook | Blocking (AI agent) | Active |
| Zorro / Besouro | classifies episodes: test-first, refactoring, test-last, test-addition, regression, production | Jess rule engine over IDE event stream | Episodes bounded by test runs; rules over edit/compile/test events | Java (Eclipse) | Eclipse plugin, build yourself, Jess non-OSS | Advisory/research | Dead (last push 2018) |
| WatchDog (TU Delft) | % of changes following TDD (strict/lenient) | NFAs → regexes over interval stream | Typing(test/prod) + JUnit run intervals | Java (Eclipse, IntelliJ), C# variant | IDE plugin + server | Advisory reports | Dead (2018) |
| cyber-dojo | red/amber/green per run, optional prediction | Per-language regex lambda on stdout/stderr/status | Every explicit "test" click = one light | ~any (Docker images per language) | Browser, hosted or self-hosted | Advisory (humans review) | Active |
| murex/TCR | nothing qualitative; enforces baby steps | Build → test → commit or revert | Build fail vs test fail distinguished; fsnotify watcher | 13 languages via YAML; JUnit XML parsing | Single Go binary, git required | Gatekeeping | Active |
| JetBrains "Red Green Refactor" | nothing — phase visualiser | manual/visual | n/a | JVM IDEs | Plugin | Visual aid | Dead (2021) |
| TDD Studio | nothing — continuous runner + coverage | Build/test on every file change | n/a | C#/F#/VB, VS 2013/15 | VS extension | Runner | Dead (2019) |
| tdd-bdd-commit | nothing — labels commits red/green/refactor | Learner declares phase | Manual | any (git) | CLI | Discipline aid | Small |

## Details

### TDD Guard — https://github.com/nizos/tdd-guard (MIT, ~2.3k stars)

- README: "TDD Guard ensures Claude Code follows Test-Driven Development principles. When your agent tries to skip tests or over-implement, TDD Guard blocks the action". Now points to Probity: "New projects should start there."
- Judge = Claude via Agent SDK (default) or Anthropic API; default model `claude-sonnet-4-6`; docs call Haiku "Fastest but unreliable results" ([docs/validation-model.md](https://github.com/nizos/tdd-guard/blob/main/docs/validation-model.md)). No local-model option.
- Test results: each framework needs a reporter that writes a normalised JSON to `.claude/tdd-guard/data/test.json`; Go reporter is a stdin filter over `go test -json` ([reporters/go/README.md](https://github.com/nizos/tdd-guard/tree/main/reporters/go)). Schema: `testModules[].tests[] {name, fullName, state: passed|failed|skipped, errors[]}`, plus `unhandledErrors`, `reason` ([src/contracts/schemas/reporterSchemas.ts](https://github.com/nizos/tdd-guard/blob/main/src/contracts/schemas/reporterSchemas.ts)).
- Rules prompt ([src/validation/prompts/rules.ts](https://github.com/nizos/tdd-guard/blob/main/src/validation/prompts/rules.ts)) is directly reusable as a checklist: one test at a time; fail for the RIGHT reason (not syntax/import); "Reaching a clean red" ladder — unresolved symbol → empty stub only; signature mismatch → adjust signature; assertion failure → minimal logic; refactor only on green; restructuring tests ≠ adding tests; adding types/constants/extracting covered helpers is fine in refactor.
- Output: `{"decision": "block"|null, "reason": ...}` with guidance: name violation, why, correct next step ([response.ts](https://github.com/nizos/tdd-guard/blob/main/src/validation/prompts/response.ts)). Explicitly excludes style/perf/design.
- Also runs linters (eslint, golangci-lint, rubocop) post-edit for refactor nudges.

### Probity — https://github.com/nizos/probity (MIT)

- Successor to TDD Guard, for Claude Code, Codex, Copilot CLI. "reads each agent's session transcript directly, so there are no per-framework reporters to install" (README).
- `enforceTdd()` ([src/rules/enforce-tdd.ts](https://github.com/nizos/probity/blob/main/src/rules/enforce-tdd.ts)): prompt = role + rules + last 10 session events (6000-char cap each) + current file + pending write → `{"kind":"pass"|"violation","reason"}`. Notable rule refinements over TDD Guard:
  - judge the *change*, not the resulting file; transient broken states are never violations (multi-write phases);
  - stub rule: "Returning a literal that contradicts the assertion … or throwing `not implemented` are both valid stubs";
  - characterization/pinning tests may pass immediately;
  - "Enforcing the refactor phase": at the green→red boundary, flag an *unmistakable* missed refactor, with a deliberately high bar ("forcing a refactor risks needless abstraction").
- Deterministic fast-path: ast-grep counts test nodes before/after; `after - before == 1` → pass without AI ([matchers/count-new-test-nodes.ts](https://github.com/nizos/probity/blob/main/src/rules/matchers/count-new-test-nodes.ts)). Off by default because it would skip the refactor check.

### Zorro / Besouro — https://github.com/brunopedroso/besouro

- Besouro = standalone Eclipse port of Zorro (Hackystat). Requires Jess rule engine, which "could not be redistributed due to it's non-open-source license" (README).
- Rules ([EpisodeClassifier.clp](https://github.com/brunopedroso/besouro/blob/master/src/besouro/classification/zorro/EpisodeClassifier.clp)): episode ends at a passing test run; categories test-first (types 1–4: with/without compile error, with/without failing test), refactoring (test-only / prod-only / both), test-last, test-addition, regression, production (sized by method/statement/byte deltas). Canonical pattern: "Test creation -> Compilation error -> Code -> Test Fail -> code -> Test Pass".
- Also has a per-episode "disagree" button (learner overrides classification) — same idea as our phase override.
- Zorro validation: recognised TDD episodes correctly ~89% vs observer ([Kou et al., pilot study](https://csdl.ics.hawaii.edu/techreports/2006/06-02/06-02.pdf)).

### WatchDog — https://github.com/TestRoots/watchdog

- Beller et al., "Developer Testing in the IDE" (TSE 2017, [PDF](https://inventitech.com/assets/publications/2017_beller_gousios_panichella_amann_proksch_zaidman_developer_testing_in_the_ide_patterns_beliefs_and_behavior.pdf)): TDD modelled as NFAs (strict, lenient, refactor), converted to regexes, matched over a linearised stream of Typing(test|prod) and JUnitExecution(ok|fail) intervals.
- Lenient NFA exists because strict test→fail→prod→ok "is difficult to follow in reality": compile errors force mixing prod edits into test creation. Only 2.2% of sessions with test runs contained strict TDD patterns.
- Gave per-developer reports ("You followed TDD 38.55% of your development changes").

### cyber-dojo — https://cyber-dojo.org, https://github.com/cyber-dojo

- Colours: red = "one or more tests failed", amber = "the tests did not run (eg syntax error)", green = "the tests ran and all passed" ([blog 2014](https://blog.cyber-dojo.org/2014/08/traffic-light-transitions.html)).
- Per-language `red_amber_green.rb`: Ruby lambda `|stdout,stderr,status|` → `:red|:amber|:green`, regex-based, amber default ([blog: adding a language](https://blog.cyber-dojo.org/2016/08/adding-new-language-and-unit-test.html)); runner returns `stdout, stderr, status, timed_out, colour` ([cyber-dojo/runner](https://github.com/cyber-dojo/runner)).
- Optional traffic-light *prediction*: learner predicts red/amber/green before the run; "helps to provide strong feedback on how 'in control' you are" ([cyber-dojo/web PR #427](https://github.com/cyber-dojo/web/pull/427), blog).
- Transition data: red→green averages ~5.4 changed lines; big edits (13+) tend to go amber ([blog 2014](https://blog.cyber-dojo.org/2014/08/traffic-light-transitions.html)) — supports "step size" heuristics.
- No judgement of step quality; review is human (diff per light).

### murex/TCR — https://github.com/murex/TCR (MIT, Go single binary, Linux/macOS/Windows)

- test && commit || revert; built-in YAML per language: source/test file dirs + regex patterns, toolchain build/test commands per OS/arch, `test-result-dir` for JUnit XML ([src/language/built-in/go.yml](https://github.com/murex/TCR/blob/main/src/language/built-in/go.yml), [src/toolchain/built-in/go-tools.yml](https://github.com/murex/TCR/blob/main/src/toolchain/built-in/go-tools.yml)). Separate build step (`go test -count=0`) distinguishes compile failure from test failure.
- Uses fsnotify; xunit parser in `src/xunit/`; `tcr stats`/`log`; web UI with driver/navigator roles + timer.

### Minor / visual-only

- JetBrains "Red Green Refactor – Learn TDD Cycle": "Visualisation for TDD Cycle Red-Green-Refactor", 645 downloads, source last pushed 2021 ([marketplace API](https://plugins.jetbrains.com/api/plugins/13784), [repo](https://github.com/gauravmi/red-green-refactor)).
- TDD Studio: continuous build/test + coverage glyphs in Visual Studio, .NET only ([repo](https://github.com/tddstud10/tddstud10)).
- tdd-bdd-commit: `commit red|green|refactor` wrappers ([repo](https://github.com/matatk/tdd-bdd-commit)).
- AI workflows (VS Code custom agents per phase, Claude Code TDD skills) *instruct* an agent to do TDD; they don't judge a human ([VS Code guide](https://code.visualstudio.com/docs/agents/guides/test-driven-development-guide)).
- TPP: no analyzer/linter found; only examples (e.g. [kuerm/tpp_by_example](https://github.com/kuerm/tpp_by_example)).

## What to reuse / learn

1. **Rules text**: TDD Guard `rules.ts` + Probity `DEFAULT_TDD_RULES` are a vetted, MIT-licensed spec of Red/Green/Refactor violations incl. the "clean red" stub ladder and refactor allowances — seed our judge criteria and eval cases from them.
2. **Fail reason = amber vs red**: cyber-dojo and TCR both separate "didn't run/compile" from "assertion failed". Do this deterministically (runner exit/structured output), not with the model.
3. **Normalised test-result schema**: adopt something like TDD Guard's `{tests[]: name, state, errors[]}`; ingest `go test -json`, pytest/Jest JUnit XML (TCR's `xunit` package is Go + MIT — candidate to reuse).
4. **Per-language YAML config** (TCR): test cmd per OS, source/test globs, build vs test commands — matches our per-project config idea; TCR's built-in list doubles as auto-detect defaults.
5. **Step inference**: Zorro/WatchDog show episode = events between passing runs, classified by rules over (test edit, prod edit, compile error, fail, pass). Use the lenient automaton — strict TDD is rare (2.2%) and compile errors legitimately mix prod edits into red.
6. **Deterministic first, model second**: Probity's AST new-test count (ast-grep) handles "one test at a time" cheaply; reserve the model for over-implementation, TPP, refactor quality.
7. **Judge the delta, allow transient states, high bar for refactor nags** (Probity) — lowers noise, important for an advisory tool.
8. **Learner override** (Besouro "disagree") and **prediction** (cyber-dojo) are proven pedagogic hooks.

## Gap this tool fills

- All LLM judges (TDD Guard, Probity) target **AI agents**, are **blocking**, hook into agent transcripts/tools, and need **cloud Claude/vendor SDKs**. None coaches a **human** at an editor-agnostic file-watch level, none runs a **local/small model**.
- Heuristic classifiers (Zorro/Besouro, WatchDog) did human step inference but are **Java/IDE-bound, dead**, and judge only process order — not test size, fail-reason quality, TPP simplicity or refactor quality.
- cyber-dojo/TCR are language-agnostic but judge only pass/fail colour (TCR gatekeeps; out of scope for us).
- **No tool judges TPP** (simplest transformation) at all.
- Gap: advisory, cross-platform single binary, file-watching, language-agnostic via config, inferring R/G/R steps for a human, judging per-step quality (size, fail reason, TPP, refactor) with a local model and an end-of-session report.
