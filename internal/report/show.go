package report

import (
	"fmt"
	"io"
	"strings"

	"github.com/sbradl/tdd-trainer/internal/app"
	"github.com/sbradl/tdd-trainer/internal/coach"
)

// Show prints step n of the history: kind, verdicts (uncertain ones
// included) and its full diff.
func Show(w io.Writer, h app.History, n int, d Differ) error {
	for _, r := range h.Steps {
		if r.Step.N != n {
			continue
		}
		s := r.Step
		fmt.Fprintf(w, "Step %d: %s", s.N, s.Kind)
		if len(s.NewTests) > 0 {
			fmt.Fprintf(w, " — %s", strings.Join(s.NewTests, ", "))
		}
		fmt.Fprintf(w, "\nsnapshots %.8s..%.8s\n", s.From, s.To)
		for _, a := range s.Anomalies {
			fmt.Fprintf(w, "anomaly: %s\n", a)
		}
		if len(r.Verdicts) > 0 {
			fmt.Fprintln(w)
		}
		for _, v := range r.Verdicts {
			fmt.Fprintf(w, "%s %-28s %s\n", icon(v.Level), coach.Label(v.Check), v.Text)
		}
		if s.From == s.To {
			fmt.Fprintln(w, "\n(no changes)")
			return nil
		}
		diffs, err := d.Diff(s.From, s.To)
		if err != nil {
			return err
		}
		fmt.Fprintln(w)
		for _, fd := range diffs {
			fmt.Fprint(w, fd.Patch)
		}
		return nil
	}
	return fmt.Errorf("no step %d in this session (it has %d)", n, len(h.Steps))
}
