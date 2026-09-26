// Package report writes the Markdown session report and the short
// summary printed on quit.
package report

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/sbradl/tdd-trainer/internal/app"
	"github.com/sbradl/tdd-trainer/internal/coach"
	"github.com/sbradl/tdd-trainer/internal/snapshot"
	"github.com/sbradl/tdd-trainer/internal/steps"
)

// Differ returns the changes of a step, for diff snippets.
type Differ interface {
	Diff(from, to snapshot.ID) ([]snapshot.FileDiff, error)
}

// Dir is where reports go, relative to the project root.
var Dir = filepath.Join(snapshot.Dir, "reports")

// cycle groups the steps from one Red up to the next; cycle 0 holds what
// happened before the first Red.
type cycle struct {
	n     int
	steps []app.StepRecord
}

func cycles(h app.History) []cycle {
	var out []cycle
	cur := cycle{}
	for _, r := range h.Steps {
		if r.Step.Kind == steps.Red {
			if len(cur.steps) > 0 {
				out = append(out, cur)
			}
			cur = cycle{n: cur.n + 1}
		}
		cur.steps = append(cur.steps, r)
	}
	if len(cur.steps) > 0 {
		out = append(out, cur)
	}
	return out
}

// clean: a full Red→Green cycle without hints, warnings or anomalies.
func (c cycle) clean() bool {
	hasGreen := false
	for _, r := range c.steps {
		hasGreen = hasGreen || r.Step.Kind == steps.Green
		if len(r.Step.Anomalies) > 0 {
			return false
		}
		for _, v := range r.Verdicts {
			if v.Level == coach.Hint || v.Level == coach.Warn {
				return false
			}
		}
	}
	return c.n > 0 && hasGreen
}

// Stats are the header numbers.
type Stats struct {
	Duration             time.Duration
	Cycles, Clean, Tests int
	Pending              int // gates not judged when the report was written
}

func stats(h app.History, pending int) Stats {
	s := Stats{Duration: h.End.Sub(h.Start).Round(time.Second), Pending: pending}
	tests := map[string]bool{}
	for _, c := range cycles(h) {
		if c.n == 0 {
			continue
		}
		s.Cycles++
		if c.clean() {
			s.Clean++
		}
	}
	for _, r := range h.Steps {
		if r.Step.Kind == steps.Red || r.Step.Kind == steps.AnomalyStep {
			for _, t := range r.Step.NewTests {
				tests[t] = true
			}
		}
	}
	s.Tests = len(tests)
	return s
}

func (s Stats) cleanShare() string {
	if s.Cycles == 0 {
		return "–"
	}
	return fmt.Sprintf("%d of %d (%d%%)", s.Clean, s.Cycles, 100*s.Clean/s.Cycles)
}

type tip struct {
	check, text string
	n           int
}

// tips are the most frequent hints and warnings, most frequent first.
func tips(h app.History) []tip {
	byCheck := map[string]*tip{}
	for _, r := range h.Steps {
		for _, v := range r.Verdicts {
			if v.Level != coach.Hint && v.Level != coach.Warn {
				continue
			}
			key := v.Check
			if key == "anomaly" {
				key = v.Text
			}
			t := byCheck[key]
			if t == nil {
				t = &tip{check: v.Check}
				byCheck[key] = t
			}
			t.n++
			t.text = v.Text
		}
	}
	var out []tip
	for _, t := range byCheck {
		out = append(out, *t)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].n != out[j].n {
			return out[i].n > out[j].n
		}
		return out[i].check < out[j].check
	})
	if len(out) > 3 {
		out = out[:3]
	}
	return out
}

// nextTestTip sums up the next-test hints the learner asked for; "" if none.
func nextTestTip(h app.History) string {
	if len(h.NextTests) == 0 {
		return ""
	}
	n := map[string]int{}
	top := ""
	for _, r := range h.NextTests {
		if r.Case == coach.Complete || r.Case == "" {
			continue // no gap found: nothing to practise
		}
		n[r.Case]++
		if top == "" || n[r.Case] > n[top] || (n[r.Case] == n[top] && r.Case < top) {
			top = r.Case
		}
	}
	text := "You asked which test to write next"
	if top != "" {
		text += fmt.Sprintf(", mostly: %s (%d×)", coach.CaseName[top], n[top])
	}
	return text + ". Before asking, go through zero, one, many, boundaries and errors yourself."
}

