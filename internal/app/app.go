// Package app wires one Session together: watcher, runner, snapshots,
// step inference and the coach. Everything that happens is sent to a
// sink as a message; the TUI or a plain printer renders them.
package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/sbradl/tdd-trainer/internal/coach"
	"github.com/sbradl/tdd-trainer/internal/config"
	"github.com/sbradl/tdd-trainer/internal/judge"
	"github.com/sbradl/tdd-trainer/internal/runner"
	"github.com/sbradl/tdd-trainer/internal/session"
	"github.com/sbradl/tdd-trainer/internal/snapshot"
	"github.com/sbradl/tdd-trainer/internal/steps"
	"github.com/sbradl/tdd-trainer/internal/watch"
)

// Messages sent to the sink, besides session.Event, steps.Event and
// coach.Verdict.
type (
	// PhaseMsg: the phase after an observation or override.
	PhaseMsg struct{ Phase steps.Phase }
	// JudgeMsg: the judge finished loading (GPU tells the backend) or failed.
	JudgeMsg struct {
		Ready, GPU bool
		Err        error
	}
	// ErrorMsg: something went wrong that does not end the session.
	ErrorMsg struct{ Err error }
)

// StepRecord is a completed step with the verdicts it got so far.
type StepRecord struct {
	Step     steps.Step
	Verdicts []coach.Verdict
}

// History is what happened in the session, for the report.
type History struct {
	Start, End   time.Time
	Steps        []StepRecord
	StartsRed    bool
	ExitCodeOnly bool
	NextTests    []NextTestRecord `json:",omitempty"`
}

// NextTestRecord is a next-test hint the learner asked for.
type NextTestRecord struct {
	Step  int // the newest step when asked
	Stage int
	Case  string
}

// SessionsDir holds one JSON file per session, for `tddt show`.
var SessionsDir = filepath.Join(snapshot.Dir, "sessions")

// SessionFile is the history file of a session that started at start.
func SessionFile(root string, start time.Time) string {
	return filepath.Join(root, SessionsDir, start.Format("2006-01-02T15-04-05")+".json")
}

// LoadLatestSession reads the newest session file under root.
func LoadLatestSession(root string) (History, string, error) {
	files, _ := filepath.Glob(filepath.Join(root, SessionsDir, "*.json"))
	if len(files) == 0 {
		return History{}, "", errors.New("no session recorded yet")
	}
	sort.Strings(files)
	path := files[len(files)-1]
	data, err := os.ReadFile(path)
	if err != nil {
		return History{}, "", err
	}
	var h History
	err = json.Unmarshal(data, &h)
	return h, path, err
}

// save writes the history for `tddt show`; errors are reported, not fatal.
func (a *App) save() {
	a.saveMu.Lock() // the newest history must be written last
	defer a.saveMu.Unlock()
	h := a.History()
	data, err := json.MarshalIndent(h, "", "  ")
	if err == nil {
		path := SessionFile(a.dir, h.Start)
		if err = os.MkdirAll(filepath.Dir(path), 0o755); err == nil {
			// write then rename, so `tddt show` never reads half a file
			if err = os.WriteFile(path+".tmp", data, 0o644); err == nil {
				err = os.Rename(path+".tmp", path)
			}
		}
	}
	if err != nil {
		a.sink(ErrorMsg{fmt.Errorf("saving session: %w", err)})
	}
}

type App struct {
	dir   string
	cfg   config.Config
	sink  func(any)
	judge *judge.Lazy // nil: exact checks only
	store *snapshot.Store
	coach *coach.Coach

	trigger chan []string // manual reruns
	saveMu  sync.Mutex

	mu      sync.Mutex
	machine *steps.Machine
	prev    snapshot.ID
	hist    History
	byStep  map[int]int // step N -> index in hist.Steps
}

// New prepares a session in dir. jd may be nil.
func New(dir string, cfg config.Config, jd *judge.Lazy, sink func(any)) (*App, error) {
	store, err := snapshot.Open(dir, cfg)
	if err != nil {
		return nil, err
	}
	a := &App{dir: dir, cfg: cfg, sink: sink, judge: jd, store: store, machine: steps.New(), byStep: map[int]int{},
		trigger: make(chan []string, 1)}
	a.hist.Start = time.Now()
	var scorer judge.Scorer
	if jd != nil {
		scorer = jd
	}
	a.coach = coach.New(store, scorer, cfg.TPPOrder, a.verdict)
	a.coach.SetNextTestSink(a.nextTest)
	return a, nil
}

// Store exposes the snapshots, e.g. for diffs in the report.
func (a *App) Store() *snapshot.Store { return a.store }

// PendingGates lists gates the judge has not answered yet.
func (a *App) PendingGates() []coach.PendingGate { return a.coach.PendingGates() }

// Phase is the current phase.
func (a *App) Phase() steps.Phase {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.machine.Phase()
}

// History returns a copy of what happened so far.
func (a *App) History() History {
	a.mu.Lock()
	defer a.mu.Unlock()
	h := a.hist
	h.End = time.Now()
	h.NextTests = append([]NextTestRecord{}, a.hist.NextTests...)
	h.Steps = make([]StepRecord, len(a.hist.Steps))
	for i, r := range a.hist.Steps {
		h.Steps[i] = StepRecord{Step: r.Step, Verdicts: append([]coach.Verdict{}, r.Verdicts...)}
	}
	return h
}

