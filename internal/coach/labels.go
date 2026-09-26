package coach

import "strings"

// checkInfo names a check for people and says what it looks at and why.
type checkInfo struct{ label, about string }

var checks = map[string]checkInfo{
	"new tests":                 {"One new test", "A Red step adds exactly one failing test, so each cycle drives one small change."},
	"test size":                 {"Small test", "A short test (up to 15 added lines) keeps the next step small."},
	"red-check":                 {"Fails for the right reason", "A new test must fail on its assertion (expected vs actual), not on a compile error or crash; only then does it prove the behaviour is missing."},
	"one-behaviour":             {"One behaviour per test", "Each test checks one behaviour, so the Green step that follows stays small."},
	"tpp":                       {"Transformation", "Which Transformation Priority Premise step the Green applied. Earlier ones (constant, then variable, then condition, loop, …) are simpler and preferred."},
	"step-size":                 {"Simplest change", "Compares the transformation the Green applied with the simplest one the failing test needed."},
	"multi":                     {"One transformation", "A Green applies one transformation; several at once suggest a test is missing in between."},
	"cheating":                  {"No test-specific code", "The code must not special-case the tests' exact inputs, beyond a first fake-it step."},
	"stayed green":              {"Tests stayed green", "Refactoring keeps every test passing."},
	"structural":                {"Behaviour unchanged", "A refactoring changes structure only, never what the code computes."},
	"refactor-effect":           {"Design effect", "Whether the refactoring made the code easier to read and change."},
	"refactor opportunity":      {"Refactor now", "Right after a Green, the new code is reviewed for things worth cleaning up while the tests are green; the verdict updates as you refactor."},
	"missed refactor":           {"Refactor after Green", "After a Green, the code is reviewed for things worth cleaning up before the next test."},
	"test refactor opportunity": {"Refactor tests now", "Right after a Green, the test code is reviewed for duplication, unclear names, logic in tests and silent failures; the verdict updates as you refactor the tests."},
	"missed test refactor":      {"Refactor tests after Green", "After a Green, the test code is reviewed for things worth cleaning up before the next test."},
	"test-duplicated":           {"Test review: duplication", "Whether test bodies repeat the same steps a helper or a table of cases would remove."},
	"test-names":                {"Test review: names", "Whether every test name says which behaviour it checks."},
	"test-logic":                {"Test review: logic", "Whether a test computes its expected value instead of stating it."},
	"test-message":              {"Test review: failure messages", "Whether a failing assertion says what was expected and what came out."},
	"anomaly":                   {"Cycle rhythm", "Steps that break the Red → Green → Refactor rhythm."},
	"judge":                     {"Judge", "The local model that answers the checks the exact rules cannot decide."},
	"covers-zero":               {"Next test: zero/empty", "Whether a test covers the degenerate case, for next-test hints."},
	"covers-one":                {"Next test: one", "Whether a test covers a single element, for next-test hints."},
	"covers-many":               {"Next test: many", "Whether a test covers several elements, for next-test hints."},
	"covers-boundary":           {"Next test: boundaries", "Whether a test covers an edge where the rules switch, for next-test hints."},
	"covers-error":              {"Next test: errors", "Whether a test covers invalid input or a failure, for next-test hints."},
	"next-tpp":                  {"Next test: transformation", "The transformation the next test probably needs, for next-test hints."},
}

// Label is the human name of a check or judge gate.
func Label(check string) string {
	if c, ok := checks[check]; ok {
		return c.label
	}
	base, probe, _ := strings.Cut(check, "~")
	if c, ok := checks[base]; ok {
		return c.label
	}
	if t, ok := lensTitle[base]; ok {
		if probe == "which" {
			return "Review: which " + t + " problem"
		}
		return "Review: " + t
	}
	return check
}

// About explains what a check looks at and why.
func About(check string) string { return checks[check].about }

// Upsert adds v to a step's verdicts; a newer verdict of the same check
// replaces the older one (a re-checked refactor hint), except anomalies,
// of which a step can have several.
func Upsert(vs []Verdict, v Verdict) []Verdict {
	if v.Check != "anomaly" {
		for i := range vs {
			if vs[i].Check == v.Check {
				vs[i] = v
				return vs
			}
		}
	}
	return append(vs, v)
}