func icon(l coach.Level) string {
	return map[coach.Level]string{coach.OK: "✓", coach.Hint: "➜", coach.Warn: "⚠", coach.Uncertain: "?"}[l]
}

// Render builds the Markdown report. d may be nil (no diff snippets).
func Render(h app.History, pending int, d Differ) string {
	var b strings.Builder
	s := stats(h, pending)
	fmt.Fprintf(&b, "# TDD session report — %s\n\n", h.Start.Format("2006-01-02 15:04"))
	fmt.Fprintf(&b, "| Duration | Cycles | Tests added | Clean cycles |\n|---|---|---|---|\n| %s | %d | %d | %s |\n\n",
		s.Duration, s.Cycles, s.Tests, s.cleanShare())
	if h.StartsRed {
		b.WriteString("> The session started with failing tests; the first steps were treated as Red in progress.\n\n")
	}
	if h.ExitCodeOnly {
		b.WriteString("> Test results were read from the exit code only; verdicts are weaker.\n\n")
	}
	if s.Pending > 0 {
		fmt.Fprintf(&b, "> %d gates were still being judged when this report was written.\n\n", s.Pending)
	}

	b.WriteString("## Focus tips\n\n")
	ts := tips(h)
	nt := nextTestTip(h)
	if len(ts) == 0 && nt == "" {
		b.WriteString("Nothing stood out. Keep going.\n\n")
	}
	for i, t := range ts {
		fmt.Fprintf(&b, "%d. **%s** (%d×): %s\n", i+1, coach.Label(t.check), t.n, t.text)
	}
	if nt != "" {
		fmt.Fprintf(&b, "%d. **Next test** (%d×): %s\n", len(ts)+1, len(h.NextTests), nt)
	}
	if len(ts) > 0 || nt != "" {
		b.WriteString("\n")
	}

	used := map[string]bool{}
	b.WriteString("## Cycles\n\n")
	if len(h.Steps) == 0 {
		b.WriteString("No steps yet.\n\n")
	}
	for _, c := range cycles(h) {
		if c.n == 0 {
			b.WriteString("### Before the first Red\n\n")
		} else {
			fmt.Fprintf(&b, "### Cycle %d\n\n", c.n)
		}
		b.WriteString("| Step | Kind | Verdicts |\n|---|---|---|\n")
		for _, r := range c.steps {
			var vs []string
			for _, v := range r.Verdicts {
				if v.Level == coach.Uncertain {
					continue // listed in their own section
				}
				used[v.Check] = true
				vs = append(vs, icon(v.Level)+" **"+coach.Label(v.Check)+"**: "+cell(v.Text))
			}
			kind := r.Step.Kind.String()
			if len(r.Step.NewTests) > 0 {
				kind += " — " + strings.Join(r.Step.NewTests, ", ")
			}
			if r.Step.Overridden {
				kind += " (phase set by hand)"
			}
			fmt.Fprintf(&b, "| %d | %s | %s |\n", r.Step.N, cell(kind), strings.Join(vs, "<br>"))
		}
		b.WriteString("\n")
		if d != nil {
			snippets(&b, h, c, d)
		}
	}

	b.WriteString("## Transformation path\n\n")
	var path []string
	for _, r := range h.Steps {
		for _, v := range r.Verdicts {
			if v.Check == "tpp" && v.Level == coach.OK && v.Text != "" && v.Text != "no production code changed" {
				path = append(path, fmt.Sprintf("%d: %s", r.Step.N, strings.TrimSuffix(strings.TrimPrefix(v.Text, "Applied "), ".")))
			}
		}
	}
	if len(path) == 0 {
		b.WriteString("No transformations judged.\n\n")
	} else {
		b.WriteString(strings.Join(path, " → ") + "\n\n")
	}

	b.WriteString("## Anomalies and missed refactors\n\n")
	n := 0
	for _, r := range h.Steps {
		for _, v := range r.Verdicts {
			if v.Check == "anomaly" || ((v.Check == "missed refactor" || v.Check == "missed test refactor") && v.Level == coach.Hint) {
				fmt.Fprintf(&b, "- Step %d (%s): %s\n", r.Step.N, r.Step.Kind, v.Text)
				n++
			}
		}
	}
	if n == 0 {
		b.WriteString("None.\n")
	}
	if len(h.NextTests) > 0 {
		b.WriteString("\n## Next-test hints\n\n")
		cycleOf := map[int]int{}
		for _, c := range cycles(h) {
			for _, r := range c.steps {
				cycleOf[r.Step.N] = c.n
			}
		}
		for _, r := range h.NextTests {
			where := "Before the first step"
			if r.Step > 0 {
				where = fmt.Sprintf("After step %d (cycle %d)", r.Step, cycleOf[r.Step])
			}
			fmt.Fprintf(&b, "- %s: %s, stage %d\n", where, coach.CaseName[r.Case], r.Stage)
		}
	}
	b.WriteString("\n## Uncertain verdicts\n\nThe judge was not sure about these; they were not shown during the session.\n\n")
	n = 0
	for _, r := range h.Steps {
		for _, v := range r.Verdicts {
			if v.Level == coach.Uncertain {
				used[v.Check] = true
				fmt.Fprintf(&b, "- Step %d (%s) %s: %s\n", r.Step.N, r.Step.Kind, coach.Label(v.Check), v.Text)
				n++
			}
		}
	}
	if n == 0 {
		b.WriteString("None.\n")
	}
	if len(used) > 0 {
		b.WriteString("\n## What the checks mean\n\n")
		var names []string
		for c := range used {
			names = append(names, c)
		}
		sort.Slice(names, func(i, j int) bool { return coach.Label(names[i]) < coach.Label(names[j]) })
		for _, c := range names {
			if a := coach.About(c); a != "" {
				fmt.Fprintf(&b, "- **%s**: %s\n", coach.Label(c), a)
			}
		}
	}
	fmt.Fprintf(&b, "\nFull diffs: `tddt show <step>`.\n")
	return b.String()
}

