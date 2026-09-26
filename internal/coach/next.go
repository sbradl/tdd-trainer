package coach

import (
	"context"

	"github.com/sbradl/tdd-trainer/internal/config"
	"github.com/sbradl/tdd-trainer/internal/judge"
	"github.com/sbradl/tdd-trainer/internal/snapshot"
	"github.com/sbradl/tdd-trainer/internal/steps"
)

// A next-test hint answers "what next?" (key n) while the tests are green.
// The coverage gates look for the first ZOMBIES case no test checks yet;
// right after a fake-it Green, triangulating comes first. Each request
// reveals more: stage 1 names the kind of test, stage 2 the case.

// NextTest is the answer to one request.
type NextTest struct {
	Stage   int    // 1 or 2; 0 for a message that is no hint (wrong phase, judge off)
	Case    string // a judge.Cases ID, "triangulate", "complete" or "" (no clear gap)
	Text    string
	Pending bool // the judge is still looking; the hint follows
}

// Case IDs besides judge.Cases.
const (
	Triangulate = "triangulate"
	Complete    = "complete"
)

// CaseName is how the report names a hint's case.
var CaseName = map[string]string{
	"zero": "zero/empty", "one": "one", "many": "many", "boundary": "boundaries", "error": "errors",
	Triangulate: "triangulate", Complete: "complete", "": "no clear gap",
}

var caseText = map[string][2]string{
	"zero":     {"Try the degenerate case.", "No test yet for the zero or empty case: 0, an empty string or list, nothing at all. It is usually the simplest test to start with."},
	"one":      {"Try the simplest real example.", "No test yet with a single element: one number, one item, one word."},
	"many":     {"Try more than one.", "No test yet with several items, or with a value built from several parts."},
	"boundary": {"Try an edge case.", "No test yet at an edge where the result switches from one rule to another: just below, at and just above a limit, or a special rule that takes over."},
	"error":    {"Try an error case.", "No test yet for invalid input or a failure: a negative number, malformed text, an out-of-range value, an operation on something empty."},
	Triangulate: {"Triangulate: add a second example.",
		"Your last Green made the test pass with a fixed value (Nil → Constant). Add an example with a different expected result, so a constant can no longer pass and the code has to generalise."},
	Complete: {"Looks complete: every case has a test. Refactor, or stop here.", "Looks complete: every case has a test. Refactor, or stop here."},
	"":       {"No clear gap. Is there a case you haven't tried yet?", "No clear gap. Is there a case you haven't tried yet?"},
}

const jumpNote = " Your last Green made a bigger jump than its test needed: pick a test closer to the current code."

// greenSignals are what the last Green's verdicts say about the next test.
type greenSignals struct{ fake, jump bool }

func signals(green []Verdict) greenSignals {
	var s greenSignals
	for _, v := range green {
		switch {
		case v.Check == "tpp" && (v.Answer == "nil" || v.Answer == "constant"):
			s.fake = true
		case v.Check == "multi" && v.Answer == "yes", v.Check == "step-size" && v.Level == Hint:
			s.jump = true
		}
	}
	return s
}

// coverage is what the coverage gates found for one set of tests.
type coverage struct {
	gap      string // first uncovered case, "" if none is sure
	complete bool   // every case is surely covered or not applicable
	tpp      string // transformation the gap's test probably needs; "" if unsure
}

// findGap picks the first case, in ZOMBIES order, that both probes call
// uncovered. Uncertain answers never become a hint.
func findGap(vs map[string]judge.Verdict) coverage {
	cov := coverage{complete: true}
	for _, c := range judge.Cases {
		switch vs["covers-"+c.ID].Answer {
		case "no":
			if cov.gap == "" {
				cov.gap = c.ID
			}
			cov.complete = false
		case "yes", "na":
		default:
			cov.complete = false
		}
	}
	return cov
}

// renderNext builds the hint for a stage. refactor is the open Refactor
// now problem, if any.
func renderNext(stage int, g greenSignals, cov coverage, order, refactor string) NextTest {
	id := cov.gap
	switch {
	case g.fake:
		id = Triangulate
	case cov.complete:
		id = Complete
	}
	t := caseText[id]
	text := t[0]
	if stage >= 2 {
		stage = 2
		text = t[1]
		if cov.tpp != "" && id == cov.gap && id != "" {
			text += " The simplest code for it probably needs " + tppLabel(order, cov.tpp) + "."
		}
		if g.jump {
			text += jumpNote
		}
	}
	if refactor != "" {
		text += " But first, Refactor now is still open: " + refactor
	}
	return NextTest{Stage: stage, Case: id, Text: text}
}

// nextState is the coverage of the current tests and how far the hint
// was revealed. Guarded by Coach.mu.
type nextState struct {
	tests    string // the stage and coverage belong to these tests
	stage    int
	cov      *coverage // nil until judged
	job      *job
	green    greenSignals
	refactor string
}

var coverGates = func() []string {
	var out []string
	for _, c := range judge.Cases {
		out = append(out, "covers-"+c.ID)
	}
	return out
}()

// SetNextTestSink sets where next-test hints go; call before NextTest.
func (c *Coach) SetNextTestSink(f func(NextTest)) { c.onNext = f }

