package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sbradl/tdd-trainer/internal/coach"
	"github.com/sbradl/tdd-trainer/internal/config"
	"github.com/sbradl/tdd-trainer/internal/results"
	"github.com/sbradl/tdd-trainer/internal/runner"
	"github.com/sbradl/tdd-trainer/internal/session"
	"github.com/sbradl/tdd-trainer/internal/steps"
)

// baselineStartsRed feeds one broken run of dir as the baseline.
func baselineStartsRed(t *testing.T, files map[string]string) bool {
	t.Helper()
	dir := t.TempDir()
	for p, text := range files {
		if err := os.WriteFile(filepath.Join(dir, p), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cfg, err := config.Parse([]byte("test: { cmd: 'exit 1' }\ntests: ['*_test.go']\nsources: ['*.go']\n"))
	if err != nil {
		t.Fatal(err)
	}
	a, err := New(dir, cfg, nil, func(any) {})
	if err != nil {
		t.Fatal(err)
	}
	id, err := a.store.Snapshot("test run")
	if err != nil {
		t.Fatal(err)
	}
	a.event(session.RunDone{Result: session.Result{Snapshot: id, Outcome: runner.Outcome{State: results.TestState{BuildBroken: true}}}})
	return a.History().StartsRed
}

func TestEmptyKataFolderStartsGreen(t *testing.T) {
	// go vet fails with "no packages" in a folder with only go.mod
	if baselineStartsRed(t, map[string]string{"go.mod": "module kata\n"}) {
		t.Error("a folder without tests starts red, want an empty green start")
	}
}

func TestBrokenTestsStartRed(t *testing.T) {
	if !baselineStartsRed(t, map[string]string{"kata_test.go": "package kata\n\nfunc TestX(t *testing.T) { Missing() }\n"}) {
		t.Error("a folder with a broken test starts green, want a Red in progress")
	}
}

func TestResumeContinuesTheLatestSession(t *testing.T) {
	dir := t.TempDir()
	cfg, err := config.Parse([]byte("test: { cmd: 'exit 1' }\ntests: ['*_test.go']\nsources: ['*.go']\n"))
	if err != nil {
		t.Fatal(err)
	}
	write := func(p, text string) {
		if err := os.WriteFile(filepath.Join(dir, p), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	observe := func(a *App, st results.TestState) {
		id, err := a.store.Snapshot("test run")
		if err != nil {
			t.Fatal(err)
		}
		a.event(session.RunDone{Result: session.Result{Snapshot: id, Outcome: runner.Outcome{State: st}}})
	}
	first, err := New(dir, cfg, nil, func(any) {})
	if err != nil {
		t.Fatal(err)
	}
	write("kata.go", "package kata\n")
	observe(first, results.TestState{Tests: []string{}})
	write("kata_test.go", "package kata\n\nfunc TestOne(t *testing.T) {}\n")
	observe(first, results.TestState{Tests: []string{"TestOne"}, Failing: []results.Failure{{ID: "TestOne", Reason: results.ReasonAssertion}}})

	second, err := New(dir, cfg, nil, func(any) {})
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := second.Resume(); !ok || err != nil {
		t.Fatalf("Resume() = %v, %v", ok, err)
	}
	write("kata.go", "package kata\n\nfunc One() {}\n")
	observe(second, results.TestState{Tests: []string{"TestOne"}})
	h := second.History()
	if len(h.Steps) != 2 || h.Steps[1].Step.N != 2 || h.Steps[1].Step.Kind.String() != "Green" {
		t.Fatalf("want the Red and then a Green as step 2, got %+v", h.Steps)
	}
	if !h.Start.Equal(first.History().Start) {
		t.Fatal("a resumed session keeps its start, and so its session file")
	}
	saved, _, err := LoadLatestSession(dir)
	if err != nil || len(saved.Steps) != 2 {
		t.Fatalf("saved %d steps, %v", len(saved.Steps), err)
	}
}

func TestResumeWithoutASessionStartsANewOne(t *testing.T) {
	cfg, _ := config.Parse([]byte("test: { cmd: 'exit 1' }\n"))
	a, err := New(t.TempDir(), cfg, nil, func(any) {})
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := a.Resume(); ok || err != nil {
		t.Fatalf("Resume() = %v, %v", ok, err)
	}
}

// brokeAndFixed runs a session in which a test breaks while refactoring
// and passes again; it returns the app and the broken step's verdicts.
func brokeAndFixed(t *testing.T) (*App, []coach.Verdict) {
	t.Helper()
	dir := t.TempDir()
	cfg, err := config.Parse([]byte("test: { cmd: 'exit 1' }\ntests: ['*_test.go']\nsources: ['*.go']\n"))
	if err != nil {
		t.Fatal(err)
	}
	a, err := New(dir, cfg, nil, func(any) {})
	if err != nil {
		t.Fatal(err)
	}
	for i, st := range []results.TestState{
		{Tests: []string{"TestOne"}},
		{Tests: []string{"TestOne"}, Failing: []results.Failure{{ID: "TestOne", Reason: results.ReasonAssertion}}},
		{Tests: []string{"TestOne"}},
	} {
		if err := os.WriteFile(filepath.Join(dir, "kata.go"), []byte(fmt.Sprintf("package kata // %d\n", i)), 0o644); err != nil {
			t.Fatal(err)
		}
		id, err := a.store.Snapshot("test run")
		if err != nil {
			t.Fatal(err)
		}
		a.event(session.RunDone{Result: session.Result{Snapshot: id, Outcome: runner.Outcome{State: st}}})
	}
	h := a.History()
	if len(h.Steps) != 1 {
		t.Fatalf("want only the broken test's anomaly as a step, got %+v", h.Steps)
	}
	return a, h.Steps[0].Verdicts
}

func TestFixingABrokenTestResolvesItsWarning(t *testing.T) {
	_, vs := brokeAndFixed(t)
	if len(vs) != 1 || vs[0].Level != coach.OK || !strings.HasPrefix(vs[0].Text, "Resolved") {
		t.Fatalf("got %+v", vs)
	}
}

func TestResumeResolvesBrokenTestWarningsOfOldSessions(t *testing.T) {
	a, _ := brokeAndFixed(t)
	// as an earlier version saved it: still a warning, and a Green after it
	a.hist.Steps[0].Verdicts = []coach.Verdict{{Step: 1, Kind: steps.AnomalyStep, Check: "anomaly", Level: coach.Warn, Exact: true,
		Text: "A test that passed before is failing now: undo the last change or get back to green first."}}
	last := a.hist.Steps[0].Step
	a.hist.Steps = append(a.hist.Steps, StepRecord{Step: steps.Step{N: 2, Kind: steps.Green, From: last.To, To: last.To}})
	a.save()
	b, err := New(a.dir, a.cfg, nil, func(any) {})
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := b.Resume(); !ok || err != nil {
		t.Fatalf("Resume() = %v, %v", ok, err)
	}
	if v := b.History().Steps[0].Verdicts[0]; v.Level != coach.OK {
		t.Fatalf("got %+v", v)
	}
	if saved, _, _ := LoadLatestSession(a.dir); saved.Steps[0].Verdicts[0].Level != coach.OK {
		t.Fatal("the resolved warning is not saved for tddt show")
	}
}
