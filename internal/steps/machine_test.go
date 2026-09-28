package steps

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/sbradl/tdd-trainer/internal/results"
	"github.com/sbradl/tdd-trainer/internal/snapshot"
)

// run describes one observation compactly:
//
//	"a b | b"      tests a and b, b failing on an assertion
//	"a b | b!"     b failing for the wrong reason (errors)
//	"a b | b?"     b failing, reason undecided
//	"broken"       build broken
//	"? | x"        test IDs unknown, x failing
//
// plus which kinds of files changed since the previous run ("t", "s", "ts").
type run struct {
	state, changed string
}

func (r run) observation(i int) Observation {
	o := Observation{
		Snapshot:      snapshot.ID(fmt.Sprintf("s%d", i)),
		TestsChanged:  strings.Contains(r.changed, "t"),
		SourceChanged: strings.Contains(r.changed, "s"),
	}
	if r.state == "broken" {
		o.State.BuildBroken = true
		return o
	}
	tests, failing, _ := strings.Cut(r.state, "|")
	if strings.TrimSpace(tests) != "?" { // "?": IDs unknown (exit code only)
		o.State.Tests = strings.Fields(tests)
		if o.State.Tests == nil {
			o.State.Tests = []string{}
		}
		slices.Sort(o.State.Tests)
	}
	for _, f := range strings.Fields(failing) {
		reason := results.ReasonAssertion
		switch {
		case strings.HasSuffix(f, "!"):
			reason = results.ReasonWrong
		case strings.HasSuffix(f, "?"):
			reason = results.ReasonUndecided
		}
		o.State.Failing = append(o.State.Failing, results.Failure{ID: strings.TrimRight(f, "!?"), Reason: reason})
	}
	return o
}

// describe renders events as short strings for comparison.
func describe(evs []Event) []string {
	var out []string
	for _, e := range evs {
		switch e := e.(type) {
		case Baseline:
			if e.StartsRed {
				out = append(out, "Baseline(red)")
			} else {
				out = append(out, "Baseline")
			}
		case RedInProgress:
			if e.Refactoring {
				out = append(out, "BuildBrokenInRefactor")
			} else {
				out = append(out, "RedInProgress")
			}
		case PhaseChanged:
			out = append(out, "Phase "+e.Phase.String())
		case TestsFixed:
			out = append(out, fmt.Sprintf("Fixed(%d)", e.Step))
		case StepDone:
			s := e.Step
			d := s.Kind.String()
			var extra []string
			if len(s.NewTests) > 0 && s.Kind != Green {
				extra = append(extra, strings.Join(s.NewTests, "+"))
			}
			for _, a := range s.Anomalies {
				extra = append(extra, anomalyTag[a])
			}
			if s.AfterGreen {
				extra = append(extra, "afterGreen")
			}
			if len(extra) > 0 {
				d += "(" + strings.Join(extra, ",") + ")"
			}
			d += fmt.Sprintf("[%s→%s]", s.From, s.To)
			out = append(out, d)
		}
	}
	return out
}

var anomalyTag = map[Anomaly]string{
	MultipleNewTests:  "multi",
	NewTestPassed:     "passed",
	CodeWithoutTest:   "noTest",
	TestEditedInGreen: "testEdited",
	BrokeExistingTest: "broke",
}

func replay(m *Machine, runs []run) []string {
	var out []string
	for i, r := range runs {
		out = append(out, describe(m.Observe(r.observation(i)))...)
	}
	return out
}

