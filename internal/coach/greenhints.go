package coach

import (
	"fmt"

	"github.com/sbradl/tdd-trainer/internal/config"
	"github.com/sbradl/tdd-trainer/internal/judge"
	"github.com/sbradl/tdd-trainer/internal/snapshot"
	"github.com/sbradl/tdd-trainer/internal/steps"
)

// Hints on a Green's change itself (a simpler change would have done, or
// the code special-cases the tests' inputs) are re-checked on every green
// save while refactoring, like a refactor hint: fixing the code resolves
// them, until the next Red starts.

// greenIssue holds the last Green's open hints. Guarded by Coach.mu.
type greenIssue struct {
	green    steps.Step
	needed   string // step-size hint: the band the test needed; "" if none
	cheating bool   // test-specific code hint
	counted  bool   // it came from counting branches, not from the judge
	before   int    // branches on exact test values before the Green
	closed   bool
	recheck  *job
	last     map[string]string // check -> text of its last recheck verdict
}

// greenHints records the Green's verdicts; a hint on step size or
// test-specific code opens an issue, re-checked at once if the learner
// refactored before the verdicts arrived.
func (c *Coach) greenHints(g steps.Step, vs []Verdict, before int) {
	issue := &greenIssue{green: g, before: before, last: map[string]string{}}
	for _, v := range vs {
		switch {
		case v.Level != Hint:
		case v.Check == "step-size":
			issue.needed = v.Answer
		case v.Check == "cheating":
			issue.cheating, issue.counted = true, v.Answer == byBranches
		}
	}
	if issue.needed == "" && !issue.cheating {
		return
	}
	c.mu.Lock()
	c.greenIssue = issue
	pending := c.greenPending
	c.mu.Unlock()
	if pending != "" {
		if err := c.recheckGreen(pending); err != nil {
			c.emit(Verdict{Step: g.N, Kind: g.Kind, Check: "judge", Level: Uncertain, Text: "re-check failed: " + err.Error()})
		}
	}
}

// recheckGreen judges the Green's net change (before the Green to now)
// again for its open hints. Only the newest recheck runs.
func (c *Coach) recheckGreen(now snapshot.ID) error {
	c.mu.Lock()
	issue := c.greenIssue
	if issue == nil || issue.closed {
		c.greenPending = now // the Green's verdicts may still come
		c.mu.Unlock()
		return nil
	}
	if issue.recheck != nil {
		issue.recheck.cancelled = true
	}
	c.mu.Unlock()
	if changed, err := c.sourceDiff(issue.green.To, now); err != nil || changed == "" {
		return err // no code change since the Green was judged
	}
	net, err := c.sourceDiff(issue.green.From, now)
	if err != nil {
		return err
	}
	tests, err := c.filesText(now, config.Test)
	if err != nil {
		return err
	}
	branches, err := c.branchesAt(now)
	if err != nil {
		return err
	}
	var gates []string
	if issue.needed != "" {
		gates = append(gates, "tpp")
	}
	if issue.cheating {
		gates = append(gates, "cheating")
	}
	g := issue.green
	j := &job{prio: prioRefactor, step: g.N, kind: g.Kind, ev: judge.Evidence{judge.PartDiff: net, judge.PartTest: tests}, gates: gates,
		finish: func(vs map[string]judge.Verdict) []Verdict {
			var out []Verdict
			if tpp, ok := vs["tpp"]; ok && tpp.Answer != judge.Uncertain {
				v := Verdict{Check: "step-size", Level: OK, P: tpp.P, Answer: issue.needed, Text: fmt.Sprintf(
					"Resolved: after your refactoring the step is a %s change (%s), as simple as the test needed.",
					bandName[band[tpp.Answer]], c.tppLabel(tpp.Answer))}
				if band[tpp.Answer] > sizeBand[issue.needed] {
					v.Level, v.Text = Hint, fmt.Sprintf(
						"Still bigger than needed after your last change: the test only needed a %s change, the code makes a %s one (%s).",
						issue.needed, bandName[band[tpp.Answer]], c.tppLabel(tpp.Answer))
				}
				out = append(out, v)
			}
			if ch, ok := vs["cheating"]; ok {
				if v, ok := c.recheckedCheating(issue, ch, branches); ok {
					out = append(out, v)
				}
			}
			// report changes only, and nothing once the next Red started
			c.mu.Lock()
			defer c.mu.Unlock()
			if issue.closed {
				return nil
			}
			var changed []Verdict
			for _, v := range out {
				if issue.last[v.Check] != v.Text {
					issue.last[v.Check] = v.Text
					changed = append(changed, v)
				}
			}
			return changed
		}}
	c.mu.Lock()
	issue.recheck = j
	c.mu.Unlock()
	c.enqueue(j)
	return nil
}

// greenClosed stops rechecks once the next Red starts.
func (c *Coach) greenClosed() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.greenPending = ""
	if c.greenIssue != nil {
		c.greenIssue.closed = true
		if c.greenIssue.recheck != nil {
			c.greenIssue.recheck.cancelled = true
		}
	}
}

// recheckedCheating turns the judge's answer on the refactored code and
// the branches on exact test values now into the test-specific code
// verdict; ok is false when there is nothing new to say.
func (c *Coach) recheckedCheating(issue *greenIssue, ch judge.Verdict, branches int) (Verdict, bool) {
	resolved := Verdict{Check: "cheating", Level: OK, P: ch.P, Answer: ch.Answer,
		Text: "Resolved: after your refactoring the code no longer special-cases the tests' inputs."}
	if v, ok := countedCheating(ch.Answer, issue.before, branches); ok {
		if v.Level == Hint {
			v.Text = fmt.Sprintf("Still special-cases %d test values after your last change: generalise instead of adding a branch per example.", branches)
		}
		return v, true
	}
	switch {
	case ch.Answer == "yes":
		return Verdict{Check: "cheating", Level: Hint, P: ch.P, Answer: ch.Answer,
			Text: "Still special-cases the tests' inputs after your last change: generalise instead of matching test values."}, true
	case ch.Answer == "no":
		return resolved, true
	case issue.counted:
		return resolved, true // the branches that raised the hint are gone
	}
	return Verdict{}, false
}
