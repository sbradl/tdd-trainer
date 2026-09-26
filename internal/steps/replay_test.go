package steps

import (
	"strings"
	"testing"
)

// The verdict-ux prototype (branch prototype/verdict-ux) scripts a
// TypeScript Roman-numerals kata. Replaying its test states must give the
// same step sequence. The prototype labels a step with an anomaly
// "Anomaly"; a Refactor is reported when the next Red starts (spec §6), so
// it appears right before that Red.
func TestReplayVerdictUXPrototype(t *testing.T) {
	runs := []run{
		{"|", ""},                        // Baseline: 0 tests
		{"i | i!", "t"},                  // 'converts 1 to I' does not compile yet
		{"i | i", "s"},                   // stub: fails on its assertion
		{"i |", "s"},                     // return "I"
		{"i ii | ii", "t"},               // converts 2 to II
		{"i ii |", "s"},                  // "I".repeat(n)
		{"i ii iv v | iv v", "t"},        // 2 new tests at once
		{"i ii iv v |", "s"},             // lookup table + while loop
		{"i ii iv v |", "s"},             // refactor: rename, extract SYMBOLS
		{"i ii iv v ix | ix", "t"},       // converts 9 to IX
		{"i ii iv v ix |", "s"},          // special-case 9
		{"i ii iv v ix xl | xl", "t"},    // converts 40 to XL, no refactor before
		{"i ii iv v ix xl | ix xl", "s"}, // refactor broke behaviour
	}
	m := New()
	var got []string
	for i, r := range runs {
		for _, e := range m.Observe(r.observation(i)) {
			switch e := e.(type) {
			case Baseline:
				got = append(got, "Baseline")
			case RedInProgress:
				got = append(got, "Red in progress")
			case StepDone:
				label := e.Step.Kind.String()
				if len(e.Step.Anomalies) > 0 && e.Step.Kind != Green {
					label = "Anomaly"
				}
				if e.Step.AfterGreen && i == 11 {
					label += " (missed refactor?)"
				}
				got = append(got, label)
			}
		}
	}
	want := []string{
		"Baseline", "Red in progress", "Red", "Green", "Red", "Green",
		"Anomaly", "Green", "Refactor", "Red", "Green",
		"Red (missed refactor?)", "Anomaly",
	}
	if strings.Join(got, ", ") != strings.Join(want, ", ") {
		t.Fatalf("\ngot  %s\nwant %s", strings.Join(got, ", "), strings.Join(want, ", "))
	}
}