func TestTransitions(t *testing.T) {
	cases := []struct {
		name string
		runs []run
		want string
	}{
		{"red then green", []run{
			{"a |", ""},
			{"a b | b", "t"},
			{"a b |", "s"},
		}, "Baseline; Red(b)[s0→s1]; Green[s1→s2]"},

		{"red spans several saves", []run{
			{"a |", ""},
			{"broken", "t"},
			{"a b | b!", "t"},
			{"a b | b", "t"},
		}, "Baseline; RedInProgress; Red(b)[s0→s3]"},

		{"test change while green is a (test) refactor", []run{
			{"a |", ""},
			{"a |", "t"},
			{"a b | b", "t"},
		}, "Baseline; Refactor[s0→s1]; Red(b)[s1→s2]"},

		{"build broken is red in progress, then red", []run{
			{"a |", ""},
			{"broken", "t"},
			{"broken", "t"},
			{"a b | b", "s"},
		}, "Baseline; RedInProgress; Red(b)[s0→s3]"},

		{"erroring new test is red in progress until it fails on an assertion", []run{
			{"a |", ""},
			{"a b | b!", "t"},
			{"a b | b!", "t"},
			{"a b | b", "s"},
		}, "Baseline; RedInProgress; Red(b)[s0→s3]"},

		{"undecided reason counts as red (judge checks)", []run{
			{"a |", ""},
			{"a b | b?", "t"},
		}, "Baseline; Red(b)[s0→s1]"},

		{"refactor accumulates until next red", []run{
			{"a |", ""},
			{"a b | b", "t"},
			{"a b |", "s"},
			{"a b |", "s"},
			{"a b |", "s"},
			{"a b c | c", "t"},
		}, "Baseline; Red(b)[s0→s1]; Green[s1→s2]; Refactor[s2→s4]; Red(c)[s4→s5]"},

		{"green directly followed by red is flagged for missed refactor", []run{
			{"a |", ""},
			{"a b | b", "t"},
			{"a b |", "s"},
			{"a b c | c", "t"},
		}, "Baseline; Red(b)[s0→s1]; Green[s1→s2]; Red(c,afterGreen)[s2→s3]"},

		{"several new tests at once", []run{
			{"a |", ""},
			{"a b c | b c", "t"},
			{"a b c |", "s"},
		}, "Baseline; Red(b+c,multi)[s0→s1]; Green[s1→s2]"},

		{"new test passes immediately", []run{
			{"a |", ""},
			{"a b |", "t"},
		}, "Baseline; Anomaly(b,passed)[s0→s1]"},

		{"code and passing test written together", []run{
			{"a |", ""},
			{"a b |", "ts"},
		}, "Baseline; Anomaly(b,noTest)[s0→s1]"},

		{"red in progress that never fails on an assertion", []run{
			{"a |", ""},
			{"broken", "t"},
			{"a b |", "s"},
		}, "Baseline; RedInProgress; Anomaly(b,passed)[s0→s2]"},

		{"test edited during green", []run{
			{"a |", ""},
			{"a b | b", "t"},
			{"a b | b", "t"},
			{"a b |", "s"},
		}, "Baseline; Red(b)[s0→s1]; Green(testEdited)[s1→s3]"},

		{"test goes red during refactor; fixing it goes on refactoring, it is no new Green", []run{
			{"a b |", ""},
			{"a b | a", "s"},
			{"a b |", "s"},
			{"a b c | c", "t"},
		}, "Baseline; Anomaly(broke)[s0→s1]; Fixed(1); Refactor[s1→s2]; Red(c)[s2→s3]"},

		{"existing test breaks while making it pass (reported once)", []run{
			{"a |", ""},
			{"a b | b", "t"},
			{"a b | a b", "s"},
			{"a b | a b", "s"},
			{"a b |", "s"},
		}, "Baseline; Red(b)[s0→s1]; Anomaly(broke)[s1→s2]; Fixed(2); Green[s2→s4]"},

		{"compile errors while making it pass are normal", []run{
			{"a |", ""},
			{"a b | b", "t"},
			{"broken", "s"},
			{"a b |", "s"},
		}, "Baseline; Red(b)[s0→s1]; Green[s1→s3]"},

		{"renamed test is a refactor, not a new test", []run{
			{"a b |", ""},
			{"a c |", "t"},
			{"a c d | d", "t"},
		}, "Baseline; Refactor[s0→s1]; Red(d)[s1→s2]"},

		{"existing test filled in and passing without failing first", []run{
			{"a |", ""},
			{"broken", "t"},
			{"a |", "s"},
		}, "Baseline; RedInProgress; Anomaly(passed)[s0→s2]"},

		{"existing empty test filled in: build broken, stub errors, then it fails on its assertion", []run{
			{"a |", ""},
			{"broken", "t"},
			{"a | a!", "s"},
			{"a | a", "s"},
		}, "Baseline; RedInProgress; Red(a)[s0→s3]"},

		{"empty test first, then its body, a panicking stub and a return (live session)", []run{
			{"|", ""},
			{"a |", "t"},
			{"broken", "t"},
			{"broken", "ts"},
			{"a | a!", "t"},
			{"a | a", "s"},
		}, "Baseline; Anomaly(a,passed)[s0→s1]; RedInProgress; Red(a)[s1→s5]"},

		{"existing test filled in, fails on its assertion once it compiles", []run{
			{"a |", ""},
			{"broken", "t"},
			{"a | a", "s"},
		}, "Baseline; RedInProgress; Red(a)[s0→s2]"},

		{"existing test changed to fail on its assertion", []run{
			{"a b |", ""},
			{"a b | a", "t"},
			{"a b |", "s"},
		}, "Baseline; Red(a)[s0→s1]; Green[s1→s2]"},

		{"test goes red while test and code change during refactor", []run{
			{"a b |", ""},
			{"a b | a", "ts"},
		}, "Baseline; Anomaly(broke)[s0→s1]"},

		{"test refactoring that breaks the build for a moment is still refactor", []run{
			{"a b |", ""},
			{"broken", "t"},
			{"a b |", "t"},
			{"a b |", "t"},
			{"a b c | c", "t"},
		}, "Baseline; RedInProgress; Refactor[s0→s3]; Red(c)[s3→s4]"},

		{"build broken during refactor then green is still refactor", []run{
			{"a |", ""},
			{"broken", "s"},
			{"a |", "s"},
			{"a b | b", "t"},
		}, "Baseline; BuildBrokenInRefactor; Refactor[s0→s2]; Red(b)[s2→s3]"},

		{"build broken during refactor, then it compiles but an existing test fails", []run{
			{"a b |", ""},
			{"broken", "s"},
			{"a b | a", "s"},
			{"a b |", "s"},
			{"a b c | c", "t"},
		}, "Baseline; BuildBrokenInRefactor; Anomaly(broke)[s0→s2]; Fixed(1); Refactor[s2→s3]; Red(c)[s3→s4]"},

		{"session starting red", []run{
			{"a b | b", ""},
			{"a b |", "s"},
			{"a b c | c", "t"},
		}, "Baseline(red); Red(c)[s1→s2]"},

		{"session starting red, its failing test then fails properly", []run{
			{"a b | b!", ""},
			{"a b | b", "s"},
			{"a b |", "s"},
		}, "Baseline(red); Red(b)[s0→s1]; Green[s1→s2]"},

		{"exit code only: tests unknown", []run{
			{"? |", ""},
			{"? | run?", "t"},
			{"? |", "s"},
		}, "Baseline; Red(run)[s0→s1]; Green[s1→s2]"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := strings.Join(replay(New(), c.runs), "; ")
			if got != c.want {
				t.Fatalf("\ngot  %s\nwant %s", got, c.want)
			}
		})
	}
}

