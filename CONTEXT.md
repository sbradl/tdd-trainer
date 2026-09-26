# TDD Trainer

A coach that watches a learner practise test-driven development and judges whether each step of the red/green/refactor cycle was done well.

## Language

### People and sessions

**Learner**:
Anyone practising TDD who wants feedback on whether they are doing it correctly.
_Avoid_: User, student, developer

**Session**:
One continuous stretch of practice on one project, from start of watching to the end-of-session report.
_Avoid_: Kata run, exercise

### The cycle

**Step**:
One inferred phase of the TDD cycle — Red, Green or Refactor — bounded by test runs.
_Avoid_: Phase, stage

**Red step**:
A step that adds exactly one new failing test.

**Green step**:
A step that changes production code so the failing test passes.

**Refactor step**:
A step that changes code while all tests stay passing.

**Fail reason**:
Why a new test fails; it is the *right* reason when the failure comes from the missing behaviour, not from a compile error, typo or broken setup.

**Test state**:
The set of currently failing tests, or "build broken" when the tests cannot run at all.

**Red in progress**:
The state after a new test is written but before it fails on an assertion (e.g. it does not compile yet); not yet a completed Red step.

**Baseline**:
The test state and code snapshot a session starts from, and against which the first step is measured.

**Anomaly**:
A step that fits no clean Red, Green or Refactor pattern (e.g. several new tests at once, code changed without a failing test); it still gets a verdict.

**Transformation**:
One of the eight ordered code changes in the Transformation Priority Premise as given in *Clean Craftsmanship* (2021), e.g. Constant→Variable; earlier ones are simpler and preferred.
_Avoid_: Refactoring (a transformation changes behaviour, a refactoring does not)

**Missed refactor**:
A Green step followed directly by a new Red step although the code had something worth refactoring.

**Next-test hint**:
On request, the kind of test to write next: the first case of the ZOMBIES checklist (zero/empty, one, many, boundaries, errors) that no test covers yet, or triangulating after a fake-it Green. Revealed in stages: first the kind, then the case.
_Avoid_: Suggestion (that is #15's generated concrete test)

### Judging

**Judge**:
The component that evaluates a step and produces a verdict, backed by a local model.
_Avoid_: Reviewer, grader

**Verdict**:
The judge's advisory assessment of one step, with a short explanation and optional hint.
_Avoid_: Score, grade