// Run watches and coaches until ctx ends.
func (a *App) Run(ctx context.Context) error {
	w, err := watch.New(a.dir, a.cfg, watch.DefaultDebounce)
	if err != nil {
		return err
	}
	batches, _ := w.Run(ctx)
	if a.judge != nil {
		go func() {
			e, err := a.judge.Wait(ctx)
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				a.sink(JudgeMsg{Err: err})
				return
			}
			a.sink(JudgeMsg{Ready: true, GPU: e.GPU})
		}()
	}
	go a.coach.Run(ctx)
	changes := make(chan []string)
	go func() {
		defer close(changes)
		for {
			var c []string
			select {
			case <-ctx.Done():
				return
			case b, ok := <-batches:
				if !ok {
					return
				}
				c = b
			case c = <-a.trigger:
			}
			select {
			case changes <- c:
			case <-ctx.Done():
				return
			}
		}
	}()
	session.Loop(ctx, changes, a.runOnce, a.cfg.SlowRunWarning, a.event)
	return nil
}

func (a *App) runOnce(ctx context.Context) (session.Result, error) {
	id, err := a.store.Snapshot("test run")
	if err != nil {
		return session.Result{}, err
	}
	o, err := runner.Run(ctx, a.cfg, a.dir)
	return session.Result{Outcome: o, Snapshot: id}, err
}

func (a *App) event(e session.Event) {
	a.sink(e)
	done, ok := e.(session.RunDone)
	if !ok {
		return
	}
	a.mu.Lock()
	obs := steps.Observation{Snapshot: done.Snapshot, State: done.Outcome.State}
	var diffErr error
	if a.prev != "" {
		var diff []snapshot.FileDiff
		diff, diffErr = a.store.Diff(a.prev, done.Snapshot)
		for _, d := range diff {
			obs.TestsChanged = obs.TestsChanged || d.Kind == config.Test
			obs.SourceChanged = obs.SourceChanged || d.Kind == config.Source
		}
	}
	a.prev = done.Snapshot
	a.hist.ExitCodeOnly = a.hist.ExitCodeOnly || done.Outcome.ExitCodeOnly
	evs := a.machine.Observe(obs)
	phase := a.machine.Phase()
	for _, ev := range evs {
		switch ev := ev.(type) {
		case steps.Baseline:
			a.hist.StartsRed = ev.StartsRed
		case steps.StepDone:
			a.byStep[ev.Step.N] = len(a.hist.Steps)
			a.hist.Steps = append(a.hist.Steps, StepRecord{Step: ev.Step})
		}
	}
	a.mu.Unlock()

	if diffErr != nil {
		a.sink(ErrorMsg{diffErr})
	}
	stepped := false
	for _, ev := range evs {
		a.sink(ev)
		if sd, ok := ev.(steps.StepDone); ok {
			stepped = true
			if err := a.coach.Step(sd.Step); err != nil {
				a.sink(ErrorMsg{err})
			}
		}
	}
	if stepped {
		a.save()
	}
	// While refactoring, every green save re-checks an open refactor hint:
	// the code one when code changed, the test one when tests changed.
	if obs.State.Green() && phase == steps.PhaseRefactor {
		if obs.SourceChanged {
			if err := a.coach.Recheck(done.Snapshot); err != nil {
				a.sink(ErrorMsg{err})
			}
		}
		if obs.TestsChanged {
			if err := a.coach.RecheckTests(done.Snapshot); err != nil {
				a.sink(ErrorMsg{err})
			}
		}
	}
	a.sink(PhaseMsg{phase})
}

func (a *App) verdict(v coach.Verdict) {
	a.mu.Lock()
	if i, ok := a.byStep[v.Step]; ok {
		a.hist.Steps[i].Verdicts = coach.Upsert(a.hist.Steps[i].Verdicts, v)
	}
	a.mu.Unlock()
	a.sink(v)
	if !v.Exact {
		a.save() // exact verdicts are saved with their step
	}
}

// NextTest asks for a hint on which test to write next (hotkey n); the
// hint arrives as a coach.NextTest message.
func (a *App) NextTest() {
	a.mu.Lock()
	now, phase := a.prev, a.machine.Phase()
	var green []coach.Verdict
	for _, r := range a.hist.Steps {
		if r.Step.Kind == steps.Green {
			green = append([]coach.Verdict{}, r.Verdicts...)
		}
	}
	a.mu.Unlock()
	if now == "" {
		a.sink(coach.NextTest{Text: "Wait for the first test run."})
		return
	}
	if err := a.coach.NextTest(now, phase, green); err != nil {
		a.sink(ErrorMsg{err})
	}
}

func (a *App) nextTest(n coach.NextTest) {
	if n.Stage > 0 && !n.Pending {
		a.mu.Lock()
		last := 0
		if len(a.hist.Steps) > 0 {
			last = a.hist.Steps[len(a.hist.Steps)-1].Step.N
		}
		a.hist.NextTests = append(a.hist.NextTests, NextTestRecord{Step: last, Stage: n.Stage, Case: n.Case})
		a.mu.Unlock()
		defer a.save()
	}
	a.sink(n)
}

// Override sets the phase by hand (hotkeys r/g/f).
func (a *App) Override(p steps.Phase) {
	a.mu.Lock()
	evs := a.machine.Override(p)
	a.mu.Unlock()
	for _, ev := range evs {
		a.sink(ev)
	}
	a.sink(PhaseMsg{p})
}

// ResetBaseline makes the next test run the new baseline (hotkey b).
func (a *App) ResetBaseline() {
	a.mu.Lock()
	a.machine.ResetBaseline()
	a.prev = ""
	a.mu.Unlock()
	a.Rerun()
}

// Rerun runs the tests now, without a file change.
func (a *App) Rerun() {
	select {
	case a.trigger <- []string{}:
	default: // one is pending already
	}
}
