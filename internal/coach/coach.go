// Package coach turns completed steps into verdicts: exact checks right
// away, judge gates in a background worker ordered by priority.
package coach

import (
	"container/heap"
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sbradl/tdd-trainer/internal/config"
	"github.com/sbradl/tdd-trainer/internal/judge"
	"github.com/sbradl/tdd-trainer/internal/results"
	"github.com/sbradl/tdd-trainer/internal/snapshot"
	"github.com/sbradl/tdd-trainer/internal/steps"
)

type Level int

const (
	OK Level = iota
	Hint
	Warn
	Uncertain
)

func (l Level) String() string { return [...]string{"ok", "hint", "warning", "uncertain"}[l] }

// Verdict is the advisory assessment of one check of one step.
type Verdict struct {
	Step  int
	Kind  steps.Kind
	Check string // gate name, or an exact check such as "new tests"
	Level Level
	Text  string
	P     float64 // judge probability; 0 for exact checks
	Exact bool
	// Answer is the judge's confident option ID, e.g. "constant" for tpp.
	Answer string `json:",omitempty"`
}

// Store is the part of the snapshot store the coach reads.
type Store interface {
	Diff(from, to snapshot.ID) ([]snapshot.FileDiff, error)
	Files(id snapshot.ID, kind config.FileKind) (map[string]string, error)
}

// MaxTestLines is the most added test lines a small test should need.
const MaxTestLines = 15

// Coach is safe for concurrent use.
type Coach struct {
	store  Store
	scorer judge.Scorer // nil: judge disabled, exact checks only
	order  string       // tpp_order
	emit   func(Verdict)

	mu      sync.Mutex
	queue   jobQueue
	running *job
	started time.Time
	perGate time.Duration // running mean of judge time per gate
	wake    chan struct{}
	seq     int
	lastRed *redEvidence
	issue   *refactorIssue // what the last Green's review found
	next    *nextState     // coverage of the current tests, for next-test hints
	onNext  func(NextTest)

	lastN    int // the newest step, for pending gates not tied to one
	lastKind steps.Kind
}

type redEvidence struct {
	// allTests: every test file at the end of the Red. step-size and
	// cheating need the other tests too: the change must keep them green.
	allTests, transcript, source string
}

// New creates a coach; emit receives verdicts from any goroutine.
func New(store Store, scorer judge.Scorer, tppOrder string, emit func(Verdict)) *Coach {
	return &Coach{store: store, scorer: scorer, order: tppOrder, emit: emit, wake: make(chan struct{}, 1)}
}

// Pending returns the number of gates waiting for or being judged.
func (c *Coach) Pending() int { return len(c.PendingGates()) }

// PendingGate is a gate not judged yet, with an estimate of when its
// verdict arrives.
type PendingGate struct {
	Step int
	Gate string
	ETA  time.Duration
}

// PendingGates lists the gates being judged and queued, in the order the
// worker will finish them.
func (c *Coach) PendingGates() []PendingGate {
	c.mu.Lock()
	defer c.mu.Unlock()
	per := c.perGate
	if per == 0 {
		per = 3 * time.Second
	}
	var out []PendingGate
	eta := time.Duration(0)
	if c.running != nil {
		eta = time.Duration(len(c.running.gates))*per - time.Since(c.started)
		if eta < 0 {
			eta = 0
		}
		for _, g := range c.running.gates {
			out = append(out, PendingGate{c.running.step, g, eta})
		}
	}
	q := append(jobQueue{}, c.queue...)
	sort.Slice(q, func(i, j int) bool { return q.Less(i, j) })
	for _, j := range q {
		if j.cancelled {
			continue
		}
		eta += time.Duration(len(j.gates)) * per
		for _, g := range j.gates {
			out = append(out, PendingGate{j.step, g, eta})
		}
	}
	return out
}

// priority classes: lower runs first
const (
	prioRedCheck = iota
	prioNext     // a next-test hint: the learner pressed n and waits for it
	prioRed
	prioGreen
	prioRefactor
	prioLenses
)

type job struct {
	prio, step, seq int
	kind            steps.Kind
	ev              judge.Evidence
	gates           []string
	adhoc           []judge.Gate // gates built on the fly; gates holds their names
	finish          func(map[string]judge.Verdict) []Verdict
	cancelled       bool // superseded before it ran; guarded by Coach.mu
}

type jobQueue []*job