func TestOverrides(t *testing.T) {
	m := New()
	m.Observe(run{"a |", ""}.observation(0))
	// the learner says they are making a test pass although nothing failed
	got := describe(m.Override(PhaseGreen))
	if strings.Join(got, ";") != "Phase Green" || m.Phase() != PhaseGreen {
		t.Fatalf("got %v", got)
	}
	evs := m.Observe(run{"a |", "s"}.observation(1))
	d := describe(evs)
	if len(d) != 1 || !strings.HasPrefix(d[0], "Green") || !evs[0].(StepDone).Step.Overridden {
		t.Fatalf("got %v", d)
	}
	if m.Phase() != PhaseRefactor {
		t.Fatalf("phase %v", m.Phase())
	}
}

func TestResetBaseline(t *testing.T) {
	m := New()
	replay(m, []run{{"a |", ""}, {"a b | b", "t"}})
	m.ResetBaseline()
	got := describe(m.Observe(run{"a b | b", ""}.observation(5)))
	if strings.Join(got, ";") != "Baseline(red)" {
		t.Fatalf("got %v", got)
	}
	// numbering continues across baselines
	evs := m.Observe(run{"a b |", "s"}.observation(6))
	if len(evs) != 0 {
		t.Fatalf("session began mid-cycle; want nothing judged, got %v", describe(evs))
	}
	evs = m.Observe(run{"a b c | c", "t"}.observation(7))
	if s := evs[len(evs)-1].(StepDone).Step; s.N != 2 {
		t.Fatalf("step number %d", s.N)
	}
}

