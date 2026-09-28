package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sbradl/tdd-trainer/internal/app"
	"github.com/sbradl/tdd-trainer/internal/coach"
	"github.com/sbradl/tdd-trainer/internal/results"
	"github.com/sbradl/tdd-trainer/internal/runner"
	"github.com/sbradl/tdd-trainer/internal/session"
	"github.com/sbradl/tdd-trainer/internal/steps"
)

type fakeCtl struct {
	overrides []steps.Phase
	resets    int
	pending   []coach.PendingGate
	reports   int
	nexts     int
}

func (f *fakeCtl) Override(p steps.Phase)            { f.overrides = append(f.overrides, p) }
func (f *fakeCtl) ResetBaseline()                    { f.resets++ }
func (f *fakeCtl) PendingGates() []coach.PendingGate { return f.pending }
func (f *fakeCtl) WriteReport() (string, error)      { f.reports++; return ".tddtrainer/reports/x.md", nil }
func (f *fakeCtl) NextTest()                         { f.nexts++ }

func run(tests []string, failing ...string) session.RunDone {
	st := results.TestState{Tests: tests}
	for _, f := range failing {
		st.Failing = append(st.Failing, results.Failure{ID: f})
	}
	return session.RunDone{Result: session.Result{Outcome: runner.Outcome{State: st}}}
}

func feed(m *Model, msgs ...tea.Msg) {
	for _, msg := range msgs {
		m.Update(msg)
	}
}

func step(n int, k steps.Kind, newTests ...string) steps.StepDone {
	return steps.StepDone{Step: steps.Step{N: n, Kind: k, NewTests: newTests}}
}

func verdict(n int, k steps.Kind, check string, l coach.Level, text string) coach.Verdict {
	return coach.Verdict{Step: n, Kind: k, Check: check, Level: l, Text: text}
}

// A kata session as in the verdict-ux prototype, cut short.
func session1(m *Model) {
	feed(m,
		tea.WindowSizeMsg{Width: 100, Height: 30},
		run([]string{}), steps.Baseline{}, app.PhaseMsg{Phase: steps.PhaseRefactor},
		run(nil), steps.RedInProgress{BuildBroken: true}, app.PhaseMsg{Phase: steps.PhaseRedInProgress},
		run([]string{"one"}, "one"), step(1, steps.Red, "one"), app.PhaseMsg{Phase: steps.PhaseGreen},
		verdict(1, steps.Red, "new tests", coach.OK, "exactly 1 new test"),
		run([]string{"one"}), step(2, steps.Green), app.PhaseMsg{Phase: steps.PhaseRefactor},
		verdict(2, steps.Green, "step-size", coach.Hint, "A simpler change would have done."),
		verdict(2, steps.Green, "multi", coach.Uncertain, "not sure (p=0.63)"),
	)
}

func TestQuietViewShowsStatusAndCards(t *testing.T) {
	m := New(&fakeCtl{}, "judge loading")
	session1(m)
	v := m.View()
	for _, want := range []string{"● Refactor", "1 passed", "✓ 1", "step 2 · Green · Simplest change", "A simpler change would have done."} {
		if !strings.Contains(v, want) {
			t.Errorf("missing %q in:\n%s", want, v)
		}
	}
	if strings.Contains(v, "not sure") {
		t.Errorf("uncertain verdicts must not show live:\n%s", v)
	}
	if strings.Contains(v, "Red in progress") {
		t.Errorf("stale red-in-progress card:\n%s", v)
	}
}

func TestClosedCycleCollapses(t *testing.T) {
	m := New(&fakeCtl{}, "")
	session1(m)
	feed(m, run([]string{"one", "two"}, "two"), step(3, steps.Red, "two"), app.PhaseMsg{Phase: steps.PhaseGreen})
	v := m.View()
	if strings.Contains(v, "A simpler change") {
		t.Errorf("closed cycle's card still expanded:\n%s", v)
	}
	if !strings.Contains(v, "cycle 1") || !strings.Contains(v, "R G") || !strings.Contains(v, "1 hint(s)") {
		t.Errorf("missing cycle summary:\n%s", v)
	}
	if !strings.Contains(v, "● Green") || !strings.Contains(v, "1 passed · 1 failed") {
		t.Errorf("status:\n%s", v)
	}
}

func TestRedInProgressCard(t *testing.T) {
	m := New(&fakeCtl{}, "")
	feed(m, run([]string{}), steps.Baseline{}, run(nil), steps.RedInProgress{BuildBroken: true})
	if v := m.View(); !strings.Contains(v, "┌ Red in progress") || !strings.Contains(v, "smallest stub") {
		t.Errorf("missing card:\n%s", v)
	}
}