func (q jobQueue) Len() int { return len(q) }
func (q jobQueue) Less(i, j int) bool {
	if q[i].prio != q[j].prio {
		return q[i].prio < q[j].prio
	}
	return q[i].seq < q[j].seq
}
func (q jobQueue) Swap(i, j int) { q[i], q[j] = q[j], q[i] }
func (q *jobQueue) Push(x any)   { *q = append(*q, x.(*job)) }
func (q *jobQueue) Pop() any     { old := *q; j := old[len(old)-1]; *q = old[:len(old)-1]; return j }

func (c *Coach) enqueue(j *job) {
	if c.scorer == nil {
		return
	}
	c.mu.Lock()
	c.seq++
	j.seq = c.seq
	heap.Push(&c.queue, j)
	c.mu.Unlock()
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

// Run works through the judge queue until ctx ends. Pending gates of
// older steps still complete; nothing is dropped.
func (c *Coach) Run(ctx context.Context) {
	j := judge.Judge{Scorer: c.scorer}
	for {
		c.mu.Lock()
		var next *job
		for c.queue.Len() > 0 {
			next = heap.Pop(&c.queue).(*job)
			if !next.cancelled {
				c.running, c.started = next, time.Now()
				break
			}
			next = nil
		}
		c.mu.Unlock()
		if next == nil {
			select {
			case <-ctx.Done():
				return
			case <-c.wake:
				continue
			}
		}
		var vs []judge.Verdict
		var err error
		if next.adhoc != nil {
			vs, err = j.EvaluateGates(ctx, next.ev, next.adhoc)
		} else {
			vs, err = j.Evaluate(ctx, next.ev, next.gates)
		}
		c.mu.Lock()
		if err == nil {
			d := time.Since(c.started) / time.Duration(len(next.gates))
			if c.perGate == 0 {
				c.perGate = d
			} else {
				c.perGate = (3*c.perGate + d) / 4
			}
		}
		c.mu.Unlock()
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			c.mu.Lock()
			c.running = nil
			c.mu.Unlock()
			c.emit(Verdict{Step: next.step, Kind: next.kind, Check: "judge", Level: Uncertain, Text: "judge failed: " + err.Error()})
			continue
		}
		byGate := map[string]judge.Verdict{}
		for _, v := range vs {
			byGate[v.Gate] = v
		}
		// finish may queue a follow-up job; the job counts as pending until
		// then, so nobody sees an empty queue in between
		vs2 := next.finish(byGate)
		c.mu.Lock()
		c.running = nil
		c.mu.Unlock()
		for _, v := range vs2 {
			v.Step, v.Kind = next.step, next.kind
			c.emit(v)
		}
	}
}

