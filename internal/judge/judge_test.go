package judge

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

type refRow struct {
	ID     string `json:"id"`
	Prompt string `json:"prompt"`
	IDs    []int  `json:"ids"`
}

// testdata/ref.jsonl was dumped from the Python reference (SemIf +
// judge.py) by the Go scoring spike's dumpref.py: prompt text and
// Hugging Face token IDs per fixture probe.
func loadRef(t *testing.T) map[string]refRow {
	t.Helper()
	f, err := os.Open("testdata/ref.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	m := map[string]refRow{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		var r refRow
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			t.Fatal(err)
		}
		m[r.ID] = r
	}
	return m
}

// tunedGates changed their questions or options after the reference dump
// (slice 12, checked with tddt judge --regress); their prompts no longer
// match it by design.
var tunedGates = map[string]bool{"tpp": true, "red-check": true, "one-behaviour": true, "structural": true, "review-pragmatic": true}

// probes lists the prompts of the untuned gates' fixtures that the
// reference covers, keyed like the reference dump (lens clean probes get a
// "~clean" suffix).
func probes(t *testing.T) map[string]string {
	t.Helper()
	fx, err := Fixtures()
	if err != nil {
		t.Fatal(err)
	}
	ref := loadRef(t)
	out := map[string]string{}
	for _, f := range fx {
		if _, inRef := ref[f.Name]; !inRef || tunedGates[f.Gate] {
			continue
		}
		g := Gates[f.Gate]
		st := g.State(f.Evidence)
		out[f.Name] = g.Prompt(st)
		if g.Clean != nil {
			out[f.Name+"~clean"] = g.Clean.Prompt(st)
		}
	}
	return out
}

func TestPromptsMatchPythonReference(t *testing.T) {
	ref := loadRef(t)
	ps := probes(t)
	if len(ps) < 40 {
		t.Errorf("only %d probes compared with the reference", len(ps))
	}
	for id, p := range ps {
		r, ok := ref[id]
		if !ok {
			t.Errorf("%s missing from reference", id)
			continue
		}
		if p != r.Prompt {
			t.Errorf("%s: prompt differs from reference", id)
		}
	}
}

func TestFixtures(t *testing.T) {
	fx, err := Fixtures()
	if err != nil {
		t.Fatal(err)
	}
	if len(fx) < 88 {
		t.Fatalf("%d fixtures", len(fx))
	}
	for _, f := range fx {
		g := Gates[f.Gate]
		for _, want := range strings.Split(f.Expected, "+") {
			valid := false
			for _, o := range g.Options {
				valid = valid || o.ID == want
			}
			if !valid {
				t.Errorf("%s: %q is not an option of %s", f.Name, want, f.Gate)
			}
		}
		for _, p := range g.Parts {
			if strings.TrimSpace(f.Evidence[p]) == "" {
				t.Errorf("%s: missing %s", f.Name, p)
			}
		}
	}
}

func TestPyJSONString(t *testing.T) {
	cases := map[string]string{
		"plain":      `"plain"`,
		"a\"b\\c":    `"a\"b\\c"`,
		"x\ny\tz":    `"x\ny\tz"`,
		"ü → é":      `"ü → é"`,
		"\x01\x1f":   `"\u0001\u001f"`,
		"\b\f\r":     `"\b\f\r"`,
		"emoji 😀":    `"emoji 😀"`,
		"<tag>&amp;": `"<tag>&amp;"`,
	}
	for in, want := range cases {
		if got := pyJSONString(in); got != want {
			t.Errorf("%q: got %s want %s", in, got, want)
		}
	}
}

func TestClipKeepsHeadAndTail(t *testing.T) {
	s := strings.Repeat("a", 4000) + strings.Repeat("b", 4000)
	c := clip(s)
	if !strings.HasPrefix(c, strings.Repeat("a", 3000)+"\n[... clipped ...]\n") || !strings.HasSuffix(c, strings.Repeat("b", 3000)) {
		t.Fatal("clip lost head or tail")
	}
	if clip("short") != "short" {
		t.Fatal("clipped a short part")
	}
}

