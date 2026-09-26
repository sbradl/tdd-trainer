# Judge eval prototype

Type: prototype
Status: resolved
Blocked by: 01, 03, 06
Map: ../map.md

## Question

Build a small eval set of good and bad TDD steps (Go, Python, TypeScript) and run them through candidate local judge(s). Are verdicts accurate and fast enough to be useful? Which criteria does a small local model handle, which need a cloud model or heuristics only?

Note: reference languages widened to Go, Python, TypeScript, .NET, Lua, Elixir (see [runner contract](05-runner-contract.md)); the eval set should cover them.

Note from [Heuristic vs judge split](06-heuristic-vs-judge.md): gates to evaluate are red-check, one-behaviour (new), tpp (remap to 8-item list), step-size (remap bands), cheating, structural, refactor-effect (new), review lenses. Relabel existing tpp fixtures.

## Answer

Prototype: branch `prototype/judge-eval`, dir `prototype-judge-eval/` (throwaway copy of the user's claude-skills tdd-judge, changed to the 2021 8-item TPP list; new gates `multi`, `one-behaviour`, `refactor-effect`; 88 fixtures across Go, Python, TypeScript, C#, Lua, Elixir). Results: `prototype-judge-eval/regress-out.txt`.

**Result: 66/88 confident and correct, 0 confident wrong.** Every miss is `uncertain`, which stays silent live.

| Gate | Result | Notes |
|---|---|---|
| red-check | 18/19 | solid in all 6 languages |
| step-size | 10/11 | |
| refactor-effect | 4/6 | improves/neutral right; worsens always uncertain |
| one-behaviour | 4/6 | "yes" right; "no" often uncertain |
| cheating | 4/6 | |
| tpp | 13/19 | constant, selection, recursion, mutation and iteration (explicit loops) fine; variable 0/2; list, `{}`→Nil in Lua and Elixir clause uncertain |
| structural | 3/6 | |
| multi | 2/5 | never confidently detects "yes" |
| review lenses | 7/10 | unchanged |

Latency (Qwen3.5-4B Q4, CPU, Ryzen AI 7 350): about 7 s per gate with the model loaded; a cold single call takes 18 s, mostly model load. A green step (tpp + step-size + cheating + multi) takes about 30 s.

Decisions (user, 2026-09-26):
- **v1 policy**: local judge only by default; `uncertain` stays silent live and is listed in the session report. The weak gates just speak up less often.
- **Weak gates** (multi, structural, tpp variable/list): tune their questions and options during implementation against the fixtures (`--regress` before and after). (The cloud fallback originally decided here was later ruled out of scope for v1, see [Cloud judge and privacy](12-cloud-judge-privacy.md).)
- **Latency**: the model stays loaded for the whole session (a background process). Gates run in the background in order, cheapest and most useful first: red-check first, then the Green gates, the Refactor gates, and review lenses last.
- **Fixture labels**: iteration fixtures use explicit loops (LINQ `Count` and `sum(...)` were ambiguous, dropped). A fixture where a loop also mutates a variable counts as `multi`, not `tpp`.
- The fixture set is the seed of the product's judge regression suite.
