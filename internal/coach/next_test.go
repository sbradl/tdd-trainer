package coach

import (
	"strings"
	"sync"
	"testing"

	"github.com/sbradl/tdd-trainer/internal/judge"
	"github.com/sbradl/tdd-trainer/internal/steps"
)

func covered(answers map[string]string) map[string]judge.Verdict {
	vs := map[string]judge.Verdict{}
	for id, a := range answers {
		vs["covers-"+id] = judge.Verdict{Gate: "covers-" + id, Answer: a}
	}
	return vs
}

func TestFindGapTakesTheFirstSureGapInZombiesOrder(t *testing.T) {
	cases := []struct {
		answers  map[string]string
		gap      string
		complete bool
	}{
		{map[string]string{"zero": "yes", "one": "yes", "many": "no", "boundary": "no", "error": "no"}, "many", false},
		{map[string]string{"zero": judge.Uncertain, "one": "yes", "many": "yes", "boundary": "no", "error": "na"}, "boundary", false},
		{map[string]string{"zero": "yes", "one": "yes", "many": "yes", "boundary": "yes", "error": "na"}, "", true},
		{map[string]string{"zero": "yes", "one": "yes", "many": judge.Uncertain, "boundary": "yes", "error": "yes"}, "", false},
	}
	for _, c := range cases {
		cov := findGap(covered(c.answers))
		if cov.gap != c.gap || cov.complete != c.complete {
			t.Errorf("%v: got gap %q complete %v, want %q %v", c.answers, cov.gap, cov.complete, c.gap, c.complete)
		}
	}
}

func TestRenderNext(t *testing.T) {
	gap := coverage{gap: "boundary", tpp: "selection"}
	cases := []struct {
		name     string
		stage    int
		g        greenSignals
		cov      coverage
		refactor string
		id       string
		want     []string
		not      []string
	}{
		{"stage 1 names the kind", 1, greenSignals{}, gap, "", "boundary", []string{"Try an edge case."}, []string{"selection", "Unconditional"}},
		{"stage 2 names the case and the transformation", 2, greenSignals{}, gap, "", "boundary",
			[]string{"No test yet at an edge", "probably needs Unconditional → Selection (4/8)."}, nil},
		{"stages stop at 2", 3, greenSignals{}, gap, "", "boundary", []string{"No test yet at an edge"}, nil},
		{"fake-it means triangulate first", 1, greenSignals{fake: true}, gap, "", Triangulate, []string{"Triangulate"}, nil},
		{"triangulate in detail", 2, greenSignals{fake: true}, gap, "", Triangulate, []string{"different expected result"}, []string{"Selection"}},
		{"a big jump is noted in detail", 2, greenSignals{jump: true}, gap, "", "boundary", []string{"closer to the current code"}, nil},
		{"but not at stage 1", 1, greenSignals{jump: true}, gap, "", "boundary", nil, []string{"closer"}},
		{"all covered", 1, greenSignals{}, coverage{complete: true}, "", Complete, []string{"Looks complete"}, nil},
		{"nothing sure", 2, greenSignals{}, coverage{}, "", "", []string{"No clear gap"}, nil},
		{"an open refactor comes first", 1, greenSignals{}, gap, "Magic numbers.", "boundary", []string{"Refactor now is still open: Magic numbers."}, nil},
	}
	for _, c := range cases {
		n := renderNext(c.stage, c.g, c.cov, "iteration-first", c.refactor)
		if n.Case != c.id || n.Pending || n.Stage < 1 || n.Stage > 2 {
			t.Errorf("%s: got %+v", c.name, n)
		}
		for _, w := range c.want {
			if !strings.Contains(n.Text, w) {
				t.Errorf("%s: %q lacks %q", c.name, n.Text, w)
			}
		}
		for _, w := range c.not {
			if strings.Contains(n.Text, w) {
				t.Errorf("%s: %q has %q", c.name, n.Text, w)
			}
		}
	}
}

func TestSignalsFromTheLastGreen(t *testing.T) {
	if s := signals([]Verdict{{Check: "tpp", Answer: "constant"}}); !s.fake || s.jump {
		t.Errorf("constant: %+v", s)
	}
	if s := signals([]Verdict{{Check: "tpp", Answer: "selection"}, {Check: "multi", Answer: "yes"}}); s.fake || !s.jump {
		t.Errorf("multi: %+v", s)
	}
	if s := signals([]Verdict{{Check: "step-size", Level: Hint}}); !s.jump {
		t.Errorf("step-size hint: %+v", s)
	}
	if s := signals([]Verdict{{Check: "tpp", Level: Uncertain}}); s.fake || s.jump {
		t.Errorf("uncertain: %+v", s)
	}
}

type nextSink struct {
	mu  sync.Mutex
	got []NextTest
}

func (s *nextSink) add(n NextTest) { s.mu.Lock(); s.got = append(s.got, n); s.mu.Unlock() }
func (s *nextSink) last() NextTest {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.got) == 0 {
		return NextTest{}
	}
	return s.got[len(s.got)-1]
}

