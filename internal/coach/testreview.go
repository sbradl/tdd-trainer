package coach

import (
	"fmt"
	"sort"
	"strings"

	"github.com/sbradl/tdd-trainer/internal/config"
	"github.com/sbradl/tdd-trainer/internal/judge"
	"github.com/sbradl/tdd-trainer/internal/snapshot"
	"github.com/sbradl/tdd-trainer/internal/steps"
)

// The test code is reviewed right after each Green like the production
// code, with the test-smell gates instead of the lenses: literals and
// repeated values are normal in tests. The smells are judged on the whole
// test code, so a re-check simply asks the flagged ones again.

const (
	testOpportunity = "test refactor opportunity"
	missedTest      = "missed test refactor"
)

// testIssue is the test review of the last Green. Guarded by Coach.mu.
type testIssue struct {
	green    steps.Step
	where    string // the test files, e.g. "roman_test.go"
	reviewed bool
	smells   []judge.TestSmell // found; empty: nothing to do
	problem  string
	resolved bool
	closed   bool        // the next Red started; no more rechecks
	redStep  *steps.Step // the Red that closed it, before the review answered
	recheck  *job
	last     string      // text of the last recheck verdict, to report changes only
	pending  snapshot.ID // refactored tests seen before the review answered
}

func smellGates(smells []judge.TestSmell) []string {
	var out []string
	for _, s := range smells {
		out = append(out, s.ID)
	}
	return out
}

// reviewTests queues the test-smell gates on the tests at a finished Green.
func (c *Coach) reviewTests(g steps.Step) error {
	files, err := c.store.Files(g.To, config.Test)
	if err != nil || len(files) == 0 {
		return err
	}
	var names []string
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	issue := &testIssue{green: g, where: fileNames(names, "the tests")}
	c.mu.Lock()
	c.testIssue = issue
	c.mu.Unlock()
	c.enqueue(&job{prio: prioRefactor, step: g.N, kind: g.Kind, ev: judge.Evidence{judge.PartTest: JoinFiles(files)},
		gates: smellGates(judge.TestSmells),
		finish: func(vs map[string]judge.Verdict) []Verdict {
			var found []judge.TestSmell
			maxP := 0.0
			for _, s := range judge.TestSmells {
				if v := vs[s.ID]; v.Answer == "yes" {
					found = append(found, s)
					maxP = max(maxP, v.P)
				}
			}
			c.mu.Lock()
			issue.reviewed, issue.smells, issue.problem = true, found, smellText(found)
			red, pending := issue.redStep, issue.pending
			c.mu.Unlock()
			if red != nil {
				mv := missedTestVerdict(issue)
				mv.Step, mv.Kind = red.N, red.Kind
				c.emit(mv)
			}
			if pending != "" && red == nil {
				if err := c.RecheckTests(pending); err != nil {
					c.emit(Verdict{Step: g.N, Kind: g.Kind, Check: "judge", Level: Uncertain, Text: "re-check failed: " + err.Error()})
				}
			}
			if len(found) == 0 {
				return []Verdict{{Check: testOpportunity, Level: OK, Text: "Nothing worth refactoring in the tests."}}
			}
			return []Verdict{{Check: testOpportunity, Level: Hint, P: maxP, Text: fmt.Sprintf(
				"Worth refactoring %s now, while the tests are green: %s The hint updates as you refactor the tests.", issue.where, issue.problem)}}
		}})
	return nil
}

// smellText names the problems found: one in full, several by name.
func smellText(found []judge.TestSmell) string {
	switch len(found) {
	case 0:
		return ""
	case 1:
		return found[0].Problem
	}
	var names []string
	for _, s := range found {
		short, _, _ := strings.Cut(s.Problem, ":")
		names = append(names, lowerFirst(short))
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1] + "."
}

// RecheckTests asks the flagged test smells again on the tests at now;
// after a green save that changed tests while refactoring. Only the
// newest recheck runs.
func (c *Coach) RecheckTests(now snapshot.ID) error {
	c.mu.Lock()
	issue := c.testIssue
	if issue != nil && !issue.reviewed && !issue.closed {
		issue.pending = now // re-check once the review has answered
	}
	if issue == nil || !issue.reviewed || issue.closed || len(issue.smells) == 0 {
		c.mu.Unlock()
		return nil
	}
	if issue.recheck != nil {
		issue.recheck.cancelled = true
	}
	gates := smellGates(issue.smells)
	c.mu.Unlock()

	tests, err := c.filesText(now, config.Test)
	if err != nil {
		return err
	}
	g := issue.green
	j := &job{prio: prioRefactor, step: g.N, kind: g.Kind, ev: judge.Evidence{judge.PartTest: tests}, gates: gates,
		finish: func(vs map[string]judge.Verdict) []Verdict {
			clean, found := 0, false
			for _, n := range gates {
				switch vs[n].Answer {
				case "no":
					clean++
				case "yes":
					found = true
				}
			}
			var v Verdict
			resolved := true
			switch {
			case clean == len(gates):
				v = Verdict{Check: testOpportunity, Level: OK, Text: "Resolved: your refactoring removed " + lowerFirst(issue.problem)}
			case found:
				resolved = false
				v = Verdict{Check: testOpportunity, Level: Hint, Text: fmt.Sprintf("Still there in %s after your last change: %s", issue.where, issue.problem)}
			default:
				v = Verdict{Check: testOpportunity, Level: OK, Text: "Probably resolved: after your refactoring the review no longer clearly finds " + lowerFirst(issue.problem)}
			}
			c.mu.Lock()
			defer c.mu.Unlock()
			if issue.closed || v.Text == issue.last {
				return nil
			}
			issue.resolved, issue.last = resolved, v.Text
			return []Verdict{v}
		}}
	c.mu.Lock()
	issue.recheck = j
	c.mu.Unlock()
	c.enqueue(j)
	return nil
}

// testRedStarted closes the last Green's test issue: unresolved means missed.
func (c *Coach) testRedStarted(red steps.Step) {
	c.mu.Lock()
	issue := c.testIssue
	if issue == nil || issue.closed {
		c.mu.Unlock()
		return
	}
	issue.closed = true
	if issue.recheck != nil {
		issue.recheck.cancelled = true
	}
	if !issue.reviewed {
		issue.redStep = &red // the review will report it
		c.mu.Unlock()
		return
	}
	c.mu.Unlock()
	v := missedTestVerdict(issue)
	v.Step, v.Kind = red.N, red.Kind
	c.emit(v)
}

func missedTestVerdict(issue *testIssue) Verdict {
	switch {
	case len(issue.smells) == 0:
		return Verdict{Check: missedTest, Level: OK, Text: "Nothing in the tests needed refactoring before this test."}
	case issue.resolved:
		return Verdict{Check: missedTest, Level: OK, Text: fmt.Sprintf("You cleaned up the tests after step %d before writing this one.", issue.green.N)}
	}
	return Verdict{Check: missedTest, Level: Hint, Text: fmt.Sprintf(
		"You started this test without cleaning up %s after step %d: %s Refactor the tests once this one passes.", issue.where, issue.green.N, issue.problem)}
}