func TestDashboard(t *testing.T) {
	ctl := &fakeCtl{pending: []coach.PendingGate{{Step: 2, Gate: "cheating", ETA: 7 * time.Second}}}
	m := New(ctl, "")
	session1(m)
	m.Update(tickMsg(time.Now()))
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	v := m.View()
	for _, want := range []string{"REFACTOR", "cycle  R✓ G", "step   2 · Green", "Simplest change", "One transformation", "not sure", "No test-specific code", "in ~7s"} {
		if !strings.Contains(v, want) {
			t.Errorf("missing %q in:\n%s", want, v)
		}
	}
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if strings.Contains(m.View(), "REFACTOR") {
		t.Error("tab did not toggle back")
	}
}

func TestKeys(t *testing.T) {
	ctl := &fakeCtl{}
	m := New(ctl, "")
	for _, k := range "rgfb" {
		before := len(ctl.overrides) + ctl.resets
		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{k}})
		if len(ctl.overrides)+ctl.resets != before {
			t.Fatal("controller called inside Update; it sends to the program and would deadlock")
		}
		cmd()
	}
	want := []steps.Phase{steps.PhaseRedInProgress, steps.PhaseGreen, steps.PhaseRefactor}
	if len(ctl.overrides) != 3 || ctl.overrides[0] != want[0] || ctl.overrides[1] != want[1] || ctl.overrides[2] != want[2] || ctl.resets != 1 {
		t.Fatalf("got %+v", ctl)
	}
	if !strings.Contains(m.View(), "baseline reset") {
		t.Errorf("no toast:\n%s", m.View())
	}
	m.Update(steps.PhaseChanged{Phase: steps.PhaseGreen})
	if !strings.Contains(m.View(), "phase set by hand → Green") {
		t.Errorf("no override toast:\n%s", m.View())
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	m.Update(cmd())
	if ctl.reports != 1 || !strings.Contains(m.View(), "report written") {
		t.Errorf("report key:\n%s", m.View())
	}
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("q does not quit")
	}
}

func TestViewFitsSmallTerminal(t *testing.T) {
	m := New(&fakeCtl{}, "")
	session1(m)
	m.Update(tea.WindowSizeMsg{Width: 50, Height: 8})
	if n := strings.Count(m.View(), "\n") + 1; n != 8 {
		t.Fatalf("%d lines for height 8:\n%s", n, m.View())
	}
}

func TestResolvedHintReplacesItsCard(t *testing.T) {
	m := New(&fakeCtl{}, "")
	session1(m)
	feed(m, verdict(2, steps.Green, "refactor opportunity", coach.Hint, "Worth refactoring now: repeated conditionals."))
	if !strings.Contains(m.View(), "Worth refactoring now") {
		t.Fatalf("no card:\n%s", m.View())
	}
	feed(m, verdict(2, steps.Green, "refactor opportunity", coach.OK, "Resolved: repeated conditionals is cleaned up."))
	v := m.View()
	if strings.Contains(v, "Worth refactoring now") || !strings.Contains(v, "Resolved: repeated conditionals") {
		t.Fatalf("card not replaced or no toast:\n%s", v)
	}
}

func TestLongToastWraps(t *testing.T) {
	m := New(&fakeCtl{}, "")
	m.Update(tea.WindowSizeMsg{Width: 40, Height: 12})
	m.showToast("Resolved: your refactoring removed magic numbers or strings, and special-case code mixed into general code.")
	if v := m.View(); !strings.Contains(v, "general") || !strings.Contains(v, "  code. ") {
		t.Fatalf("toast cut off:\n%s", v)
	}
}

