# TDD session report — 2026-09-26 19:29

| Duration | Cycles | Tests added | Clean cycles |
|---|---|---|---|
| 1m25s | 5 | 6 | 3 of 5 (60%) |

## Focus tips

1. **Refactor after Green** (2×): The last Green (step 9) left something to refactor: special-case code mixed into general code, and magic numbers or strings. Clean it up before the next Red (tddt show 9).
2. **Cycle rhythm** (1×): Several new tests in one step: write one failing test at a time.
3. **No test-specific code** (1×): The code special-cases the test's inputs: generalise instead of matching test values.

## Cycles

### Cycle 1

| Step | Kind | Verdicts |
|---|---|---|
| 1 | Red — roman::TestOne | ✓ **One new test**: Exactly one new failing test: roman::TestOne.<br>✓ **Fails for the right reason**: It fails on its assertion (expected vs actual), so it proves the behaviour is missing.<br>✓ **One behaviour per test**: The test checks a single behaviour. |
| 2 | Green | ✓ **Transformation**: Applied Nil → Constant (2/8).<br>✓ **Simplest change**: The test needed a constant change and the code made one: no bigger than necessary.<br>✓ **One transformation**: One transformation, as a single new test should need. |

### Cycle 2

| Step | Kind | Verdicts |
|---|---|---|
| 3 | Red — roman::TestTwo | ✓ **One new test**: Exactly one new failing test: roman::TestTwo.<br>✓ **Fails for the right reason**: It fails on its assertion (expected vs actual), so it proves the behaviour is missing.<br>✓ **One behaviour per test**: The test checks a single behaviour. |
| 4 | Green | ✓ **Transformation**: Applied Unconditional → Selection (4/8).<br>✓ **Simplest change**: The test needed a simple change and the code made one: no bigger than necessary.<br>✓ **One transformation**: One transformation, as a single new test should need. |

### Cycle 3

| Step | Kind | Verdicts |
|---|---|---|
| 5 | Red — roman::TestThree | ✓ **One new test**: Exactly one new failing test: roman::TestThree.<br>✓ **Fails for the right reason**: It fails on its assertion (expected vs actual), so it proves the behaviour is missing.<br>✓ **One behaviour per test**: The test checks a single behaviour.<br>➜ **Refactor after Green**: The last Green (step 4) left something to refactor: special-case code mixed into general code, and magic numbers or strings. Clean it up before the next Red (tddt show 4). |
| 6 | Green | ✓ **Transformation**: Applied Unconditional → Selection (4/8).<br>✓ **Simplest change**: The test needed a simple change and the code made one: no bigger than necessary.<br>✓ **One transformation**: One transformation, as a single new test should need.<br>➜ **No test-specific code**: The code special-cases the test's inputs: generalise instead of matching test values. |
| 7 | Refactor | ✓ **Tests stayed green**: All tests kept passing during the refactoring.<br>✓ **Design effect**: The code is easier to read or change afterwards. |

<details><summary>Step 4 diff (the Green before step 5)</summary>

```diff
diff --git a/roman.go b/roman.go
index 847de54d816493d3b35bcab1609573c778c6e73c..b7dd9f3eef2d0f001b30f3d85913c5da81f4479c 100644
--- a/roman.go
+++ b/roman.go
@@ -1,5 +1,8 @@
 package roman
 
 func Roman(n int) string {
+	if n == 2 {
+		return "II"
+	}
 	return "I"
 }
```

</details>

<details><summary>Step 6 diff</summary>

```diff
diff --git a/roman.go b/roman.go
index b7dd9f3eef2d0f001b30f3d85913c5da81f4479c..b78881bb600593847ea199a9f3f0a3432d218d80 100644
--- a/roman.go
+++ b/roman.go
@@ -1,6 +1,9 @@
 package roman
 
 func Roman(n int) string {
+	if n == 3 {
+		return "III"
+	}
 	if n == 2 {
 		return "II"
 	}
```

</details>

### Cycle 4

| Step | Kind | Verdicts |
|---|---|---|
| 8 | Red — roman::TestFour | ✓ **One new test**: Exactly one new failing test: roman::TestFour.<br>✓ **Fails for the right reason**: It fails on its assertion (expected vs actual), so it proves the behaviour is missing.<br>✓ **One behaviour per test**: The test checks a single behaviour. |
| 9 | Green | ✓ **Transformation**: Applied Unconditional → Selection (4/8).<br>✓ **Simplest change**: The code made a simple change where the test seemed to need a complex one: check that it really generalises.<br>✓ **One transformation**: One transformation, as a single new test should need. |

