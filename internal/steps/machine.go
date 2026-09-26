// Package steps infers Red, Green and Refactor steps from the sequence of
// test runs. It is a pure state machine: observations in, events out.
package steps

import (
	"slices"

	"github.com/sbradl/tdd-trainer/internal/results"
	"github.com/sbradl/tdd-trainer/internal/snapshot"
)

// Observation is one finished test run.
type Observation struct {
	Snapshot snapshot.ID
	State    results.TestState
	// What changed since the previous observation's snapshot.
	TestsChanged, SourceChanged bool
}

// Phase is what the learner is expected to do next.
type Phase int

const (
	// PhaseRefactor: all green; refactor, or write the next failing test.
	PhaseRefactor Phase = iota
	// PhaseRedInProgress: a new test is being written but does not fail on
	// an assertion yet (build broken or the test errors).
	PhaseRedInProgress
	// PhaseGreen: a new test fails; make it pass.
	PhaseGreen
)

func (p Phase) String() string {
	return [...]string{"Refactor", "Red in progress", "Green"}[p]
}

type Kind int

const (
	Red Kind = iota
	Green
	Refactor
	// AnomalyStep fits no clean pattern, e.g. an existing test broke.
	AnomalyStep
)

func (k Kind) String() string { return [...]string{"Red", "Green", "Refactor", "Anomaly"}[k] }

type Anomaly int

const (
	MultipleNewTests  Anomaly = iota // several new tests in one Red
	NewTestPassed                    // a new or changed test passed without ever failing
	CodeWithoutTest                  // production code changed along with new, already passing tests
	TestEditedInGreen                // a test file changed during Green
	BrokeExistingTest                // a previously passing test went red
)

func (a Anomaly) String() string {
	return [...]string{
		"several new tests in one step",
		"new or changed test passed without failing first",
		"production code changed without a failing test",
		"test edited during Green",
		"an existing test went red",
	}[a]
}

// Step is one completed step of the cycle.
type Step struct {
	N        int // 1-based, in session order
	Kind     Kind
	From, To snapshot.ID
	// NewTests: for Red the new failing tests, for anomalies the new tests.
	NewTests  []string
	Failing   []results.Failure // test state at the end of the step
	Anomalies []Anomaly
	// AfterGreen: a Red that directly follows a Green without refactoring;
	// the review lenses decide whether a refactor was missed.
	AfterGreen bool
	// Overridden: the learner set the phase by hand during this step.
	Overridden                  bool
	TestsChanged, SourceChanged bool
}

// Event is one of StepDone, RedInProgress, Baseline or PhaseChanged.
type Event interface{ isEvent() }

// StepDone: a step completed and can be judged.
type StepDone struct{ Step Step }

// RedInProgress: a new test is being written but does not fail on an
// assertion yet; not a completed Red.
type RedInProgress struct {
	BuildBroken bool
	Erroring    []results.Failure // new tests failing for the wrong reason
}

// Baseline: the session's starting point was (re)set. StartsRed warns that
// the session starts with failing tests.
type Baseline struct {
	Snapshot  snapshot.ID
	StartsRed bool
}

// PhaseChanged: the learner overrode the phase.
type PhaseChanged struct{ Phase Phase }

func (StepDone) isEvent()      {}
func (RedInProgress) isEvent() {}
func (Baseline) isEvent()      {}
func (PhaseChanged) isEvent()  {}

type changes struct{ tests, source bool }

func (c *changes) add(o changes) { c.tests, c.source = c.tests || o.tests, c.source || o.source }

// Machine is not safe for concurrent use.
type Machine struct {
	started bool
	phase   Phase
	n       int
	last    *Step // last completed step

	stepStart snapshot.ID
	// In PhaseRefactor, greenAt is the last all-green run; refactored
	// holds the changes up to it and pending those after it.
	greenAt    snapshot.ID
	refactored changes
	pending    changes
	overridden bool

	known      []string        // test IDs of the last run that reported them
	failing    map[string]bool // failing IDs of the last run that ran
	inProgress map[string]bool // tests being written in PhaseRedInProgress
	startedRed bool            // baseline was red; its failures are not ours
	brokeShown bool            // BrokeExistingTest already reported this Green
}

