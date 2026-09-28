package coach

import (
	"slices"

	"github.com/sbradl/tdd-trainer/internal/config"
	"github.com/sbradl/tdd-trainer/internal/steps"
)

// Resume restores what the next steps need from an earlier session of the
// same project: the last Red's evidence for the Green that follows it, or
// the last Green's open hints, re-checked as the learner refactors. done
// are its steps in order, verdicts their verdicts by step number. A review
// that never answered, or whose hint is still open, runs again; one that
// found nothing or was resolved stays closed.
func (c *Coach) Resume(done []steps.Step, verdicts map[int][]Verdict) error {
	var red, green *steps.Step
	for i := range done {
		switch done[i].Kind {
		case steps.Red:
			red, green = &done[i], nil
		case steps.Green:
			if green != nil && fixedBreak(done[:i]) {
				continue // earlier versions made fixing a broken test a Green
			}
			red, green = nil, &done[i]
		}
	}
	if red != nil {
		return c.keepRed(*red)
	}
	if green == nil {
		return nil
	}
	g := *green
	open := func(check string) bool {
		for _, v := range verdicts[g.N] {
			if v.Check == check {
				return v.Level == Hint
			}
		}
		return true // not answered before the session ended
	}
	if open(opportunity) {
		diffs, err := c.store.Diff(g.From, g.To)
		if err != nil {
			return err
		}
		_, sourceDiff, _ := split(diffs)
		var files []string
		for _, d := range diffs {
			if d.Kind == config.Source {
				files = append(files, d.Path)
			}
		}
		if sourceDiff != "" {
			c.reviewGreen(g, sourceDiff, files)
		}
	}
	if open(testOpportunity) {
		if err := c.reviewTests(g); err != nil {
			return err
		}
	}
	before, err := c.branchesAt(g.From)
	if err != nil {
		return err
	}
	after, err := c.branchesAt(g.To)
	if err != nil {
		return err
	}
	// sessions from versions that did not count branches on test values
	vs := countCheating(slices.Clone(verdicts[g.N]), before, after)
	for i, v := range vs {
		if v != verdicts[g.N][i] {
			c.emit(v)
		}
	}
	c.greenHints(g, vs, before)
	return nil
}

// fixedBreak tells whether done ends with a test broken while refactoring:
// a Green right after it only fixed that test.
func fixedBreak(done []steps.Step) bool {
	n := len(done)
	return n >= 2 && slices.Contains(done[n-1].Anomalies, steps.BrokeExistingTest) && done[n-2].Kind != steps.Red
}
