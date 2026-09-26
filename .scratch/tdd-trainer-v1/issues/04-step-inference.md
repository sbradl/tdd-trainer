# Step inference model

Type: grilling
Status: resolved
Blocked by: none
Map: ../map.md

## Question

How does the coach bound and classify steps from a live file watcher? When are tests run (every save, debounced, on runner exit)? How are before/after snapshots and diffs kept (shadow git repo, in-memory, other)? How are ambiguous sequences handled: multiple tests added at once, production code edited during red, tests edited during green, a test that passes immediately, broken builds? How does the learner override the inferred phase?

## Answer

- **Test trigger**: coach runs the configured test command itself on debounced save (~300–500 ms); a newer save cancels the in-flight run. Learner never runs tests manually. (Slow suites → runner contract ticket.)
- **Snapshots**: shadow git repo in `.tddtrainer/` (own git dir, work tree = project), via embedded pure-Go git lib; no dependency on system git, never touches learner's repo. Feeds diffs + session report.
- **Test state** = set of failing tests, or "build broken". A **step** is all saves between two test-state transitions, judged at the transition:
  - all green → exactly one new failing test = Red complete
  - failing → all green = Green complete
  - green → green with code changed = Refactor (accumulates until next Red begins)
- **Compile errors**: build broken is "red in progress", not a Red. Red completes only when the new test fails on an assertion (learner adds minimal stub). Hint mentions this.
- **Anomalies** (advisory verdict, never block): multiple new tests in one step; new test passes immediately; production code changed without a failing test; test edited during Green; tests went red during Refactor.
- **Baseline**: session starting with failing tests = red in progress + warning.
- **Override**: TUI hotkeys set phase (`r`/`g`/`f`) and reset baseline (`b`).
- **Test refactoring**: allowed while green, judged like production refactoring.
