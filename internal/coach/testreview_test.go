package coach

import (
	"strings"
	"testing"

	"github.com/sbradl/tdd-trainer/internal/judge"
)

// duplicatedTests is a kata whose tests repeat the same if-check; the
// scripted judge calls that duplication while two tests share it.
func duplicatedTests(t *testing.T) *kata {
	sc := &scripted{evidence: map[string]string{}, answers: map[string]string{
		"red-check": "yes", "one-behaviour": "yes", "tpp": "selection", "step-size": "simple", "multi": "no", "cheating": "no",
		"test-names": "no", "test-logic": "no", "test-message": "no",
	}, answer: func(gate, state string) string {
		if gate == "test-duplicated" {
			if strings.Count(state, "if got := Roman(") >= 2 {
				return "yes"
			}
			return "no"
		}
		return ""
	}}
	k := newKata(t, sc)
	k.write("kata.go", "package kata\n\nfunc Roman(n int) string { return \"I\" }\n")
	k.write("kata_test.go", header+test("TestOne", "1", "I"))
	k.run("TestOne")
	k.write("kata_test.go", header+test("TestOne", "1", "I")+test("TestTwo", "2", "II"))
	k.run("TestOne TestTwo", "TestTwo")
	k.write("kata.go", "package kata\n\nimport \"strings\"\n\nfunc Roman(n int) string { return strings.Repeat(\"I\", n) }\n")
	k.run("TestOne TestTwo")
	k.drain()
	return k
}

const helperTests = header + `
func TestOne(t *testing.T) { assertRoman(t, 1, "I") }

func TestTwo(t *testing.T) { assertRoman(t, 2, "II") }

func assertRoman(t *testing.T, n int, want string) {
	t.Helper()
	if got := Roman(n); got != want {
		t.Fatalf("Roman(%d) = %q, want %q", n, got, want)
	}
}
`

func TestTestReviewRightAfterGreen(t *testing.T) {
	k := duplicatedTests(t)
	v := k.last(2, testOpportunity)
	if v == nil || v.Level != Hint || !strings.HasPrefix(v.Text, "Worth refactoring kata_test.go now") || !strings.Contains(v.Text, "Duplicated test code") {
		t.Fatalf("got %+v", v)
	}
	sc := k.coach.scorer.(*scripted)
	if ev := sc.evidence["test-duplicated"]; !strings.Contains(ev, "=== Test ===\npackage kata") || !strings.Contains(ev, "func TestTwo") || strings.Contains(ev, "strings.Repeat") {
		t.Errorf("the test review sees all tests and only tests:\n%s", ev)
	}
}

func TestRefactoringTheTestsResolvesTheHint(t *testing.T) {
	k := duplicatedTests(t)
	k.write("kata_test.go", helperTests)
	k.run("TestOne TestTwo")
	k.coach.RecheckTests(k.prev)
	k.drain()
	if v := k.last(2, testOpportunity); v == nil || v.Level != OK || !strings.HasPrefix(v.Text, "Resolved: your refactoring removed duplicated test code") {
		t.Fatalf("got %+v", v)
	}
	k.write("kata_test.go", helperTests+"\nfunc TestThree(t *testing.T) { assertRoman(t, 3, \"III\") }\n")
	k.run("TestOne TestTwo TestThree", "TestThree")
	k.drain()
	if v := k.last(4, missedTest); v == nil || v.Level != OK || !strings.Contains(v.Text, "cleaned up the tests after step 2") {
		t.Fatalf("got %+v", v)
	}
}

func TestSkippingTheTestRefactorIsMissed(t *testing.T) {
	k := duplicatedTests(t)
	k.write("kata_test.go", header+test("TestOne", "1", "I")+test("TestTwo", "2", "II")+test("TestThree", "3", "III"))
	k.run("TestOne TestTwo TestThree", "TestThree")
	k.drain()
	v := k.last(3, missedTest)
	if v == nil || v.Level != Hint || !strings.Contains(v.Text, "without cleaning up kata_test.go after step 2") {
		t.Fatalf("got %+v", v)
	}
}

func TestSmellTextNamesSeveralProblemsShort(t *testing.T) {
	var found []judge.TestSmell
	for _, s := range judge.TestSmells {
		if s.ID == "test-duplicated" || s.ID == "test-message" {
			found = append(found, s)
		}
	}
	got := smellText(found)
	if got != "duplicated test code and silent failures." {
		t.Errorf("got %q", got)
	}
}