func New() *Machine { return &Machine{} }

func (m *Machine) Phase() Phase { return m.phase }

// Observe feeds one test run and returns what it completed.
func (m *Machine) Observe(o Observation) []Event {
	st := o.State
	if !m.started {
		return m.baseline(o)
	}
	m.pending.add(changes{o.TestsChanged, o.SourceChanged})
	added, removed := m.diffTests(st)
	if st.Tests != nil {
		m.known = st.Tests
	}
	prevFailing := m.failing
	if !st.BuildBroken {
		m.failing = failingSet(st)
	}

	if st.BuildBroken {
		if m.phase == PhaseRefactor {
			m.phase = PhaseRedInProgress
			return []Event{RedInProgress{BuildBroken: true}}
		}
		return nil // compile errors while writing a test or code are normal
	}

	// An existing test that fails after the tests changed is being filled
	// in or rewritten, e.g. an empty test given its body: it counts like a
	// new one. While a Red is in progress a stub may change code too.
	rewritten := m.pending.tests && !m.startedRed &&
		(m.phase == PhaseRedInProgress || (m.phase == PhaseRefactor && !m.pending.source))
	var candidates, broke []results.Failure
	for _, f := range st.Failing {
		switch {
		case slices.Contains(added, f.ID) || m.inProgress[f.ID] || rewritten:
			candidates = append(candidates, f)
		case !prevFailing[f.ID]:
			broke = append(broke, f)
		}
	}

	switch m.phase {
	case PhaseGreen:
		if len(st.Failing) == 0 {
			s := Step{Kind: Green}
			if m.pending.tests {
				s.Anomalies = []Anomaly{TestEditedInGreen}
			}
			m.phase = PhaseRefactor
			m.greenAt = o.Snapshot
			return m.complete(o, s)
		}
		if len(broke) > 0 && !m.brokeShown {
			m.brokeShown = true
			return m.complete(o, Step{Kind: AnomalyStep, Anomalies: []Anomaly{BrokeExistingTest}})
		}
		return nil

	case PhaseRedInProgress:
		if len(st.Failing) == 0 {
			m.phase = PhaseRefactor
			m.greenAt = o.Snapshot
			if m.startedRed {
				// the session began mid-cycle; nothing to judge
				m.startedRed = false
				m.stepStart = o.Snapshot
				m.pending, m.refactored = changes{}, changes{}
				return nil
			}
			tests := slices.Clone(added)
			for id := range m.inProgress {
				if !slices.Contains(tests, id) {
					tests = append(tests, id)
				}
			}
			slices.Sort(tests)
			m.inProgress = nil
			if len(tests) == 0 && !m.pending.tests {
				// broke the build while refactoring, then fixed it
				m.refactored.add(m.pending)
				m.pending = changes{}
				return nil
			}
			return m.complete(o, Step{Kind: AnomalyStep, NewTests: tests, Anomalies: []Anomaly{NewTestPassed}})
		}
		return m.maybeRed(o, candidates)

	default: // PhaseRefactor
		if len(broke) > 0 {
			m.phase = PhaseGreen
			m.brokeShown = true
			return m.complete(o, Step{Kind: AnomalyStep, Anomalies: []Anomaly{BrokeExistingTest}})
		}
		if len(candidates) > 0 {
			return m.maybeRed(o, candidates)
		}
		if len(st.Failing) > 0 {
			return nil
		}
		// all green; tests disappearing while others appear are renames
		if len(added) > len(removed) {
			a := NewTestPassed
			if m.pending.source {
				a = CodeWithoutTest
			}
			return m.complete(o, Step{Kind: AnomalyStep, NewTests: added, Anomalies: []Anomaly{a}})
		}
		m.refactored.add(m.pending)
		m.pending = changes{}
		m.greenAt = o.Snapshot
		return nil
	}
}

