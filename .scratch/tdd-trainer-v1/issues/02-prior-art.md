# Prior art: TDD coaching tools

Type: research
Status: resolved
Blocked by: none
Map: ../map.md

## Question

What existing tools judge or enforce TDD practice (e.g. TDD Guard, cyber-dojo, TCR tools, kata coaches, IDE plugins)? For each: what they judge, how (heuristics vs LLM), how they detect steps, language support, setup effort. What can we reuse or learn, and what gap remains for this tool?

## Answer

Gist: LLM TDD judges exist (TDD Guard, its successor Probity) but target AI coding agents, block, and need cloud Claude/vendor SDKs. Human-facing step classifiers (Zorro/Besouro, WatchDog) are Java/IDE-bound and dead; cyber-dojo/TCR are language-agnostic but only judge pass/fail colour. No tool judges TPP. Gap confirmed: advisory, editor-agnostic, file-watching coach for humans with a local model judging size, fail reason, TPP, refactor.

Key takeaways:
- Reuse TDD Guard/Probity rules text (MIT) as judge criteria + eval seeds: one test at a time, "clean red" stub ladder, judge the delta, allow transient states, high bar for refactor nags.
- Fail reason: separate "didn't compile/run" (amber) from assertion failure deterministically, as cyber-dojo and TCR do.
- Step inference: episode = events between passing runs (Zorro); use lenient automaton (WatchDog: strict TDD in only 2.2% of sessions).
- Config: TCR's per-language YAML (test cmd per OS, source/test globs, build vs test, JUnit XML) is Go + MIT — model/reuse for config + auto-detect.
- Deterministic first (AST new-test count, à la Probity), model only for over-implementation, TPP, refactor quality.
- Learner override (Besouro "disagree") + prediction (cyber-dojo) are proven hooks.

Details: [../research/prior-art.md](../research/prior-art.md)
