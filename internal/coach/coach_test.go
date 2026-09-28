package coach

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sbradl/tdd-trainer/internal/config"
	"github.com/sbradl/tdd-trainer/internal/judge"
	"github.com/sbradl/tdd-trainer/internal/results"
	"github.com/sbradl/tdd-trainer/internal/snapshot"
	"github.com/sbradl/tdd-trainer/internal/steps"
)

// scripted answers each gate with a fixed option at p=0.95 and records
// the evidence it saw. answer, if set, overrides answers per evidence.
type scripted struct {
	mu       sync.Mutex
	answers  map[string]string
	answer   func(gate, state string) string
	evidence map[string]string // gate -> state text
	order    []string
	block    chan struct{} // if set, Score waits for it
}

func (s *scripted) Score(ctx context.Context, state string, gates []judge.Gate) ([][]float64, error) {
	if s.block != nil {
		select {
		case <-s.block:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var out [][]float64
	for _, g := range gates {
		s.evidence[g.Name] = state
		s.order = append(s.order, g.Name)
		ans := s.answers[strings.TrimSuffix(g.Name, "~clean")]
		if strings.HasPrefix(g.Name, "refactor-fixed") {
			// a refactoring to strings.Repeat removes the if-chain; anything else does not
			ans = "no"
			if strings.Contains(state, "+import \"strings\"") {
				ans = "yes"
			}
		}
		if s.answer != nil {
			if a := s.answer(g.Name, state); a != "" {
				ans = a
			}
		}
		if strings.HasPrefix(g.Name, "review-") && ans == "" {
			ans = "no"
		}
		probs := make([]float64, len(g.Options))
		for i, o := range g.Options {
			if o.ID == ans {
				probs[i] = 0.95
			} else {
				probs[i] = 0.05 / float64(len(g.Options)-1)
			}
		}
		out = append(out, probs)
	}
	return out, nil
}

var cfg = config.Config{Tests: []string{"**/*_test.go"}, Sources: []string{"**/*.go"}}

// kata drives the real snapshot store and step machine through a scripted
// Go Roman-numerals session and returns every verdict.
type kata struct {
	t       *testing.T
	root    string
	store   *snapshot.Store
	machine *steps.Machine
	coach   *Coach
	prev    snapshot.ID
	mu      sync.Mutex
	got     []Verdict
	n       int
	done    []steps.Step
}

func newKata(t *testing.T, sc judge.Scorer) *kata {
	k := &kata{t: t, root: t.TempDir(), machine: steps.New()}
	var err error
	if k.store, err = snapshot.Open(k.root, cfg); err != nil {
		t.Fatal(err)
	}
	k.coach = New(k.store, sc, "iteration-first", func(v Verdict) {
		k.mu.Lock()
		k.got = append(k.got, v)
		k.mu.Unlock()
	})
	return k
}

func (k *kata) write(rel, body string) {
	p := filepath.Join(k.root, rel)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		k.t.Fatal(err)
	}
	k.n++
	ts := time.Now().Add(time.Duration(k.n) * time.Second)
	os.Chtimes(p, ts, ts)
}

// run records a test run with the given tests and failures ("id" or "id!").
func (k *kata) run(tests string, failing ...string) {
	id, err := k.store.Snapshot("run")
	if err != nil {
		k.t.Fatal(err)
	}
	st := results.TestState{Tests: strings.Fields(tests)}
	sort.Strings(st.Tests)
	for _, f := range failing {
		r := results.ReasonAssertion
		if strings.HasSuffix(f, "?") {
			r = results.ReasonUndecided
		}
		id := strings.TrimSuffix(f, "?")
		st.Failing = append(st.Failing, results.Failure{ID: id, Reason: r, Message: "FAIL " + id + ": want I, got \"\""})
	}
	obs := steps.Observation{Snapshot: id, State: st}
	if k.prev != "" {
		ds, _ := k.store.Diff(k.prev, id)
		for _, d := range ds {
			obs.TestsChanged = obs.TestsChanged || d.Kind == config.Test
			obs.SourceChanged = obs.SourceChanged || d.Kind == config.Source
		}
	}
	k.prev = id
	for _, e := range k.machine.Observe(obs) {
		if sd, ok := e.(steps.StepDone); ok {
			k.done = append(k.done, sd.Step)
			if err := k.coach.Step(sd.Step); err != nil {
				k.t.Fatal(err)
			}
		}
	}
	if st.Green() && obs.SourceChanged && k.machine.Phase() == steps.PhaseRefactor {
		if err := k.coach.Recheck(id); err != nil {
			k.t.Fatal(err)
		}
	}
}

// last returns the newest verdict of a check for a step.
func (k *kata) last(step int, check string) *Verdict {
	k.mu.Lock()
	defer k.mu.Unlock()
	for i := len(k.got) - 1; i >= 0; i-- {
		if k.got[i].Step == step && k.got[i].Check == check {
			v := k.got[i]
			return &v
		}
	}
	return nil
}

func (k *kata) drain() []string {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { k.coach.Run(ctx); close(done) }()
	for deadline := time.Now().Add(5 * time.Second); k.coach.Pending() > 0; time.Sleep(5 * time.Millisecond) {
		if time.Now().After(deadline) {
			k.t.Fatal("judge queue did not drain")
		}
	}
	time.Sleep(20 * time.Millisecond) // let the last job's verdicts land
	cancel()
	<-done
	k.mu.Lock()
	defer k.mu.Unlock()
	var out []string
	for _, v := range k.got {
		out = append(out, fmt.Sprintf("%d %s %s:%s", v.Step, v.Kind, v.Check, v.Level))
	}
	return out
}

const header = "package kata\n\nimport \"testing\"\n"

func test(name, in, want string) string {
	return fmt.Sprintf("\nfunc %s(t *testing.T) {\n\tif got := Roman(%s); got != %q {\n\t\tt.Fatalf(\"want %s, got %%q\", got)\n\t}\n}\n", name, in, want, want)
}

func TestKataReplay(t *testing.T) {
	sc := &scripted{evidence: map[string]string{}, answers: map[string]string{
		"red-check": "yes", "one-behaviour": "yes",
		"tpp": "constant", "step-size": "constant", "multi": "no", "cheating": "no",
		"structural": "yes", "refactor-effect": "improves",
	}, answer: func(gate, state string) string {
		// the second Green adds a loop where one condition would do
		if strings.Contains(state, "+\tfor i") {
			return map[string]string{"tpp": "iteration", "step-size": "simple"}[gate]
		}
		return ""
	}}
	k := newKata(t, sc)
	k.write("kata.go", "package kata\n\nfunc Roman(n int) string { return \"\" }\n")
	k.write("kata_test.go", header)
	k.run("")

	// Red 1, Green 1 (constant)
	k.write("kata_test.go", header+test("TestOne", "1", "I"))
	k.run("TestOne", "TestOne")
	k.write("kata.go", "package kata\n\nfunc Roman(n int) string { return \"I\" }\n")
	k.run("TestOne")

	// Red 2 directly after Green: missed-refactor lenses run
	k.write("kata_test.go", header+test("TestOne", "1", "I")+test("TestTwo", "2", "II"))
	k.run("TestOne TestTwo", "TestTwo?")

	// Green 2: over-sized (iteration where one condition would do)
	k.write("kata.go", "package kata\n\nfunc Roman(n int) string {\n\ts := \"\"\n\tfor i := 0; i < n; i++ {\n\t\ts += \"I\"\n\t}\n\treturn s\n}\n")
	k.run("TestOne TestTwo")

	// Refactor, then Red 3 with two new tests
	k.write("kata.go", "package kata\n\nimport \"strings\"\n\nfunc Roman(n int) string { return strings.Repeat(\"I\", n) }\n")
	k.run("TestOne TestTwo")
	k.write("kata_test.go", header+test("TestOne", "1", "I")+test("TestTwo", "2", "II")+test("TestFour", "4", "IV")+test("TestFive", "5", "V"))
	k.run("TestOne TestTwo TestFour TestFive", "TestFour", "TestFive")

	got := k.drain()
	want := []string{
		// exact verdicts arrive first, in step order
		"1 Red new tests:ok", "1 Red red-check:ok",
		"3 Red new tests:ok",
		"5 Refactor stayed green:ok",
		"6 Red anomaly:warning", "6 Red red-check:ok",
		// judge verdicts by priority: red-check, Red, Green, Refactor, lenses
		"3 Red red-check:ok",
		"1 Red one-behaviour:ok", "3 Red one-behaviour:ok",
		"2 Green tpp:ok", "2 Green step-size:ok", "2 Green multi:ok", "2 Green cheating:ok",
		"4 Green tpp:ok", "4 Green step-size:hint", "4 Green multi:ok", "4 Green cheating:ok",
		// each Green is reviewed for refactoring; the next Red learns the result
		"3 Red missed refactor:ok", "2 Green refactor opportunity:ok",
		"3 Red missed test refactor:ok", "2 Green test refactor opportunity:ok",
		"6 Red missed refactor:ok", "4 Green refactor opportunity:ok",
		"6 Red missed test refactor:ok", "4 Green test refactor opportunity:ok",
		"5 Refactor structural:ok", "5 Refactor refactor-effect:ok",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got:\n%s\n\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	// evidence looks like the fixtures the judge was evaluated on
	ev := sc.evidence
	if !strings.Contains(ev["red-check"], "=== Test ===\nfunc TestTwo") || !strings.Contains(ev["red-check"], "=== Test-runner output ===\nFAIL TestTwo") {
		t.Errorf("red-check evidence:\n%s", ev["red-check"])
	}
	if strings.Contains(ev["red-check"], "TestOne") {
		t.Errorf("red-check evidence includes the old test:\n%s", ev["red-check"])
	}
	if !strings.Contains(ev["step-size"], "=== Current source ===\npackage kata\n\nfunc Roman(n int) string { return \"I\" }") {
		t.Errorf("step-size must see the source before Green:\n%s", ev["step-size"])
	}
	// the smallest change must keep the other tests green, so step-size
	// sees them all (a new-test-only view suggested "change the literal")
	for _, g := range []string{"step-size", "cheating"} {
		if !strings.Contains(ev[g], "func TestOne") || !strings.Contains(ev[g], "func TestTwo") {
			t.Errorf("%s must see all tests:\n%s", g, ev[g])
		}
	}
	if !strings.Contains(ev["tpp"], "+\tfor i := 0; i < n; i++ {") || strings.Contains(ev["tpp"], "func Test") {
		t.Errorf("tpp evidence:\n%s", ev["tpp"])
	}
	if !strings.Contains(ev["review-smells"], "+\tfor i := 0; i < n; i++ {") || strings.Contains(ev["review-smells"], "func Test") {
		t.Errorf("lenses must see the Green's source diff:\n%s", ev["review-smells"])
	}
}

func TestStepDoesNotBlockOnJudge(t *testing.T) {
	sc := &scripted{evidence: map[string]string{}, answers: map[string]string{}, block: make(chan struct{})}
	k := newKata(t, sc)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go k.coach.Run(ctx)
	k.write("kata.go", "package kata\n")
	k.write("kata_test.go", header)
	k.run("")
	start := time.Now()
	for i := 0; i < 5; i++ {
		k.write("kata_test.go", header+test(fmt.Sprintf("T%d", i), "1", "I"))
		k.run(strings.Repeat("x ", i)+fmt.Sprintf("T%d", i), fmt.Sprintf("T%d", i))
		k.write("kata.go", fmt.Sprintf("package kata\n\nvar x%d = 1\n", i))
		k.run(strings.Repeat("x ", i) + fmt.Sprintf("T%d", i))
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("steps took %v with a blocked judge", d)
	}
	if k.coach.Pending() == 0 {
		t.Fatal("want gates queued")
	}
	close(sc.block)
}

func TestNoJudgeMeansExactChecksOnly(t *testing.T) {
	k := newKata(t, nil)
	k.write("kata.go", "package kata\n")
	k.write("kata_test.go", header)
	k.run("")
	k.write("kata_test.go", header+test("TestOne", "1", "I"))
	k.run("TestOne", "TestOne?")
	if k.coach.Pending() != 0 {
		t.Fatal("queued gates without a judge")
	}
	if len(k.got) != 1 || k.got[0].Check != "new tests" {
		t.Fatalf("got %+v", k.got)
	}
}

func TestLongTestGetsSizeHint(t *testing.T) {
	k := newKata(t, nil)
	k.write("kata.go", "package kata\n")
	k.write("kata_test.go", header)
	k.run("")
	long := "\nfunc TestLong(t *testing.T) {\n" + strings.Repeat("\t_ = 1\n", 20) + "}\n"
	k.write("kata_test.go", header+long)
	k.run("TestLong", "TestLong")
	found := false
	for _, v := range k.got {
		found = found || (v.Check == "test size" && v.Level == Hint)
	}
	if !found {
		t.Fatalf("got %+v", k.got)
	}
}

func TestNewTestFileBoilerplateIsNotTestSize(t *testing.T) {
	k := newKata(t, nil)
	k.write("kata.go", "package kata\n")
	k.run("")
	file := "package kata\n\nimport (\n\t\"fmt\"\n\t\"slices\"\n\t\"testing\"\n)\n\n" +
		"func TestTable(t *testing.T) {\n\ttests := []struct{ n int }{\n\t\t{1},\n\t}\n\n" +
		"\tfor _, tt := range tests {\n\t\tt.Run(fmt.Sprint(tt.n), func(t *testing.T) {\n" +
		"\t\t\tif !slices.Equal(nil, []int{tt.n}) {\n\t\t\t\tt.Error(tt.n)\n\t\t\t}\n\t\t})\n\t}\n}\n"
	k.write("kata_test.go", file)
	k.run("TestTable", "TestTable")
	for _, v := range k.got {
		if v.Check == "test size" {
			t.Fatalf("package, imports and blank lines counted: %s", v.Text)
		}
	}
}

func TestStepSizeBands(t *testing.T) {
	c := New(nil, nil, "iteration-first", nil)
	cases := []struct {
		tpp, size string
		want      Level
	}{
		{"constant", "constant", OK},
		{"selection", "simple", OK},
		{"variable", "simple", OK},
		{"selection", "complex", OK},
		{"selection", "constant", Hint},
		{"iteration", "simple", Hint},
		{"recursion", "complex", OK},
		{"nil", "complex", OK},
	}
	for _, tc := range cases {
		vs := c.finishGreen(map[string]judge.Verdict{
			"tpp":       {Gate: "tpp", Answer: tc.tpp, P: 0.9},
			"step-size": {Gate: "step-size", Answer: tc.size, P: 0.9},
			"multi":     {Gate: "multi", Answer: "no", P: 0.9},
		})
		for _, v := range vs {
			if v.Check == "step-size" && v.Level != tc.want {
				t.Errorf("tpp %s, needed %s: got %v (%s)", tc.tpp, tc.size, v.Level, v.Text)
			}
		}
	}
}

func TestTPPLabelHonoursOrder(t *testing.T) {
	if got := New(nil, nil, "iteration-first", nil).tppLabel("iteration"); got != "Selection → Iteration (6/8)" {
		t.Errorf("got %q", got)
	}
	if got := New(nil, nil, "recursion-first", nil).tppLabel("recursion"); got != "Statement → Recursion (6/8)" {
		t.Errorf("got %q", got)
	}
}

// smellyKata: step 1 Red, step 2 Green that special-cases n == 2.
// The smells lens flags code containing a special case for 2.
func smellyKata(t *testing.T) *kata {
	sc := &scripted{evidence: map[string]string{}, answers: map[string]string{
		"red-check": "yes", "one-behaviour": "yes", "tpp": "selection", "step-size": "simple", "multi": "no", "cheating": "no",
		"structural": "yes", "refactor-effect": "improves", "review-smells~which": "repeated-switch",
	}, answer: func(gate, state string) string {
		if strings.HasPrefix(gate, "review-smells") && !strings.HasSuffix(gate, "~which") {
			if strings.Contains(state, "+\tif n == 2") {
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
	k.write("kata.go", "package kata\n\nfunc Roman(n int) string {\n\tif n == 2 {\n\t\treturn \"II\"\n\t}\n\treturn \"I\"\n}\n")
	k.run("TestOne TestTwo")
	k.drain()
	return k
}

func TestRefactorOpportunityRightAfterGreen(t *testing.T) {
	k := smellyKata(t)
	v := k.last(2, "refactor opportunity")
	if v == nil || v.Level != Hint || !strings.Contains(v.Text, "Repeated conditionals") || !strings.HasPrefix(v.Text, "Worth refactoring kata.go now, while the tests are green") {
		t.Fatalf("got %+v", v)
	}
}

func TestRefactoringResolvesTheOpportunity(t *testing.T) {
	k := smellyKata(t)
	k.write("kata.go", "package kata\n\nimport \"strings\"\n\nfunc Roman(n int) string { return strings.Repeat(\"I\", n) }\n")
	k.run("TestOne TestTwo")
	k.drain()
	if v := k.last(2, "refactor opportunity"); v == nil || v.Level != OK || !strings.HasPrefix(v.Text, "Resolved: your refactoring removed repeated conditionals") {
		t.Fatalf("got %+v", v)
	}
	k.write("kata_test.go", header+test("TestOne", "1", "I")+test("TestTwo", "2", "II")+test("TestThree", "3", "III"))
	k.run("TestOne TestTwo TestThree", "TestThree")
	k.drain()
	if v := k.last(4, "missed refactor"); v == nil || v.Level != OK || !strings.Contains(v.Text, "cleaned up after step 2") {
		t.Fatalf("got %+v", v)
	}
}

func TestRefactoringSomethingElseKeepsTheHint(t *testing.T) {
	k := smellyKata(t)
	k.write("kata.go", "package kata\n\n// Roman converts n.\nfunc Roman(n int) string {\n\tif n == 2 {\n\t\treturn \"II\"\n\t}\n\treturn \"I\"\n}\n")
	k.run("TestOne TestTwo")
	k.drain()
	if v := k.last(2, "refactor opportunity"); v == nil || v.Level != Hint || !strings.HasPrefix(v.Text, "Still there in kata.go after your last change") {
		t.Fatalf("got %+v", v)
	}
}

const loopRoman = "package kata\n\nfunc Roman(n int) string {\n\tout := \"\"\n\tfor i := 0; i < n; i++ {\n\t\tout += \"I\"\n\t}\n\treturn out\n}\n"

func TestLensesConfirmARefactoringTheFixQuestionMissed(t *testing.T) {
	k := smellyKata(t) // refactor-fixed says no without strings.Repeat
	k.write("kata.go", loopRoman)
	k.run("TestOne TestTwo")
	k.drain()
	if v := k.last(2, "refactor opportunity"); v == nil || v.Level != OK || !strings.HasPrefix(v.Text, "Resolved: your refactoring removed repeated conditionals") {
		t.Fatalf("got %+v", v)
	}
	sc := k.coach.scorer.(*scripted)
	if ev := sc.evidence["review-smells"]; !strings.Contains(ev, "+\tfor i := 0") || !strings.Contains(ev, "-func Roman(n int) string { return \"I\" }") {
		t.Errorf("lenses must see the Green's net change, from before the Green:\n%s", ev)
	}
}

func TestUnsureLensesMeanProbablyResolved(t *testing.T) {
	k := smellyKata(t)
	sc := k.coach.scorer.(*scripted)
	sc.answer = func(gate, state string) string {
		if strings.HasPrefix(gate, "review-smells") {
			return "?" // no option: uncertain
		}
		return ""
	}
	k.write("kata.go", loopRoman)
	k.run("TestOne TestTwo")
	k.drain()
	if v := k.last(2, "refactor opportunity"); v == nil || v.Level != OK || !strings.HasPrefix(v.Text, "Probably resolved:") {
		t.Fatalf("got %+v", v)
	}
	// resolved counts for the next Red
	k.write("kata_test.go", header+test("TestOne", "1", "I")+test("TestTwo", "2", "II")+test("TestThree", "3", "III"))
	k.run("TestOne TestTwo TestThree", "TestThree")
	k.drain()
	if v := k.last(4, "missed refactor"); v == nil || v.Level != OK {
		t.Fatalf("got %+v", v)
	}
}

// lensSees makes the smells lens still find something after the
// refactoring, and names it with which ("?": not clearly anything).
func lensSees(k *kata, which string) {
	sc := k.coach.scorer.(*scripted)
	sc.answer = func(gate, state string) string {
		switch gate {
		case "review-smells", "review-smells~clean":
			return "yes"
		case "review-smells~which":
			return which
		}
		return ""
	}
}

func TestRefactoringThatLeavesAnotherProblemSaysSo(t *testing.T) {
	k := smellyKata(t)
	lensSees(k, "dead-code")
	k.write("kata.go", loopRoman)
	k.run("TestOne TestTwo")
	k.drain()
	v := k.last(2, "refactor opportunity")
	if v == nil || v.Level != Hint || !strings.HasPrefix(v.Text, "Your refactoring removed repeated conditionals") || !strings.Contains(v.Text, "now finds in kata.go: Dead code.") {
		t.Fatalf("got %+v", v)
	}
	// the hint follows the new problem
	k.write("kata_test.go", header+test("TestOne", "1", "I")+test("TestTwo", "2", "II")+test("TestThree", "3", "III"))
	k.run("TestOne TestTwo TestThree", "TestThree")
	k.drain()
	if v := k.last(4, "missed refactor"); v == nil || v.Level != Hint || !strings.Contains(v.Text, "Dead code.") || strings.Contains(v.Text, "Repeated") {
		t.Fatalf("got %+v", v)
	}
}

func TestLensUnsureWhichProblemMeansProbablyResolved(t *testing.T) {
	k := smellyKata(t)
	lensSees(k, "?")
	k.write("kata.go", loopRoman)
	k.run("TestOne TestTwo")
	k.drain()
	if v := k.last(2, "refactor opportunity"); v == nil || v.Level != OK || !strings.HasPrefix(v.Text, "Probably resolved:") {
		t.Fatalf("got %+v", v)
	}
}

func TestSkippingTheRefactorIsMissed(t *testing.T) {
	k := smellyKata(t)
	k.write("kata_test.go", header+test("TestOne", "1", "I")+test("TestTwo", "2", "II")+test("TestThree", "3", "III"))
	k.run("TestOne TestTwo TestThree", "TestThree")
	k.drain()
	v := k.last(3, "missed refactor")
	if v == nil || v.Level != Hint || !strings.Contains(v.Text, "without cleaning up kata.go after step 2") || !strings.Contains(v.Text, "Repeated conditionals") {
		t.Fatalf("got %+v", v)
	}
}

func TestMissedRefactorWhenTheReviewIsStillRunning(t *testing.T) {
	// the next Red starts before the judge reviewed the Green
	sc := &scripted{evidence: map[string]string{}, answers: map[string]string{
		"review-smells": "yes", "review-smells~clean": "yes", "review-smells~which": "duplicated",
	}}
	k := newKata(t, sc)
	k.write("kata.go", "package kata\n\nfunc Roman(n int) string { return \"\" }\n")
	k.write("kata_test.go", header)
	k.run("")
	k.write("kata_test.go", header+test("TestOne", "1", "I"))
	k.run("TestOne", "TestOne")
	k.write("kata.go", "package kata\n\nfunc Roman(n int) string { return \"I\" }\n")
	k.run("TestOne")
	k.write("kata_test.go", header+test("TestOne", "1", "I")+test("TestTwo", "2", "II"))
	k.run("TestOne TestTwo", "TestTwo")
	k.drain()
	if v := k.last(3, "missed refactor"); v == nil || v.Level != Hint || !strings.Contains(v.Text, "Duplicated code") {
		t.Fatalf("got %+v", v)
	}
}

func TestWhereNamesTheProductionFiles(t *testing.T) {
	for files, want := range map[string]string{"": "the production code", "a.go": "a.go", "a.go b.go": "a.go and b.go", "a.go b.go c.go": "the production code"} {
		if got := where(strings.Fields(files)); got != want {
			t.Errorf("%q: got %q, want %q", files, got, want)
		}
	}
}

func TestProblemTextKeepsTheClearestTwo(t *testing.T) {
	w := func(top, answer string, p float64) judge.Verdict {
		return judge.Verdict{Top: top, Answer: answer, P: p}
	}
	got, _ := problemText([]string{"review-clean-code", "review-philosophy", "review-pragmatic", "review-smells"}, map[string]judge.Verdict{
		"review-clean-code~which": w("magic", "magic", 0.7),
		"review-philosophy~which": w("special-case", "special-case", 0.9),
		"review-pragmatic~which":  w("dry", "dry", 0.6),
		"review-smells~which":     w("duplicated", judge.Uncertain, 0.4),
	})
	if want := "special-case code mixed into general code, and magic numbers or strings."; got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}

func TestRefactoringBeforeTheReviewAnsweredIsRechecked(t *testing.T) {
	sc := &scripted{evidence: map[string]string{}, answers: map[string]string{"review-smells~which": "repeated-switch"},
		answer: func(gate, state string) string {
			if strings.HasPrefix(gate, "review-smells") && !strings.HasSuffix(gate, "~which") {
				if strings.Contains(state, "+\tif n == 2") {
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
	k.write("kata.go", "package kata\n\nfunc Roman(n int) string {\n\tif n == 2 {\n\t\treturn \"II\"\n\t}\n\treturn \"I\"\n}\n")
	k.run("TestOne TestTwo")
	// refactored before the judge ran at all
	k.write("kata.go", "package kata\n\nimport \"strings\"\n\nfunc Roman(n int) string { return strings.Repeat(\"I\", n) }\n")
	k.run("TestOne TestTwo")
	k.drain()
	if v := k.last(2, "refactor opportunity"); v == nil || v.Level != OK || !strings.HasPrefix(v.Text, "Resolved") {
		t.Fatalf("got %+v", v)
	}
}

// bigGreenKata: the first Green adds an if where a constant would do.
func bigGreenKata(t *testing.T) *kata {
	sc := &scripted{evidence: map[string]string{}, answers: map[string]string{
		"red-check": "yes", "one-behaviour": "yes", "step-size": "constant", "multi": "no", "cheating": "no",
	}, answer: func(gate, state string) string {
		if gate != "tpp" {
			return ""
		}
		if strings.Contains(state, "+\tif") {
			return "selection"
		}
		return "constant"
	}}
	k := newKata(t, sc)
	k.write("kata.go", "package kata\n\nfunc Roman(n int) string { return \"\" }\n")
	k.write("kata_test.go", header)
	k.run("")
	k.write("kata_test.go", header+test("TestOne", "1", "I"))
	k.run("TestOne", "TestOne")
	k.write("kata.go", "package kata\n\nfunc Roman(n int) string {\n\tif n == 1 {\n\t\treturn \"I\"\n\t}\n\treturn \"\"\n}\n")
	k.run("TestOne")
	k.drain()
	if v := k.last(2, "step-size"); v == nil || v.Level != Hint || strings.HasPrefix(v.Text, "Still") {
		t.Fatalf("want the Green's own simpler-change hint, got %+v", v)
	}
	return k
}

func TestSimplifyingAfterGreenResolvesTheStepSizeHint(t *testing.T) {
	k := bigGreenKata(t)
	k.write("kata.go", "package kata\n\nfunc Roman(n int) string { return \"I\" }\n")
	k.run("TestOne")
	k.drain()
	if v := k.last(2, "step-size"); v == nil || v.Level != OK || !strings.HasPrefix(v.Text, "Resolved:") {
		t.Fatalf("got %+v", v)
	}
}

func TestRefactoringThatKeepsTheBigChangeKeepsTheStepSizeHint(t *testing.T) {
	k := bigGreenKata(t)
	k.write("kata.go", "package kata\n\n// Roman converts n.\nfunc Roman(n int) string {\n\tif n == 1 {\n\t\treturn \"I\"\n\t}\n\treturn \"\"\n}\n")
	k.run("TestOne")
	k.drain()
	if v := k.last(2, "step-size"); v == nil || v.Level != Hint {
		t.Fatalf("got %+v", v)
	}
}

func TestSimplifyingBeforeTheGreenVerdictIsRechecked(t *testing.T) {
	sc := &scripted{evidence: map[string]string{}, answers: map[string]string{
		"red-check": "yes", "one-behaviour": "yes", "step-size": "constant", "multi": "no", "cheating": "no",
	}, answer: func(gate, state string) string {
		if gate == "tpp" && strings.Contains(state, "+\tif") {
			return "selection"
		}
		if gate == "tpp" {
			return "constant"
		}
		return ""
	}}
	k := newKata(t, sc)
	k.write("kata.go", "package kata\n\nfunc Roman(n int) string { return \"\" }\n")
	k.write("kata_test.go", header)
	k.run("")
	k.write("kata_test.go", header+test("TestOne", "1", "I"))
	k.run("TestOne", "TestOne")
	k.write("kata.go", "package kata\n\nfunc Roman(n int) string {\n\tif n == 1 {\n\t\treturn \"I\"\n\t}\n\treturn \"\"\n}\n")
	k.run("TestOne")
	k.write("kata.go", "package kata\n\nfunc Roman(n int) string { return \"I\" }\n")
	k.run("TestOne") // before the judge answered the Green
	k.drain()
	if v := k.last(2, "step-size"); v == nil || v.Level != OK || !strings.HasPrefix(v.Text, "Resolved:") {
		t.Fatalf("got %+v", v)
	}
}

func TestGeneralisingAfterGreenResolvesTheTestSpecificCodeHint(t *testing.T) {
	sc := &scripted{evidence: map[string]string{}, answers: map[string]string{
		"red-check": "yes", "one-behaviour": "yes", "tpp": "selection", "step-size": "simple", "multi": "no",
	}, answer: func(gate, state string) string {
		if gate != "cheating" {
			return ""
		}
		if strings.Contains(state, "n == 2") {
			return "yes"
		}
		return "no"
	}}
	k := newKata(t, sc)
	k.write("kata.go", "package kata\n\nfunc Roman(n int) string {\n\tif n == 3 {\n\t\treturn \"III\"\n\t}\n\treturn \"I\"\n}\n")
	k.write("kata_test.go", header+test("TestOne", "1", "I"))
	k.run("TestOne")
	k.write("kata_test.go", header+test("TestOne", "1", "I")+test("TestTwo", "2", "II"))
	k.run("TestOne TestTwo", "TestTwo")
	k.write("kata.go", "package kata\n\nfunc Roman(n int) string {\n\tif n == 3 {\n\t\treturn \"III\"\n\t}\n\tif n == 2 {\n\t\treturn \"II\"\n\t}\n\treturn \"I\"\n}\n")
	k.run("TestOne TestTwo")
	k.drain()
	if v := k.last(2, "cheating"); v == nil || v.Level != Hint || strings.HasPrefix(v.Text, "Still") {
		t.Fatalf("want the Green's own test-specific code hint, got %+v", v)
	}
	k.write("kata.go", "package kata\n\nimport \"strings\"\n\nfunc Roman(n int) string { return strings.Repeat(\"I\", n) }\n")
	k.run("TestOne TestTwo")
	k.drain()
	if v := k.last(2, "cheating"); v == nil || v.Level != OK || !strings.HasPrefix(v.Text, "Resolved:") {
		t.Fatalf("got %+v", v)
	}
}

// restart ends the session and resumes it in a new coach and machine, as
// tddt --resume does with the saved history.
func (k *kata) restart(sc judge.Scorer) {
	byStep := map[int][]Verdict{}
	k.mu.Lock()
	for _, v := range k.got {
		byStep[v.Step] = Upsert(byStep[v.Step], v)
	}
	k.got = nil
	k.mu.Unlock()
	k.coach = New(k.store, sc, "iteration-first", func(v Verdict) {
		k.mu.Lock()
		k.got = append(k.got, v)
		k.mu.Unlock()
	})
	k.machine = steps.New()
	k.machine.Resume(k.done)
	if err := k.coach.Resume(k.done, byStep); err != nil {
		k.t.Fatal(err)
	}
}

func TestResumedSessionResolvesTheOpenRefactorHint(t *testing.T) {
	k := smellyKata(t)
	k.restart(k.coach.scorer)
	k.write("kata.go", "package kata\n\nimport \"strings\"\n\nfunc Roman(n int) string { return strings.Repeat(\"I\", n) }\n")
	k.run("TestOne TestTwo")
	k.drain()
	if v := k.last(2, "refactor opportunity"); v == nil || v.Level != OK || !strings.HasPrefix(v.Text, "Resolved") {
		t.Fatalf("got %+v", v)
	}
}

func TestResumedSessionMissesTheSkippedRefactor(t *testing.T) {
	k := smellyKata(t)
	k.restart(k.coach.scorer)
	k.write("kata_test.go", header+test("TestOne", "1", "I")+test("TestTwo", "2", "II")+test("TestThree", "3", "III"))
	k.run("TestOne TestTwo TestThree", "TestThree")
	k.drain()
	if v := k.last(3, "missed refactor"); v == nil || v.Level != Hint {
		t.Fatalf("got %+v", v)
	}
}

func TestResumedSessionJudgesTheGreenAfterItsRed(t *testing.T) {
	sc := &scripted{evidence: map[string]string{}, answers: map[string]string{
		"red-check": "yes", "one-behaviour": "yes", "tpp": "constant", "step-size": "constant", "multi": "no", "cheating": "no",
	}}
	k := newKata(t, sc)
	k.write("kata.go", "package kata\n\nfunc Roman(n int) string { return \"\" }\n")
	k.write("kata_test.go", header)
	k.run("")
	k.write("kata_test.go", header+test("TestOne", "1", "I"))
	k.run("TestOne", "TestOne")
	k.drain()
	k.restart(sc)
	k.write("kata.go", "package kata\n\nfunc Roman(n int) string { return \"I\" }\n")
	k.run("TestOne")
	k.drain()
	if v := k.last(2, "cheating"); v == nil || v.Level != OK {
		t.Fatalf("the Green after a resumed Red gets all its checks, got %+v", v)
	}
	if !strings.Contains(sc.evidence["step-size"], "want I") {
		t.Fatalf("step-size did not see the Red's transcript: %q", sc.evidence["step-size"])
	}
}

func TestExactValueBranches(t *testing.T) {
	cases := []struct {
		src  string
		want int
	}{
		{"func Roman(n int) string {\n\tif n == 4 {\n\t\treturn \"IV\"\n\t}\n\treturn strings.Repeat(\"I\", n)\n}\n", 1},
		{"\tif n == 4 {\n\t\treturn \"IV\"\n\t}\n\tif n == 9 {\n\t\treturn \"IX\"\n\t}\n", 2},
		{"if (commands == \"R\") return \"0:0:E\";\nif (commands == \"RR\") return \"0:0:S\";\n", 2},
		{"    if p1 == 1:\n        return \"Fifteen-Love\"\n", 1},
		{"\tswitch n {\n\tcase 4:\n\t\treturn \"IV\"\n\tcase 9:\n\t\treturn \"IX\"\n\t}\n", 2},
		{"  def roman(4), do: \"IV\"\n  def roman(n), do: String.duplicate(\"I\", n)\n", 1},
		{"\tif pins < 0 || pins > allPins {\n", 0},
		{"\tif n != 0 {\n", 0},
		{"\tif n % 3 == 0 {\n\t\treturn \"Fizz\"\n\t}\n\tif n%5 == 0 {\n", 0}, // FizzBuzz rules, not test values
		{"    if len(cells) == 1:\n", 0},
		{"\tif n == 0 {\n\t\treturn \"\"\n\t}\n\tif s == \"\" {\n", 0}, // the zero/empty guard
		{"    if p1 == 3 and p2 == 3:\n        return \"Deuce\"\n", 1},
	}
	for _, c := range cases {
		if got := exactValueBranches(c.src); got != c.want {
			t.Errorf("exactValueBranches(%q) = %d, want %d", c.src, got, c.want)
		}
	}
}

func TestOneBranchForATestValueIsFakeIt(t *testing.T) {
	sc := &scripted{evidence: map[string]string{}, answers: map[string]string{
		"red-check": "yes", "one-behaviour": "yes", "tpp": "selection", "step-size": "simple", "multi": "no", "cheating": "yes",
	}}
	k := newKata(t, sc)
	k.write("kata.go", "package kata\n\nimport \"strings\"\n\nfunc Roman(n int) string { return strings.Repeat(\"I\", n) }\n")
	k.write("kata_test.go", header+test("TestOne", "1", "I"))
	k.run("TestOne")
	k.write("kata_test.go", header+test("TestOne", "1", "I")+test("TestFour", "4", "IV"))
	k.run("TestOne TestFour", "TestFour")
	k.write("kata.go", "package kata\n\nimport \"strings\"\n\nfunc Roman(n int) string {\n\tif n == 4 {\n\t\treturn \"IV\"\n\t}\n\treturn strings.Repeat(\"I\", n)\n}\n")
	k.run("TestOne TestFour")
	k.drain()
	if v := k.last(2, "cheating"); v == nil || v.Level != OK || !strings.Contains(v.Text, "fake-it") {
		t.Fatalf("first example of a new rule: want an OK fake-it note, got %+v", v)
	}
	k.write("kata_test.go", header+test("TestOne", "1", "I")+test("TestFour", "4", "IV")+test("TestNine", "9", "IX"))
	k.run("TestOne TestFour TestNine", "TestNine")
	k.write("kata.go", "package kata\n\nimport \"strings\"\n\nfunc Roman(n int) string {\n\tif n == 4 {\n\t\treturn \"IV\"\n\t}\n\tif n == 9 {\n\t\treturn \"IX\"\n\t}\n\treturn strings.Repeat(\"I\", n)\n}\n")
	k.run("TestOne TestFour TestNine")
	k.drain()
	if v := k.last(4, "cheating"); v == nil || v.Level != Hint {
		t.Fatalf("second example special-cased: want a hint, got %+v", v)
	}
}

func TestResumeLooksPastAGreenThatOnlyFixedABrokenTest(t *testing.T) {
	k := smellyKata(t)
	g := k.done[len(k.done)-1]
	// recorded by an earlier version: a test broke while refactoring, and
	// fixing it counted as a Green of its own
	k.done = append(k.done,
		steps.Step{N: 3, Kind: steps.AnomalyStep, Anomalies: []steps.Anomaly{steps.BrokeExistingTest}, From: g.To, To: g.To},
		steps.Step{N: 4, Kind: steps.Green, From: g.To, To: g.To})
	k.restart(k.coach.scorer)
	k.write("kata.go", "package kata\n\nimport \"strings\"\n\nfunc Roman(n int) string { return strings.Repeat(\"I\", n) }\n")
	k.run("TestOne TestTwo")
	k.drain()
	if v := k.last(2, "refactor opportunity"); v == nil || v.Level != OK || !strings.HasPrefix(v.Text, "Resolved") {
		t.Fatalf("got %+v", v)
	}
}

// specialCaseKata: the Green adds branches on exact values, and the
// module-design lens calls them special-case code.
func specialCaseKata(t *testing.T, green string) *kata {
	sc := &scripted{evidence: map[string]string{}, answers: map[string]string{
		"red-check": "yes", "one-behaviour": "yes", "tpp": "selection", "step-size": "simple", "multi": "no", "cheating": "no",
		"review-philosophy": "yes", "review-philosophy~which": "special-case",
	}}
	k := newKata(t, sc)
	k.write("kata.go", "package kata\n\nimport \"strings\"\n\nfunc Roman(n int) string { return strings.Repeat(\"I\", n) }\n")
	k.write("kata_test.go", header+test("TestOne", "1", "I"))
	k.run("TestOne")
	k.write("kata_test.go", header+test("TestOne", "1", "I")+test("TestFour", "4", "IV"))
	k.run("TestOne TestFour", "TestFour")
	k.write("kata.go", green)
	k.run("TestOne TestFour")
	k.drain()
	return k
}

func TestOneFakeItBranchIsNotSpecialCaseToRefactor(t *testing.T) {
	k := specialCaseKata(t, "package kata\n\nimport \"strings\"\n\nfunc Roman(n int) string {\n\tif n == 4 {\n\t\treturn \"IV\"\n\t}\n\treturn strings.Repeat(\"I\", n)\n}\n")
	if v := k.last(2, "refactor opportunity"); v == nil || v.Level != OK {
		t.Fatalf("got %+v", v)
	}
}

func TestSeveralSpecialCasesAreWorthRefactoring(t *testing.T) {
	k := specialCaseKata(t, "package kata\n\nimport \"strings\"\n\nfunc Roman(n int) string {\n\tif n == 4 {\n\t\treturn \"IV\"\n\t}\n\tif n == 9 {\n\t\treturn \"IX\"\n\t}\n\treturn strings.Repeat(\"I\", n)\n}\n")
	if v := k.last(2, "refactor opportunity"); v == nil || v.Level != Hint || !strings.Contains(v.Text, "Special-case code") {
		t.Fatalf("got %+v", v)
	}
}

func TestAnotherBranchPerExampleIsTestSpecific(t *testing.T) {
	// the judge can't decide; the branch count can
	sc := &scripted{evidence: map[string]string{}, answers: map[string]string{
		"red-check": "yes", "one-behaviour": "yes", "tpp": "selection", "step-size": "simple", "multi": "no", "cheating": "unsure",
	}}
	k := newKata(t, sc)
	four := "\tif n == 4 {\n\t\treturn \"IV\"\n\t}\n"
	five := "\tif n == 5 {\n\t\treturn \"V\"\n\t}\n"
	k.write("kata.go", "package kata\n\nimport \"strings\"\n\nfunc Roman(n int) string {\n"+four+"\treturn strings.Repeat(\"I\", n)\n}\n")
	k.write("kata_test.go", header+test("TestFour", "4", "IV"))
	k.run("TestFour")
	k.write("kata_test.go", header+test("TestFour", "4", "IV")+test("TestFive", "5", "V"))
	k.run("TestFour TestFive", "TestFive")
	k.write("kata.go", "package kata\n\nimport \"strings\"\n\nfunc Roman(n int) string {\n"+four+five+"\treturn strings.Repeat(\"I\", n)\n}\n")
	k.run("TestFour TestFive")
	k.drain()
	if v := k.last(2, "cheating"); v == nil || v.Level != Hint || !strings.Contains(v.Text, "2 test values") {
		t.Fatalf("got %+v", v)
	}
	// generalising removes the branches: resolved
	k.write("kata.go", "package kata\n\nfunc Roman(n int) string { return table(n) }\n\nfunc table(n int) string { return [...]string{\"\", \"I\", \"II\", \"III\", \"IV\", \"V\"}[n] }\n")
	k.run("TestFour TestFive")
	k.drain()
	if v := k.last(2, "cheating"); v == nil || v.Level == Hint {
		t.Fatalf("got %+v", v)
	}
}