func TestNextTestKeyAndCard(t *testing.T) {
	ctl := &fakeCtl{}
	m := New(ctl, "")
	session1(m)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if ctl.nexts != 0 {
		t.Fatal("controller called inside Update")
	}
	cmd()
	if ctl.nexts != 1 {
		t.Fatalf("n did not ask: %d", ctl.nexts)
	}

	m.Update(coach.NextTest{Stage: 1, Pending: true, Text: "Looking at your tests…"})
	if v := m.View(); !strings.Contains(v, "Next test") || !strings.Contains(v, "Looking at your tests") {
		t.Errorf("pending card:\n%s", v)
	}
	m.Update(coach.NextTest{Stage: 1, Case: "boundary", Text: "Try an edge case."})
	if v := m.View(); !strings.Contains(v, "Try an edge case.") || !strings.Contains(v, "press n for more") {
		t.Errorf("stage 1 card:\n%s", v)
	}
	m.Update(coach.NextTest{Stage: 2, Case: "boundary", Text: "No test yet at an edge."})
	if v := m.View(); !strings.Contains(v, "No test yet at an edge.") || strings.Contains(v, "press n for more") {
		t.Errorf("stage 2 card:\n%s", v)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if v := m.View(); !strings.Contains(v, "No test yet at an edge.") {
		t.Errorf("dashboard lacks the hint:\n%s", v)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyTab})

	// writing the next test clears it
	m.Update(steps.RedInProgress{BuildBroken: true})
	if v := m.View(); strings.Contains(v, "Next test") {
		t.Errorf("hint still shown once a new test is written:\n%s", v)
	}

	// a message that is no hint is a toast
	m.Update(coach.NextTest{Text: "Finish the current step first: make the failing test pass."})
	if v := m.View(); strings.Contains(v, "Next test") || !strings.Contains(v, "Finish the current step first") {
		t.Errorf("toast:\n%s", v)
	}
}

func TestResumedSessionShowsItsEarlierSteps(t *testing.T) {
	m := New(&fakeCtl{}, "")
	start := time.Now().Add(-time.Hour)
	h := app.History{Start: start, End: time.Now(), Paused: 50 * time.Minute, Steps: []app.StepRecord{
		{Step: steps.Step{N: 1, Kind: steps.Red, NewTests: []string{"one"}}},
		{Step: steps.Step{N: 2, Kind: steps.Green}, Verdicts: []coach.Verdict{
			verdict(2, steps.Green, "step-size", coach.Hint, "A simpler change would have done."),
		}},
	}}
	feed(m, tea.WindowSizeMsg{Width: 100, Height: 30}, app.ResumedMsg{History: h}, app.PhaseMsg{Phase: steps.PhaseRefactor})
	v := m.View()
	for _, want := range []string{"step 2 · Green · Simplest change", "A simpler change would have done.", "resumed the session: 2 steps so far"} {
		if !strings.Contains(v, want) {
			t.Errorf("missing %q in:\n%s", want, v)
		}
	}
	if d := m.now().Sub(m.start); d < 9*time.Minute || d > 11*time.Minute {
		t.Errorf("session clock at %v, want the 10 minutes practised before the pause", d)
	}
}

func TestFixedBrokenTestDropsItsWarning(t *testing.T) {
	m := New(&fakeCtl{}, "")
	session1(m)
	broke := coach.Verdict{Step: 3, Kind: steps.AnomalyStep, Check: "anomaly", Level: coach.Warn, Answer: steps.BrokeExistingTest.String(),
		Text: "A test that passed before is failing now: undo the last change or get back to green first."}
	feed(m, run([]string{"one"}, "one"), step(3, steps.AnomalyStep), broke)
	if !strings.Contains(m.View(), "A test that passed before is failing now") {
		t.Fatal("want the warning while the test fails")
	}
	feed(m, run([]string{"one"}), steps.TestsFixed{Step: 3}, coach.FixedBreak(3))
	v := m.View()
	if strings.Contains(v, "A test that passed before is failing now") || !strings.Contains(v, "all tests pass again") {
		t.Fatalf("want the warning gone and a note that it was fixed:\n%s", v)
	}
}

func TestBuildBrokenWhileRefactoringThenABrokenTest(t *testing.T) {
	m := New(&fakeCtl{}, "")
	session1(m)
	feed(m, run(nil), steps.RedInProgress{BuildBroken: true, Refactoring: true})
	v := m.View()
	if !strings.Contains(v, "The build broke while refactoring") || strings.Contains(v, "smallest stub") {
		t.Fatalf("want a build-broken card, not a stub request:\n%s", v)
	}
	broke := coach.Verdict{Step: 3, Kind: steps.AnomalyStep, Check: "anomaly", Level: coach.Warn, Answer: steps.BrokeExistingTest.String(),
		Text: "A test that passed before is failing now: undo the last change or get back to green first."}
	feed(m, run([]string{"one"}, "one"), step(3, steps.AnomalyStep), broke)
	v = m.View()
	if strings.Contains(v, "The build broke while refactoring") || !strings.Contains(v, "A test that passed before is failing now") {
		t.Fatalf("want the broken-test warning instead of the build card:\n%s", v)
	}
}
