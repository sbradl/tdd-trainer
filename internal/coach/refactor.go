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

// A refactor opportunity is found right after a Green, re-checked on every
// green save while refactoring, and reported as missed if the next Red
// starts before it is resolved.

// refactorIssue is the review result of the last Green. Guarded by Coach.mu.
type refactorIssue struct {
	green    steps.Step
	where    string   // the production files the Green changed, e.g. "roman.go"
	reviewed bool     // the lenses have answered
	lenses   []string // lenses that found something; empty: nothing to do
	problem  string   // what to refactor, for the texts
	problems []string // the problems named in it, re-checked one by one
	resolved bool
	closed   bool        // the next Red started; no more rechecks
	redStep  *steps.Step // the Red that closed it, before the review answered
	recheck  *job
	last     string      // text of the last recheck verdict, to report changes only
	pending  snapshot.ID // refactored code seen before the review answered
}

const opportunity = "refactor opportunity"

// reviewGreen queues the review lenses on a finished Green's production
// code diff; files are the production files it changed.
func (c *Coach) reviewGreen(g steps.Step, diff string, files []string) {
	issue := &refactorIssue{green: g, where: where(files)}
	c.mu.Lock()
	c.issue = issue
	c.mu.Unlock()
	ev := judge.Evidence{judge.PartDiff: diff}
	c.enqueue(&job{prio: prioRefactor, step: g.N, kind: g.Kind, ev: ev, gates: judge.LensNames(),
		finish: func(vs map[string]judge.Verdict) []Verdict {
			var found []string
			maxP := 0.0
			for _, name := range judge.LensNames() {
				if v := vs[name]; v.Answer == "yes" {
					found = append(found, name)
					maxP = max(maxP, v.P)
				}
			}
			if len(found) == 0 {
				return c.reviewed(issue, nil, "", nil, Verdict{Check: opportunity, Level: OK, Text: "Nothing worth refactoring in this Green."})
			}
			which := make([]string, len(found))
			for i, n := range found {
				which[i] = n + "~which"
			}
			c.enqueue(&job{prio: prioRefactor, step: g.N, kind: g.Kind, ev: ev, gates: which,
				finish: func(ws map[string]judge.Verdict) []Verdict {
					problem, problems := problemText(found, ws)
					return c.reviewed(issue, found, problem, problems, Verdict{Check: opportunity, Level: Hint, P: maxP, Text: fmt.Sprintf(
						"Worth refactoring %s now, while the tests are green: %s The hint updates as you refactor (tddt show %d).", issue.where, problem, g.N)})
				}})
			return nil
		}})
}

// reviewed records the review result; if the next Red already started,
// the missed-refactor verdict for it follows too.
func (c *Coach) reviewed(issue *refactorIssue, lenses []string, problem string, problems []string, v Verdict) []Verdict {
	c.mu.Lock()
	issue.reviewed, issue.lenses, issue.problem, issue.problems = true, lenses, problem, problems
	red, pending := issue.redStep, issue.pending
	c.mu.Unlock()
	if red != nil {
		mv := missedVerdict(issue)
		mv.Step, mv.Kind = red.N, red.Kind
		c.emit(mv)
	}
	if pending != "" && red == nil {
		if err := c.Recheck(pending); err != nil {
			c.emit(Verdict{Step: issue.green.N, Kind: issue.green.Kind, Check: "judge", Level: Uncertain, Text: "re-check failed: " + err.Error()})
		}
	}
	return []Verdict{v}
}