### Cycle 5

| Step | Kind | Verdicts |
|---|---|---|
| 10 | Red — roman::TestFive, roman::TestTen | ⚠ **Cycle rhythm**: Several new tests in one step: write one failing test at a time.<br>✓ **Fails for the right reason**: It fails on its assertion (expected vs actual), so it proves the behaviour is missing.<br>➜ **Refactor after Green**: The last Green (step 9) left something to refactor: special-case code mixed into general code, and magic numbers or strings. Clean it up before the next Red (tddt show 9). |

<details><summary>Step 10 diff</summary>

```diff
diff --git a/roman_test.go b/roman_test.go
index 033d55aa20d44b69bc107b028e4dabc664fe5e65..e1921cef99638fb05524f492455450082dab4c6d 100644
--- a/roman_test.go
+++ b/roman_test.go
@@ -25,3 +25,15 @@ 	if got := Roman(4); got != "IV" {
 		t.Errorf("Roman(4) = %q, want %q", got, "IV")
 	}
 }
+
+func TestFive(t *testing.T) {
+	if got := Roman(5); got != "V" {
+		t.Errorf("Roman(5) = %q, want %q", got, "V")
+	}
+}
+
+func TestTen(t *testing.T) {
+	if got := Roman(10); got != "X" {
+		t.Errorf("Roman(10) = %q, want %q", got, "X")
+	}
+}
```

</details>

<details><summary>Step 9 diff (the Green before step 10)</summary>

```diff
diff --git a/roman.go b/roman.go
index bcb3948f98e70d4b8dfaa6137c826d0153e659cb..7dff669b7e065add3a63e10f89dd339ecbe461e1 100644
--- a/roman.go
+++ b/roman.go
@@ -3,5 +3,8 @@
 import "strings"
 
 func Roman(n int) string {
+	if n == 4 {
+		return "IV"
+	}
 	return strings.Repeat("I", n)
 }
```

</details>

## Transformation path

2: Nil → Constant (2/8) → 4: Unconditional → Selection (4/8) → 6: Unconditional → Selection (4/8) → 9: Unconditional → Selection (4/8)

## Anomalies and missed refactors

- Step 5 (Red): The last Green (step 4) left something to refactor: special-case code mixed into general code, and magic numbers or strings. Clean it up before the next Red (tddt show 4).
- Step 10 (Red): Several new tests in one step: write one failing test at a time.
- Step 10 (Red): The last Green (step 9) left something to refactor: special-case code mixed into general code, and magic numbers or strings. Clean it up before the next Red (tddt show 9).

## Uncertain verdicts

The judge was not sure about these; they were not shown during the session.

- Step 2 (Green) No test-specific code: The judge could not decide (best guess "no", p=0.69).
- Step 3 (Red) Refactor after Green: The judge could not decide whether the last Green left something to refactor (domain design, module design).
- Step 4 (Green) No test-specific code: The judge could not decide (best guess "yes", p=0.79).
- Step 7 (Refactor) Behaviour unchanged: The judge could not decide (best guess "no", p=0.60).
- Step 9 (Green) No test-specific code: The judge could not decide (best guess "yes", p=0.60).

## What the checks mean

- **Behaviour unchanged**: A refactoring changes structure only, never what the code computes.
- **Cycle rhythm**: Steps that break the Red → Green → Refactor rhythm.
- **Design effect**: Whether the refactoring made the code easier to read and change.
- **Fails for the right reason**: A new test must fail on its assertion (expected vs actual), not on a compile error or crash; only then does it prove the behaviour is missing.
- **No test-specific code**: The code must not special-case the tests' exact inputs, beyond a first fake-it step.
- **One behaviour per test**: Each test checks one behaviour, so the Green step that follows stays small.
- **One new test**: A Red step adds exactly one failing test, so each cycle drives one small change.
- **One transformation**: A Green applies one transformation; several at once suggest a test is missing in between.
- **Refactor after Green**: After a Green, the code is reviewed for things worth cleaning up before the next test.
- **Simplest change**: Compares the transformation the Green applied with the simplest one the failing test needed.
- **Tests stayed green**: Refactoring keeps every test passing.
- **Transformation**: Which Transformation Priority Premise step the Green applied. Earlier ones (constant, then variable, then condition, loop, …) are simpler and preferred.

Full diffs: `tddt show <step>`.