// maybeRed completes a Red if some new test fails for a reason other than
// a wrong one; otherwise the Red is still in progress.
func (m *Machine) maybeRed(o Observation, candidates []results.Failure) []Event {
	var right []results.Failure
	var erroring []results.Failure
	for _, f := range candidates {
		if f.Reason == results.ReasonWrong {
			erroring = append(erroring, f)
		} else {
			right = append(right, f)
		}
	}
	if len(right) == 0 {
		if m.inProgress == nil {
			m.inProgress = map[string]bool{}
		}
		for _, f := range erroring {
			m.inProgress[f.ID] = true
		}
		if m.phase == PhaseRedInProgress {
			return nil
		}
		m.phase = PhaseRedInProgress
		return []Event{RedInProgress{Erroring: erroring}}
	}

	var evs []Event
	if m.phase == PhaseRefactor || (m.phase == PhaseRedInProgress && !m.startedRed) {
		evs = m.closeRefactor()
	}
	s := Step{Kind: Red, AfterGreen: m.last != nil && m.last.Kind == Green}
	for _, f := range right {
		s.NewTests = append(s.NewTests, f.ID)
	}
	if len(right) > 1 {
		s.Anomalies = []Anomaly{MultipleNewTests}
	}
	m.phase = PhaseGreen
	m.inProgress = nil
	m.startedRed = false
	m.brokeShown = false
	return append(evs, m.complete(o, s)...)
}

// closeRefactor emits the refactoring done before the current Red began,
// if any: from the step start to the last all-green run.
func (m *Machine) closeRefactor() []Event {
	if m.greenAt == "" || m.greenAt == m.stepStart || (!m.refactored.tests && !m.refactored.source) {
		m.pending.add(m.refactored)
		m.refactored = changes{}
		return nil
	}
	m.n++
	s := Step{
		N: m.n, Kind: Refactor, From: m.stepStart, To: m.greenAt,
		TestsChanged: m.refactored.tests, SourceChanged: m.refactored.source,
		Overridden: m.overridden,
	}
	m.last = &s
	m.stepStart = m.greenAt
	m.refactored = changes{}
	return []Event{StepDone{Step: s}}
}

func (m *Machine) complete(o Observation, s Step) []Event {
	m.n++
	s.N = m.n
	s.From, s.To = m.stepStart, o.Snapshot
	s.Failing = o.State.Failing
	c := m.refactored
	c.add(m.pending)
	s.TestsChanged, s.SourceChanged = c.tests, c.source
	s.Overridden = m.overridden
	m.stepStart = o.Snapshot
	m.pending, m.refactored = changes{}, changes{}
	m.overridden = false
	m.last = &s
	return []Event{StepDone{Step: s}}
}

func (m *Machine) baseline(o Observation) []Event {
	*m = Machine{started: true, n: m.n}
	m.stepStart, m.greenAt = o.Snapshot, o.Snapshot
	m.known = o.State.Tests
	m.failing = failingSet(o.State)
	startsRed := !o.State.Green()
	if startsRed {
		m.phase = PhaseRedInProgress
		m.startedRed = true
		m.inProgress = failingSet(o.State)
	}
	return []Event{Baseline{Snapshot: o.Snapshot, StartsRed: startsRed}}
}

// ResetBaseline makes the next observation the new baseline.
func (m *Machine) ResetBaseline() { m.started = false }

// Override sets the phase by hand; the next completed step is marked.
func (m *Machine) Override(p Phase) []Event {
	m.phase = p
	m.overridden = true
	m.brokeShown = false
	if p == PhaseGreen {
		m.inProgress = nil
	}
	return []Event{PhaseChanged{Phase: p}}
}

// diffTests compares the reported test IDs with the last known ones. When
// IDs are unknown (build broken before, exit code only), failing tests not
// failing before count as added.
func (m *Machine) diffTests(st results.TestState) (added, removed []string) {
	if st.BuildBroken {
		return nil, nil
	}
	if st.Tests == nil || m.known == nil {
		for _, f := range st.Failing {
			if !m.failing[f.ID] {
				added = append(added, f.ID)
			}
		}
		return added, nil
	}
	for _, id := range st.Tests {
		if !slices.Contains(m.known, id) {
			added = append(added, id)
		}
	}
	for _, id := range m.known {
		if !slices.Contains(st.Tests, id) {
			removed = append(removed, id)
		}
	}
	return added, removed
}

func failingSet(st results.TestState) map[string]bool {
	out := map[string]bool{}
	for _, f := range st.Failing {
		out[f.ID] = true
	}
	return out
}