// snippets writes the diffs of the cycle's steps that got a hint or
// warning. A missed refactor points at the Green before its Red.
func snippets(b *strings.Builder, h app.History, c cycle, d Differ) {
	for _, r := range c.steps {
		var own, missed bool
		for _, v := range r.Verdicts {
			if v.Level != coach.Hint && v.Level != coach.Warn {
				continue
			}
			if v.Check == "missed refactor" || v.Check == "missed test refactor" {
				missed = true
			} else {
				own = true
			}
		}
		if own {
			snippet(b, r.Step, fmt.Sprintf("Step %d diff", r.Step.N), d)
		}
		if missed {
			if g := greenBefore(h, r.Step.N); g != nil {
				snippet(b, *g, fmt.Sprintf("Step %d diff (the Green before step %d)", g.N, r.Step.N), d)
			}
		}
	}
}

func greenBefore(h app.History, n int) *steps.Step {
	var g *steps.Step
	for i := range h.Steps {
		if h.Steps[i].Step.N >= n {
			break
		}
		if h.Steps[i].Step.Kind == steps.Green {
			g = &h.Steps[i].Step
		}
	}
	return g
}

func snippet(b *strings.Builder, s steps.Step, title string, d Differ) {
	if s.From == s.To {
		return
	}
	diffs, err := d.Diff(s.From, s.To)
	if err != nil || len(diffs) == 0 {
		return
	}
	fmt.Fprintf(b, "<details><summary>%s</summary>\n\n```diff\n", title)
	for _, fd := range diffs {
		b.WriteString(fd.Patch)
	}
	b.WriteString("```\n\n</details>\n\n")
}

func cell(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "|", `\|`), "\n", " ")
}

// Write renders the report into root/.tddtrainer/reports/<start>.md and
// returns the path. A session keeps one report file, rewritten each time.
func Write(root string, h app.History, pending int, d Differ) (string, error) {
	dir := filepath.Join(root, Dir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, h.Start.Format("2006-01-02T15-04-05")+".md")
	if err := os.WriteFile(path, []byte(Render(h, pending, d)), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// Summary is the short text printed in the terminal on quit.
func Summary(h app.History, pending int, path string) string {
	s := stats(h, pending)
	var b strings.Builder
	fmt.Fprintf(&b, "Session: %s, %d cycles, %d tests added, clean cycles %s.\n", s.Duration, s.Cycles, s.Tests, s.cleanShare())
	if ts := tips(h); len(ts) > 0 {
		fmt.Fprintf(&b, "Focus: %s\n", ts[0].text)
	}
	if path != "" {
		fmt.Fprintf(&b, "Report: %s\n", path)
	}
	return b.String()
}