// nextKata: Roman(1) and Roman(2) pass with an if; the coverage gates see
// no "many" test yet.
func nextKata(t *testing.T) (*kata, *scripted, *nextSink) {
	sc := &scripted{evidence: map[string]string{}, answers: map[string]string{
		"red-check": "yes", "one-behaviour": "yes", "tpp": "selection", "step-size": "simple", "multi": "no", "cheating": "no",
		"covers-zero": "na", "covers-one": "yes", "covers-many": "no", "covers-boundary": "no", "covers-error": "no",
		"next-tpp": "iteration",
	}}
	k := newKata(t, sc)
	sink := &nextSink{}
	k.coach.SetNextTestSink(sink.add)
	k.write("kata.go", "package kata\n\nfunc Roman(n int) string { return \"I\" }\n")
	k.write("kata_test.go", header+test("TestOne", "1", "I"))
	k.run("TestOne")
	k.write("kata_test.go", header+test("TestOne", "1", "I")+test("TestTwo", "2", "II"))
	k.run("TestOne TestTwo", "TestTwo")
	k.write("kata.go", "package kata\n\nfunc Roman(n int) string {\n\tif n == 2 {\n\t\treturn \"II\"\n\t}\n\treturn \"I\"\n}\n")
	k.run("TestOne TestTwo")
	return k, sc, sink
}

func (k *kata) greenVerdicts(n int) []Verdict {
	k.mu.Lock()
	defer k.mu.Unlock()
	var out []Verdict
	for _, v := range k.got {
		if v.Step == n {
			out = append(out, v)
		}
	}
	return out
}

func TestNextTestAfterGreenRevealsInStages(t *testing.T) {
	k, sc, sink := nextKata(t)
	k.drain() // coverage is judged right after the Green, before anyone asks
	for _, g := range coverGates {
		if !strings.Contains(sc.evidence[g], "=== Test ===\npackage kata") || !strings.Contains(sc.evidence[g], "func TestTwo") ||
			!strings.Contains(sc.evidence[g], "=== Current source ===\npackage kata") || !strings.Contains(sc.evidence[g], "if n == 2") {
			t.Fatalf("%s evidence:\n%s", g, sc.evidence[g])
		}
	}
	asked := len(sc.order)

	if err := k.coach.NextTest(k.prev, steps.PhaseRefactor, k.greenVerdicts(2)); err != nil {
		t.Fatal(err)
	}
	if n := sink.last(); n.Stage != 1 || n.Case != "many" || n.Text != "Try more than one." {
		t.Fatalf("stage 1: %+v", n)
	}
	k.coach.NextTest(k.prev, steps.PhaseRefactor, k.greenVerdicts(2))
	if n := sink.last(); n.Stage != 2 || !strings.Contains(n.Text, "several items") || !strings.Contains(n.Text, "Selection → Iteration") {
		t.Fatalf("stage 2: %+v", n)
	}
	if len(sc.order) != asked {
		t.Errorf("asking again re-judged: %v", sc.order[asked:])
	}

	// refactoring keeps the coverage and the stage
	k.write("kata.go", "package kata\n\nfunc Roman(n int) string {\n\tif n == 2 {\n\t\treturn \"II\"\n\t}\n\treturn \"I\" // one\n}\n")
	k.run("TestOne TestTwo")
	k.coach.NextTest(k.prev, steps.PhaseRefactor, k.greenVerdicts(2))
	if n := sink.last(); n.Stage != 2 || n.Pending {
		t.Fatalf("after a refactoring: %+v", n)
	}

	// a new test starts over
	k.write("kata_test.go", header+test("TestOne", "1", "I")+test("TestTwo", "2", "II")+test("TestThree", "3", "III"))
	k.run("TestOne TestTwo TestThree", "TestThree")
	k.coach.NextTest(k.prev, steps.PhaseGreen, nil)
	if n := sink.last(); n.Stage != 0 || !strings.Contains(n.Text, "make the failing test pass") {
		t.Fatalf("during Green: %+v", n)
	}
}

func TestNextTestWaitsForTheJudge(t *testing.T) {
	k, _, sink := nextKata(t)
	k.coach.NextTest(k.prev, steps.PhaseRefactor, nil)
	if n := sink.last(); !n.Pending || n.Stage != 1 {
		t.Fatalf("before judging: %+v", n)
	}
	k.drain()
	if n := sink.last(); n.Pending || n.Case != "many" || n.Stage != 1 {
		t.Fatalf("after judging: %+v", n)
	}
}

func TestNextTestAfterFakeItSaysTriangulate(t *testing.T) {
	k, _, sink := nextKata(t)
	k.drain()
	k.coach.NextTest(k.prev, steps.PhaseRefactor, []Verdict{{Check: "tpp", Level: OK, Answer: "constant"}})
	if n := sink.last(); n.Case != Triangulate {
		t.Fatalf("got %+v", n)
	}
}

func TestNextTestNeedsTheJudge(t *testing.T) {
	k := newKata(t, nil)
	sink := &nextSink{}
	k.coach.SetNextTestSink(sink.add)
	k.write("kata.go", "package kata\n")
	k.write("kata_test.go", header)
	k.run("")
	k.coach.NextTest(k.prev, steps.PhaseRefactor, nil)
	if n := sink.last(); n.Stage != 0 || !strings.Contains(n.Text, "judge") {
		t.Fatalf("got %+v", n)
	}
}

func TestNextTestNamesAnOpenRefactor(t *testing.T) {
	k := smellyKata(t)
	sink := &nextSink{}
	k.coach.SetNextTestSink(sink.add)
	k.coach.NextTest(k.prev, steps.PhaseRefactor, nil)
	k.drain()
	if n := sink.last(); !strings.Contains(n.Text, "Refactor now is still open: Repeated conditionals") {
		t.Fatalf("got %+v", n)
	}
}