func (c *Coach) sendNext(n NextTest) {
	if c.onNext != nil {
		c.onNext(n)
	}
}

// NextTest answers a "what next?" request for the code at now. green is
// the last Green's verdicts. The hint goes to the sink, now or once the
// judge has answered.
func (c *Coach) NextTest(now snapshot.ID, phase steps.Phase, green []Verdict) error {
	switch {
	case phase == steps.PhaseGreen:
		c.sendNext(NextTest{Text: "Finish the current step first: make the failing test pass."})
		return nil
	case phase == steps.PhaseRedInProgress:
		c.sendNext(NextTest{Text: "Finish the current step first: get the new test to fail on its assertion."})
		return nil
	case c.scorer == nil:
		c.sendNext(NextTest{Text: "Next-test hints need the judge, and it is off."})
		return nil
	}
	st, err := c.prepareNext(now)
	if err != nil {
		return err
	}
	c.mu.Lock()
	st.stage = min(st.stage+1, 2)
	st.green = signals(green)
	st.refactor = ""
	if is := c.issue; is != nil && is.reviewed && len(is.lenses) > 0 && !is.resolved && !is.closed {
		st.refactor = is.problem
	}
	var n NextTest
	if st.cov != nil {
		n = renderNext(st.stage, st.green, *st.cov, c.order, st.refactor)
	} else {
		n = NextTest{Stage: st.stage, Pending: true, Text: "Looking at your tests…"}
	}
	c.mu.Unlock()
	c.sendNext(n)
	return nil
}

// prepareNext starts judging the coverage of the tests at id, unless it is
// known or being judged already. Refactoring keeps behaviour, so the
// coverage stays valid until the tests change.
func (c *Coach) prepareNext(id snapshot.ID) (*nextState, error) {
	if c.scorer == nil {
		return nil, nil
	}
	tests, err := c.filesText(id, config.Test)
	if err != nil {
		return nil, err
	}
	source, err := c.filesText(id, config.Source)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	if st := c.next; st != nil && st.tests == tests {
		c.mu.Unlock()
		return st, nil
	}
	if c.next != nil && c.next.job != nil {
		c.next.job.cancelled = true
	}
	st := &nextState{tests: tests}
	c.next = st
	step, kind := c.lastN, c.lastKind
	ev := judge.Evidence{judge.PartTest: tests, judge.PartSource: source}
	st.job = &job{prio: prioNext, step: step, kind: kind, ev: ev, gates: coverGates,
		finish: func(vs map[string]judge.Verdict) []Verdict {
			cov := findGap(vs)
			gc, ok := caseByID(cov.gap)
			if !ok {
				c.nextJudged(st, cov)
				return nil
			}
			g := judge.NextTPPGate(gc)
			follow := &job{prio: prioNext, step: step, kind: kind, ev: ev, gates: []string{g.Name}, adhoc: []judge.Gate{g},
				finish: func(ts map[string]judge.Verdict) []Verdict {
					if a := ts[g.Name].Answer; a != judge.Uncertain {
						cov.tpp = a
					}
					c.nextJudged(st, cov)
					return nil
				}}
			c.mu.Lock()
			current := c.next == st
			if current {
				st.job = follow
			}
			c.mu.Unlock()
			if current {
				c.enqueue(follow)
			}
			return nil
		}}
	j := st.job
	c.mu.Unlock()
	c.enqueue(j)
	return st, nil
}

func caseByID(id string) (judge.Case, bool) {
	for _, x := range judge.Cases {
		if x.ID == id {
			return x, true
		}
	}
	return judge.Case{}, false
}

// nextJudged stores the coverage; a request waiting for it gets its hint.
func (c *Coach) nextJudged(st *nextState, cov coverage) {
	c.mu.Lock()
	st.cov, st.job = &cov, nil
	waiting := c.next == st && st.stage > 0
	n := renderNext(st.stage, st.green, cov, c.order, st.refactor)
	c.mu.Unlock()
	if waiting {
		c.sendNext(n)
	}
}

// dropNext forgets the coverage once a new test is written.
func (c *Coach) dropNext() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.next != nil && c.next.job != nil {
		c.next.job.cancelled = true
	}
	c.next = nil
}

// NextTestNow judges a hint at full detail right away, for `tddt next`.
func NextTestNow(ctx context.Context, sc judge.Scorer, tests, source string, green []Verdict, order string) (NextTest, error) {
	j := judge.Judge{Scorer: sc}
	ev := judge.Evidence{judge.PartTest: tests, judge.PartSource: source}
	vs, err := j.Evaluate(ctx, ev, coverGates)
	if err != nil {
		return NextTest{}, err
	}
	byGate := map[string]judge.Verdict{}
	for _, v := range vs {
		byGate[v.Gate] = v
	}
	cov := findGap(byGate)
	if gc, ok := caseByID(cov.gap); ok {
		ts, err := j.EvaluateGates(ctx, ev, []judge.Gate{judge.NextTPPGate(gc)})
		if err != nil {
			return NextTest{}, err
		}
		if ts[0].Answer != judge.Uncertain {
			cov.tpp = ts[0].Answer
		}
	}
	return renderNext(2, signals(green), cov, order, ""), nil
}
