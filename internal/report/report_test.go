package report

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/sbradl/tdd-trainer/internal/app"
	"github.com/sbradl/tdd-trainer/internal/coach"
	"github.com/sbradl/tdd-trainer/internal/config"
	"github.com/sbradl/tdd-trainer/internal/snapshot"
	"github.com/sbradl/tdd-trainer/internal/steps"
)

type fakeDiffer struct{}

func (fakeDiffer) Diff(from, to snapshot.ID) ([]snapshot.FileDiff, error) {
	return []snapshot.FileDiff{{Path: "kata.go", Kind: config.Source, Patch: "--- a/kata.go\n+++ b/kata.go\n-\treturn \"\"\n+\tif n == 9 {\n"}}, nil
}

func rec(n int, k steps.Kind, newTests []string, vs ...coach.Verdict) app.StepRecord {
	for i := range vs {
		vs[i].Step, vs[i].Kind = n, k
	}
	return app.StepRecord{Step: steps.Step{N: n, Kind: k, NewTests: newTests, From: snapshot.ID(rune('a' + n)), To: snapshot.ID(rune('b' + n))}, Verdicts: vs}
}

func v(check string, l coach.Level, text string) coach.Verdict {
	return coach.Verdict{Check: check, Level: l, Text: text}
}

func history() app.History {
	start := time.Date(2026, 9, 26, 10, 0, 0, 0, time.Local)
	return app.History{Start: start, End: start.Add(12*time.Minute + 30*time.Second), Steps: []app.StepRecord{
		rec(1, steps.Red, []string{"one"}, v("new tests", coach.OK, "exactly 1 new test"), v("one-behaviour", coach.OK, "checks one behaviour")),
		rec(2, steps.Green, nil, v("tpp", coach.OK, "Nil → Constant (2/8)"), v("multi", coach.Uncertain, "not sure (p=0.63)")),
		rec(3, steps.Red, []string{"two"}, v("missed refactor", coach.Hint, "The last Green left something to refactor (review: code smells).")),
		rec(4, steps.Green, nil, v("tpp", coach.OK, "Unconditional → Selection (4/8)"), v("cheating", coach.Hint, "The code special-cases the test's inputs.")),
		rec(5, steps.Red, []string{"four", "five"}, v("anomaly", coach.Warn, "Several new tests in one step: write one failing test at a time.")),
		rec(6, steps.Green, nil, v("cheating", coach.Hint, "The code special-cases the test's inputs.")),
	}}
}

func TestReportSections(t *testing.T) {
	r := Render(history(), 2, fakeDiffer{})
	for _, want := range []string{
		"# TDD session report — 2026-09-26 10:00",
		"| 12m30s | 3 | 4 | 1 of 3 (33%) |",
		"> 2 gates were still being judged",
		"## Focus tips", "1. **No test-specific code** (2×): The code special-cases",
		"## Cycles", "### Cycle 1", "### Cycle 3", "| 5 | Red — four, five | ⚠ **Cycle rhythm**: Several new tests",
		"## Transformation path", "2: Nil → Constant (2/8) → 4: Unconditional → Selection (4/8)",
		"## Anomalies and missed refactors", "- Step 3 (Red): The last Green left", "- Step 5 (Red): Several new tests",
		"## Uncertain verdicts", "- Step 2 (Green) One transformation: not sure (p=0.63)",
		"## What the checks mean", "- **One transformation**: A Green applies one transformation",
		"<details><summary>Step 4 diff</summary>", "+\tif n == 9 {",
		"<details><summary>Step 2 diff (the Green before step 3)</summary>",
	} {
		if !strings.Contains(r, want) {
			t.Errorf("missing %q in:\n%s", want, r)
		}
	}
	// uncertain verdicts appear only in their own section
	if strings.Count(r, "not sure (p=0.63)") != 1 {
		t.Errorf("uncertain verdict outside its section:\n%s", r)
	}
	// diff snippets only for steps with a hint or warning
	if strings.Contains(r, "Step 1 diff") || strings.Contains(r, "Step 3 diff") || strings.Contains(r, "<summary>Step 2 diff</summary>") {
		t.Errorf("snippet for a clean step:\n%s", r)
	}
	// no scores
	if strings.Contains(strings.ToLower(r), "score") {
		t.Errorf("report has a score:\n%s", r)
	}
}

func TestEmptyReport(t *testing.T) {
	r := Render(app.History{Start: time.Now(), End: time.Now()}, 0, nil)
	for _, want := range []string{"No steps yet.", "Nothing stood out", "No transformations judged.", "None."} {
		if !strings.Contains(r, want) {
			t.Errorf("missing %q in:\n%s", want, r)
		}
	}
}

func TestWriteAndSummary(t *testing.T) {
	root := t.TempDir()
	h := history()
	path, err := Write(root, h, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(path, ".tddtrainer/reports/2026-09-26T10-00-00.md") {
		t.Fatalf("path %s", path)
	}
	s := Summary(h, 0, path)
	if !strings.Contains(s, "3 cycles, 4 tests added, clean cycles 1 of 3") || !strings.Contains(s, "Focus: The code special-cases") || !strings.Contains(s, path) {
		t.Fatalf("summary:\n%s", s)
	}
}

func TestShow(t *testing.T) {
	var b bytes.Buffer
	if err := Show(&b, history(), 2, fakeDiffer{}); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{"Step 2: Green", "✓ Transformation", "? One transformation", "not sure", "+\tif n == 9 {"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if err := Show(&b, history(), 99, fakeDiffer{}); err == nil {
		t.Error("want error for unknown step")
	}
}
