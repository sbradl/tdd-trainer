package judge

import (
	"context"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"path"
	"slices"
	"sort"
	"strings"
	"time"
)

// Fixtures are labelled evidence per gate, named <gate>.<expected>[.<tag>]
// (several acceptable answers joined by "+"),
// holding test.txt, transcript.txt, source.txt and diff.txt as needed.
//
//go:embed fixtures
var fixturesFS embed.FS

// Fixture is one labelled regression case.
type Fixture struct {
	Name, Gate, Expected string
	Evidence             Evidence
}

// Fixtures loads the embedded regression suite, sorted by name.
func Fixtures() ([]Fixture, error) {
	dirs, err := fs.ReadDir(fixturesFS, "fixtures")
	if err != nil {
		return nil, err
	}
	var out []Fixture
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		parts := strings.Split(d.Name(), ".")
		if len(parts) < 2 {
			return nil, fmt.Errorf("fixture %s: want <gate>.<expected>[.<tag>]", d.Name())
		}
		if _, ok := Gates[parts[0]]; !ok {
			return nil, fmt.Errorf("fixture %s: unknown gate", d.Name())
		}
		ev := Evidence{}
		for _, p := range partOrder {
			data, err := fixturesFS.ReadFile(path.Join("fixtures", d.Name(), string(p)+".txt"))
			if err != nil {
				continue
			}
			// Python universal newlines, as in the reference implementation
			s := strings.ReplaceAll(string(data), "\r\n", "\n")
			ev[p] = strings.ReplaceAll(s, "\r", "\n")
		}
		out = append(out, Fixture{Name: d.Name(), Gate: parts[0], Expected: parts[1], Evidence: ev})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// RegressResult summarises a regression run.
type RegressResult struct {
	Total, Correct, ConfidentWrong, Uncertain int
}

// Regress judges every fixture and writes one line per fixture plus a
// summary. only restricts the run to the named gates (nil = all).
func Regress(ctx context.Context, j Judge, only []string, w io.Writer) (RegressResult, error) {
	fixtures, err := Fixtures()
	if err != nil {
		return RegressResult{}, err
	}
	var r RegressResult
	start := time.Now()
	perGate := map[string]time.Duration{}
	perGateN := map[string]int{}
	for _, f := range fixtures {
		if len(only) > 0 && !contains(only, f.Gate) {
			continue
		}
		t0 := time.Now()
		vs, err := j.Evaluate(ctx, f.Evidence, []string{f.Gate})
		if err != nil {
			return r, fmt.Errorf("%s: %w", f.Name, err)
		}
		d := time.Since(t0)
		perGate[f.Gate] += d
		perGateN[f.Gate]++
		v := vs[0]
		r.Total++
		mark := "MISS"
		switch {
		case slices.Contains(strings.Split(f.Expected, "+"), v.Answer):
			r.Correct++
			mark = "ok"
		case v.Answer == Uncertain:
			r.Uncertain++
		default:
			r.ConfidentWrong++
			mark = "WRONG"
		}
		fmt.Fprintf(w, "%-44s expect=%-14s got=%-14s p=%.3f %s  secs=%.2f\n", f.Name, f.Expected, v.Answer, v.P, mark, d.Seconds())
	}
	fmt.Fprintf(w, "%d/%d confident and correct, %d confident and wrong, %d uncertain  total=%.1fs\n",
		r.Correct, r.Total, r.ConfidentWrong, r.Uncertain, time.Since(start).Seconds())
	var gs []string
	for g := range perGate {
		gs = append(gs, g)
	}
	sort.Strings(gs)
	for _, g := range gs {
		fmt.Fprintf(w, "# gate %-22s n=%-2d mean %.2fs\n", g, perGateN[g], perGate[g].Seconds()/float64(perGateN[g]))
	}
	return r, nil
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if x == y {
			return true
		}
	}
	return false
}