// Step handles a completed step: exact verdicts are emitted before it
// returns, judge gates are queued.
func (c *Coach) Step(s steps.Step) error {
	c.mu.Lock()
	c.lastN, c.lastKind = s.N, s.Kind
	c.mu.Unlock()
	var diffs []snapshot.FileDiff
	if s.From != s.To {
		var err error
		if diffs, err = c.store.Diff(s.From, s.To); err != nil {
			return err
		}
	}
	testDiff, sourceDiff, testAdded := split(diffs)
	exact := func(check string, l Level, text string) {
		c.emit(Verdict{Step: s.N, Kind: s.Kind, Check: check, Level: l, Text: text, Exact: true})
	}
	for _, a := range s.Anomalies {
		exact("anomaly", Warn, anomalyText[a])
	}

	switch s.Kind {
	case steps.Red:
		if len(s.NewTests) == 1 {
			exact("new tests", OK, "Exactly one new failing test: "+s.NewTests[0]+".")
		}
		if testAdded > MaxTestLines {
			exact("test size", Hint, fmt.Sprintf("The new test adds %d lines. A smaller test makes a smaller step: can you test less at once?", testAdded))
		}
		ev := judge.Evidence{
			judge.PartTest:       addedLines(testDiff),
			judge.PartTranscript: transcript(s),
		}
		source, err := c.filesText(s.To, config.Source)
		if err != nil {
			return err
		}
		allTests, err := c.filesText(s.To, config.Test)
		if err != nil {
			return err
		}
		c.mu.Lock()
		c.lastRed = &redEvidence{allTests: allTests, transcript: ev[judge.PartTranscript], source: source}
		c.mu.Unlock()

		if allAssertions(s) {
			exact("red-check", OK, "It fails on its assertion (expected vs actual), so it proves the behaviour is missing.")
		} else {
			c.enqueue(&job{prio: prioRedCheck, step: s.N, kind: s.Kind, ev: ev, gates: []string{"red-check"}, finish: finishRedCheck})
		}
		if len(s.NewTests) == 1 { // several new tests are already an anomaly
			c.enqueue(&job{prio: prioRed, step: s.N, kind: s.Kind, ev: ev, gates: []string{"one-behaviour"}, finish: finishOneBehaviour})
		}
		c.redStarted(s)
		c.dropNext()

	case steps.Green:
		c.mu.Lock()
		red := c.lastRed
		c.mu.Unlock()
		if strings.TrimSpace(sourceDiff) == "" {
			exact("tpp", OK, "No production code changed.")
			return nil
		}
		var files []string
		for _, d := range diffs {
			if d.Kind == config.Source {
				files = append(files, d.Path)
			}
		}
		c.reviewGreen(s, sourceDiff, files)
		ev := judge.Evidence{judge.PartDiff: sourceDiff}
		gates := []string{"tpp", "multi"}
		if red != nil {
			ev[judge.PartTest] = red.allTests
			ev[judge.PartTranscript] = red.transcript
			ev[judge.PartSource] = red.source
			gates = append(gates, "cheating", "step-size")
		}
		c.enqueue(&job{prio: prioGreen, step: s.N, kind: s.Kind, ev: ev, gates: gates, finish: c.finishGreen})

	case steps.Refactor:
		exact("stayed green", OK, "All tests kept passing during the refactoring.")
		if strings.TrimSpace(sourceDiff+testDiff) == "" {
			return nil
		}
		diff := sourceDiff
		if strings.TrimSpace(diff) == "" {
			diff = testDiff // test refactoring is judged like production code
		}
		c.enqueue(&job{prio: prioRefactor, step: s.N, kind: s.Kind, ev: judge.Evidence{judge.PartDiff: diff},
			gates: []string{"structural", "refactor-effect"}, finish: finishRefactor})
	}
	return nil
}

var anomalyText = map[steps.Anomaly]string{
	steps.MultipleNewTests:  "Several new tests in one step: write one failing test at a time.",
	steps.NewTestPassed:     "A new or changed test passed without failing first: make sure it can fail, or it proves nothing.",
	steps.CodeWithoutTest:   "Production code changed without a failing test: write the test first and watch it fail.",
	steps.TestEditedInGreen: "A test was changed while making it pass: change tests in Red or Refactor, not in Green.",
	steps.BrokeExistingTest: "A test that passed before is failing now: undo the last change or get back to green first.",
}

// filesText joins the snapshot's files of one kind.
func (c *Coach) filesText(id snapshot.ID, kind config.FileKind) (string, error) {
	files, err := c.store.Files(id, kind)
	if err != nil {
		return "", err
	}
	return JoinFiles(files), nil
}

// JoinFiles joins files as the judge sees them: sorted by name, with a
// header per file when there are several.
func JoinFiles(files map[string]string) string {
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	if len(names) == 1 {
		return files[names[0]]
	}
	var b strings.Builder
	for _, n := range names {
		fmt.Fprintf(&b, "--- %s\n%s\n", n, files[n])
	}
	return b.String()
}

// split joins the patches by kind and counts added test lines.
func split(diffs []snapshot.FileDiff) (test, source string, testAdded int) {
	var t, s strings.Builder
	for _, d := range diffs {
		switch d.Kind {
		case config.Test:
			t.WriteString(d.Patch)
			testAdded += d.Added
		case config.Source:
			s.WriteString(d.Patch)
		}
	}
	return t.String(), s.String(), testAdded
}

// addedLines extracts the added lines of a unified diff: the new test.
func addedLines(patch string) string {
	var out []string
	for _, l := range strings.Split(patch, "\n") {
		if strings.HasPrefix(l, "+") && !strings.HasPrefix(l, "+++") {
			out = append(out, l[1:])
		}
	}
	return strings.Join(out, "\n")
}

func transcript(s steps.Step) string {
	var out []string
	for _, f := range s.Failing {
		if contains(s.NewTests, f.ID) && f.Message != "" {
			out = append(out, f.Message)
		}
	}
	return strings.Join(out, "\n\n")
}

// allAssertions: every new failing test is known to fail on an assertion.
func allAssertions(s steps.Step) bool {
	for _, f := range s.Failing {
		if contains(s.NewTests, f.ID) && f.Reason != results.ReasonAssertion {
			return false
		}
	}
	return true
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}