func TestStepCarriesChangesAndFailing(t *testing.T) {
	m := New()
	evs := replay2(m, []run{{"a |", ""}, {"a b | b", "t"}})
	s := evs[len(evs)-1].(StepDone).Step
	if !s.TestsChanged || s.SourceChanged || len(s.Failing) != 1 || s.Failing[0].ID != "b" {
		t.Fatalf("got %+v", s)
	}
}

func replay2(m *Machine, runs []run) []Event {
	var out []Event
	for i, r := range runs {
		out = append(out, m.Observe(r.observation(i))...)
	}
	return out
}

// lastStep runs a first session and returns its last completed step.
func lastStep(t *testing.T, runs []run) Step {
	t.Helper()
	evs := replay2(New(), runs)
	for i := len(evs) - 1; i >= 0; i-- {
		if sd, ok := evs[i].(StepDone); ok {
			return sd.Step
		}
	}
	t.Fatal("no step")
	return Step{}
}

func TestResumeAfterRedCompletesTheGreen(t *testing.T) {
	last := lastStep(t, []run{{"a |", ""}, {"a b | b", "t"}})
	m := New()
	m.Resume([]Step{last})
	if m.Phase() != PhaseGreen {
		t.Fatalf("phase %v", m.Phase())
	}
	got := describe(m.Observe(run{"a b |", "s"}.observation(9)))
	if strings.Join(got, ";") != "Green[s1→s9]" {
		t.Fatalf("got %v", got)
	}
}

func TestResumeAfterGreenContinuesTheCycle(t *testing.T) {
	last := lastStep(t, []run{{"a |", ""}, {"a b | b", "t"}, {"a b |", "s"}})
	m := New()
	m.Resume([]Step{last})
	if m.Phase() != PhaseRefactor {
		t.Fatalf("phase %v", m.Phase())
	}
	// refactored while tddt was off, then the next test
	got := describe(m.Observe(run{"a b |", "s"}.observation(8)))
	got = append(got, describe(m.Observe(run{"a b c | c", "t"}.observation(9)))...)
	if strings.Join(got, ";") != "Refactor[s2→s8];Red(c)[s8→s9]" {
		t.Fatalf("got %v", got)
	}
}

func TestResumeAfterGreenNextRedIsAfterGreen(t *testing.T) {
	last := lastStep(t, []run{{"a |", ""}, {"a b | b", "t"}, {"a b |", "s"}})
	m := New()
	m.Resume([]Step{last})
	evs := m.Observe(run{"a b c | c", "t"}.observation(9))
	if s := evs[len(evs)-1].(StepDone).Step; s.N != 3 || !s.AfterGreen || s.Kind != Red {
		t.Fatalf("got %v", describe(evs))
	}
}

func TestResumeAfterATestBrokeWhileRefactoring(t *testing.T) {
	runs := []run{{"a |", ""}, {"a b | b", "t"}, {"a b |", "s"}, {"a b | a", "s"}}
	evs := replay2(New(), runs)
	var done []Step
	for _, e := range evs {
		if sd, ok := e.(StepDone); ok {
			done = append(done, sd.Step)
		}
	}
	m := New()
	m.Resume(done)
	got := describe(m.Observe(run{"a b |", "s"}.observation(8)))
	got = append(got, describe(m.Observe(run{"a b c | c", "t"}.observation(9)))...)
	if strings.Join(got, ";") != "Fixed(3);Refactor[s3→s8];Red(c)[s8→s9]" {
		t.Fatalf("got %v", got)
	}
}

func TestResumeWithABrokenTestIsNoRed(t *testing.T) {
	last := lastStep(t, []run{{"a |", ""}, {"a b | b", "t"}, {"a b |", "s"}})
	m := New()
	m.Resume([]Step{last})
	// the code changed while tddt was off and broke a test that existed
	got := describe(m.Observe(run{"a b | a", "s"}.observation(8)))
	if strings.Join(got, ";") != "Anomaly(broke)[s2→s8]" {
		t.Fatalf("got %v", got)
	}
}
