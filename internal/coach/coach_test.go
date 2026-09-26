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
			if err := k.coach.Step(sd.Step); err != nil {
				k.t.Fatal(err)
			}
		}
	}
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
		"1 Red one-behaviour:ok", "3 Red one-behaviour:ok", "6 Red one-behaviour:ok",
		"2 Green tpp:ok", "2 Green step-size:ok", "2 Green multi:ok", "2 Green cheating:ok",
		"4 Green tpp:ok", "4 Green step-size:hint", "4 Green multi:ok", "4 Green cheating:ok",
		"5 Refactor structural:ok", "5 Refactor refactor-effect:ok",
		"3 Red missed refactor:ok",
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
	if !strings.Contains(ev["review-smells"], `-func Roman(n int) string { return "" }`) {
		t.Errorf("lenses must see the previous Green's diff:\n%s", ev["review-smells"])
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

func TestMissedRefactorSaysWhatToRefactor(t *testing.T) {
	sc := &scripted{evidence: map[string]string{}, answers: map[string]string{
		"red-check": "yes", "one-behaviour": "yes", "tpp": "selection", "step-size": "simple", "multi": "no", "cheating": "no",
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
	var hint *Verdict
	for i, v := range k.got {
		if v.Check == "missed refactor" {
			if hint != nil {
				t.Fatalf("two missed-refactor verdicts: %+v", k.got)
			}
			hint = &k.got[i]
		}
	}
	if hint == nil || hint.Level != Hint || hint.Step != 3 {
		t.Fatalf("got %+v", hint)
	}
	for _, want := range []string{"step 2", "tddt show 2", "Duplicated code", "(code smells)"} {
		if !strings.Contains(hint.Text, want) {
			t.Errorf("hint %q misses %q", hint.Text, want)
		}
	}
	if !strings.Contains(sc.evidence["review-smells~which"], `+func Roman(n int) string { return "I" }`) {
		t.Errorf("which-probe evidence:\n%s", sc.evidence["review-smells~which"])
	}
}