// Recheck judges whether the refactoring since the flagged code removed
// the problems the review found; after a green save while refactoring.
// First it asks per problem whether the change removed it. If not all
// surely are, the lenses that raised the hint look again at the Green's
// net change (before the Green to now). Every recheck ends in a verdict
// the learner sees. Only the newest recheck runs.
func (c *Coach) Recheck(now snapshot.ID) error {
	c.mu.Lock()
	issue := c.issue
	if issue != nil && !issue.reviewed && !issue.closed {
		issue.pending = now // re-check once the review has answered
	}
	if issue == nil || !issue.reviewed || issue.closed || len(issue.problems) == 0 {
		c.mu.Unlock()
		return nil
	}
	g, problems, lenses := issue.green, issue.problems, issue.lenses
	if issue.recheck != nil {
		issue.recheck.cancelled = true
	}
	c.mu.Unlock()

	diff, err := c.sourceDiff(g.To, now)
	if err != nil || diff == "" {
		return err // or back to the flagged code: nothing new to judge
	}
	net, err := c.sourceDiff(g.From, now)
	if err != nil {
		return err
	}
	var gates []judge.Gate
	var names []string
	for i, p := range problems {
		fg := judge.FixedGate(p)
		fg.Name = fmt.Sprintf("refactor-fixed~%d", i)
		gates = append(gates, fg)
		names = append(names, fg.Name)
	}
	// verdict turns a result into what the learner sees; nil: unchanged
	verdict := func(v Verdict, resolved bool) []Verdict {
		c.mu.Lock()
		defer c.mu.Unlock()
		if issue.closed || v.Text == issue.last {
			return nil // the next Red started meanwhile, or nothing new
		}
		issue.resolved, issue.last = resolved, v.Text
		return []Verdict{v}
	}
	resolved := Verdict{Check: opportunity, Level: OK, Text: "Resolved: your refactoring removed " + lowerFirst(issue.problem)}
	lensJob := &job{prio: prioRefactor, step: g.N, kind: g.Kind, ev: judge.Evidence{judge.PartDiff: net}, gates: lenses,
		finish: func(vs map[string]judge.Verdict) []Verdict {
			clean, found := 0, false
			for _, n := range lenses {
				switch vs[n].Answer {
				case "no":
					clean++
				case "yes":
					found = true
				}
			}
			switch {
			case clean == len(lenses):
				return verdict(resolved, true)
			case found:
				return verdict(Verdict{Check: opportunity, Level: Hint, Text: fmt.Sprintf(
					"Still there in %s after your last change: %s (tddt show %d)", issue.where, issue.problem, g.N)}, false)
			}
			return verdict(Verdict{Check: opportunity, Level: OK, Text: "Probably resolved: after your refactoring the review no longer clearly finds " +
				lowerFirst(issue.problem)}, true)
		}}
	var j *job
	j = &job{prio: prioRefactor, step: g.N, kind: g.Kind, ev: judge.Evidence{judge.PartDiff: diff}, gates: names, adhoc: gates,
		finish: func(vs map[string]judge.Verdict) []Verdict {
			fixed := 0
			for _, n := range names {
				if vs[n].Answer == "yes" {
					fixed++
				}
			}
			if fixed == len(names) {
				return verdict(resolved, true)
			}
			c.mu.Lock()
			current := issue.recheck == j && !issue.closed // no newer recheck started
			if current {
				issue.recheck = lensJob
			}
			c.mu.Unlock()
			if current {
				c.enqueue(lensJob)
			}
			return nil
		}}
	c.mu.Lock()
	issue.recheck = j
	c.mu.Unlock()
	c.enqueue(j)
	return nil
}

// sourceDiff joins the production code patches between two snapshots.
func (c *Coach) sourceDiff(from, to snapshot.ID) (string, error) {
	diffs, err := c.store.Diff(from, to)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, d := range diffs {
		if d.Kind == config.Source {
			b.WriteString(d.Patch)
		}
	}
	return b.String(), nil
}

// redStarted closes the last Green's issue: unresolved means missed.
func (c *Coach) redStarted(red steps.Step) {
	c.mu.Lock()
	issue := c.issue
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
	v := missedVerdict(issue)
	v.Step, v.Kind = red.N, red.Kind
	c.emit(v)
}

func missedVerdict(issue *refactorIssue) Verdict {
	switch {
	case len(issue.lenses) == 0:
		return Verdict{Check: "missed refactor", Level: OK, Text: "Nothing needed refactoring before this test."}
	case issue.resolved:
		return Verdict{Check: "missed refactor", Level: OK, Text: fmt.Sprintf("You cleaned up after step %d before writing this test.", issue.green.N)}
	}
	return Verdict{Check: "missed refactor", Level: Hint, Text: fmt.Sprintf(
		"You started this test without cleaning up %s after step %d: %s Refactor once this test passes (tddt show %d).", issue.where, issue.green.N, issue.problem, issue.green.N)}
}

// problemText names the one or two clearest problems the lenses found,
// and returns their descriptions for re-checking.
func problemText(lenses []string, which map[string]judge.Verdict) (string, []string) {
	type problem struct {
		desc string
		p    float64
		sure bool
	}
	var ps []problem
	seen := map[string]bool{}
	for _, name := range lenses {
		w := which[name+"~which"]
		for _, o := range judge.LensProblems[name] {
			if o.ID == w.Top && !seen[o.Desc] {
				seen[o.Desc] = true
				ps = append(ps, problem{o.Desc, w.P, w.Answer != judge.Uncertain})
			}
		}
	}
	sort.SliceStable(ps, func(i, j int) bool {
		if ps[i].sure != ps[j].sure {
			return ps[i].sure
		}
		return ps[i].p > ps[j].p
	})
	if len(ps) > 2 {
		ps = ps[:2]
	}
	if len(ps) == 2 && ps[0].sure && !ps[1].sure {
		ps = ps[:1]
	}
	var descs []string
	for _, x := range ps {
		descs = append(descs, x.desc)
	}
	switch len(ps) {
	case 0:
		return "the review found something to clean up.", nil
	case 1:
		if !ps[0].sure {
			return "probably " + lowerFirst(ps[0].desc), descs
		}
		return ps[0].desc, descs
	}
	var names []string
	for _, x := range ps {
		short, _, _ := strings.Cut(x.desc, ":")
		names = append(names, lowerFirst(strings.TrimSuffix(short, ".")))
	}
	return strings.Join(names, ", and ") + ".", descs
}

// where names the production files a hint is about, so nobody looks for
// the problem in the tests.
func where(files []string) string { return fileNames(files, "the production code") }

// fileNames names one or two files, else the fallback.
func fileNames(files []string, fallback string) string {
	switch len(files) {
	case 1:
		return files[0]
	case 2:
		return files[0] + " and " + files[1]
	}
	return fallback
}