// fakeScorer answers from a table keyed by gate name.
type fakeScorer struct {
	probs map[string][]float64
	calls [][]string // gate names per Score call
}

func (f *fakeScorer) Score(_ context.Context, _ string, gates []Gate) ([][]float64, error) {
	var names []string
	var out [][]float64
	for _, g := range gates {
		names = append(names, g.Name)
		out = append(out, f.probs[g.Name])
	}
	f.calls = append(f.calls, names)
	return out, nil
}

func TestVerdictThresholds(t *testing.T) {
	f := &fakeScorer{probs: map[string][]float64{
		"red-check": {0.9, 0.1},
		"multi":     {0.7, 0.3},
		"step-size": {0.5, 0.3, 0.2},
		"tpp":       {0, 0, 0.85, 0.15, 0, 0, 0, 0},
	}}
	vs, err := Judge{f}.Evaluate(context.Background(), Evidence{PartTest: "t", PartTranscript: "o", PartDiff: "d", PartSource: "s"},
		[]string{"red-check", "multi", "step-size", "tpp"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"yes", Uncertain, "constant", "variable"}
	for i, v := range vs {
		if v.Answer != want[i] {
			t.Errorf("%s: got %s want %s (p=%.3f)", v.Gate, v.Answer, want[i], v.P)
		}
	}
	if vs[1].Top != "yes" {
		t.Errorf("multi top %s", vs[1].Top)
	}
}

func TestReviewLensNeedsBothProbesForNo(t *testing.T) {
	cases := []struct {
		main, clean []float64
		want        string
		p           float64
	}{
		{[]float64{0.9, 0.1}, []float64{0.9, 0.1}, "yes", 0.9},     // detect says yes
		{[]float64{0.1, 0.9}, []float64{0.05, 0.95}, "no", 0.95},   // both say no
		{[]float64{0.1, 0.9}, []float64{0.4, 0.6}, Uncertain, 0.6}, // clean unsure
		{[]float64{0.1, 0.9}, []float64{0.7, 0.3}, Uncertain, 0.3}, // clean says yes
		{[]float64{0.35, 0.65}, []float64{0.1, 0.9}, "no", 0.9},    // main unsure no, clean sure
	}
	for _, c := range cases {
		f := &fakeScorer{probs: map[string][]float64{"review-smells": c.main, "review-smells~clean": c.clean}}
		vs, err := Judge{f}.Evaluate(context.Background(), Evidence{PartDiff: "d"}, []string{"review-smells"})
		if err != nil {
			t.Fatal(err)
		}
		if vs[0].Answer != c.want || vs[0].P != c.p {
			t.Errorf("%v/%v: got %s p=%.3f, want %s p=%.3f", c.main, c.clean, vs[0].Answer, vs[0].P, c.want, c.p)
		}
	}
}

func TestGatesSharingEvidenceAreScoredTogether(t *testing.T) {
	f := &fakeScorer{probs: map[string][]float64{
		"tpp": make([]float64, 8), "multi": {1, 0}, "cheating": {0, 1},
		"review-ddd": {1, 0}, "review-ddd~clean": {1, 0},
	}}
	_, err := Judge{f}.Evaluate(context.Background(), Evidence{PartTest: "t", PartDiff: "d"},
		[]string{"tpp", "cheating", "multi", "review-ddd"})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 2 || strings.Join(f.calls[0], ",") != "tpp,multi,review-ddd,review-ddd~clean" || strings.Join(f.calls[1], ",") != "cheating" {
		t.Fatalf("calls %v", f.calls)
	}
}

func TestUnknownGate(t *testing.T) {
	if _, err := (Judge{&fakeScorer{}}).Evaluate(context.Background(), Evidence{}, []string{"nope"}); err == nil {
		t.Fatal("want error")
	}
}
