package coach

import (
	"fmt"
	"strings"

	"github.com/sbradl/tdd-trainer/internal/judge"
	"github.com/sbradl/tdd-trainer/internal/steps"
)

func gateVerdict(v judge.Verdict, level Level, text string) Verdict {
	if v.Answer == judge.Uncertain {
		return Verdict{Check: v.Gate, Level: Uncertain, P: v.P, Text: fmt.Sprintf("The judge could not decide (best guess %q, p=%.2f).", v.Top, v.P)}
	}
	return Verdict{Check: v.Gate, Level: level, P: v.P, Text: text}
}

func finishRedCheck(vs map[string]judge.Verdict) []Verdict {
	v := vs["red-check"]
	if v.Answer == "yes" {
		return []Verdict{gateVerdict(v, OK, "It fails on its assertion (expected vs actual), so it proves the behaviour is missing.")}
	}
	return []Verdict{gateVerdict(v, Warn, "The new test fails for the wrong reason: make it compile and reach its assertion, then watch it fail there.")}
}

func finishOneBehaviour(vs map[string]judge.Verdict) []Verdict {
	v := vs["one-behaviour"]
	if v.Answer == "yes" {
		return []Verdict{gateVerdict(v, OK, "The test checks a single behaviour.")}
	}
	return []Verdict{gateVerdict(v, Hint, "This test checks more than one behaviour; split it so each test drives one small step.")}
}

// TPP bands: which transformations count as which step size.
var band = map[string]int{
	"nil": 0, "constant": 0,
	"variable": 1, "selection": 1,
	"list": 2, "iteration": 2, "recursion": 2, "mutation": 2,
}

var bandName = []string{"constant", "simple", "complex"}

// sizeBand maps a step-size answer to its band.
var sizeBand = map[string]int{"constant": 0, "simple": 1, "complex": 2}

func (c *Coach) tppLabel(id string) string {
	pos, name := 0, id
	for i, o := range judge.TPPOptions {
		if o.ID == id {
			pos = i + 1
			name, _, _ = strings.Cut(o.Desc, ":")
		}
	}
	if c.order == "recursion-first" {
		switch id {
		case "iteration":
			pos = 7
		case "recursion":
			pos = 6
		}
	}
	return fmt.Sprintf("%s (%d/8)", strings.ReplaceAll(name, "->", "→"), pos)
}

func (c *Coach) finishGreen(vs map[string]judge.Verdict) []Verdict {
	var out []Verdict
	tpp := vs["tpp"]
	if tpp.Answer != judge.Uncertain {
		out = append(out, gateVerdict(tpp, OK, "Applied "+c.tppLabel(tpp.Answer)+"."))
	} else {
		out = append(out, gateVerdict(tpp, OK, ""))
	}

	if size, ok := vs["step-size"]; ok {
		switch {
		case size.Answer == judge.Uncertain || tpp.Answer == judge.Uncertain:
			out = append(out, Verdict{Check: "step-size", Level: Uncertain, P: size.P,
				Text: fmt.Sprintf("The judge could not decide (needed: %q p=%.2f, applied: %q p=%.2f).", size.Top, size.P, tpp.Top, tpp.P)})
		case band[tpp.Answer] > sizeBand[size.Answer]:
			out = append(out, gateVerdict(size, Hint, fmt.Sprintf(
				"A simpler change would have done: the test only needed a %s change, the code made a %s one (%s).",
				size.Answer, bandName[band[tpp.Answer]], c.tppLabel(tpp.Answer))))
		default:
			out = append(out, gateVerdict(size, OK, fmt.Sprintf("The test needed a %s change and the code made a %s one: no bigger than necessary.", size.Answer, bandName[band[tpp.Answer]])))
		}
	}

	multi := vs["multi"]
	if multi.Answer == "yes" {
		out = append(out, gateVerdict(multi, Hint, "Several transformations in one Green: a test is probably missing in between."))
	} else {
		out = append(out, gateVerdict(multi, OK, "One transformation, as a single new test should need."))
	}

	if ch, ok := vs["cheating"]; ok {
		if ch.Answer == "yes" {
			out = append(out, gateVerdict(ch, Hint, "The code special-cases the test's inputs: generalise instead of matching test values."))
		} else {
			out = append(out, gateVerdict(ch, OK, "No special-casing of the tests' inputs beyond fake-it."))
		}
	}
	return out
}

func finishRefactor(vs map[string]judge.Verdict) []Verdict {
	var out []Verdict
	st := vs["structural"]
	if st.Answer == "no" {
		out = append(out, gateVerdict(st, Warn, "This refactoring seems to change what the code computes; refactorings keep behaviour exactly."))
	} else {
		out = append(out, gateVerdict(st, OK, "Only the structure changed; the code computes the same as before."))
	}
	eff := vs["refactor-effect"]
	switch eff.Answer {
	case "worsens":
		out = append(out, gateVerdict(eff, Hint, "This refactoring makes the code harder to read or change."))
	default:
		out = append(out, gateVerdict(eff, OK, map[string]string{
			"improves": "The code is easier to read or change afterwards.",
			"neutral":  "About as readable as before: no clear gain or loss.",
		}[eff.Answer]))
	}
	return out
}

var lensTitle = map[string]string{
	"review-ddd":        "domain design",
	"review-smells":     "code smells",
	"review-clean-code": "clean code",
	"review-pragmatic":  "pragmatic design",
	"review-philosophy": "module design",
}

// finishLenses combines the review lenses into one missed-refactor
// verdict. When lenses find something, a follow-up job asks each of them
// which problem it is, so the hint can say what to refactor.
func (c *Coach) finishLenses(red steps.Step, green int, ev judge.Evidence, vs map[string]judge.Verdict) []Verdict {
	var found, unsure []string
	maxP := 0.0
	for _, name := range judge.LensNames() {
		v := vs[name]
		switch v.Answer {
		case "yes":
			found = append(found, name)
			maxP = max(maxP, v.P)
		case judge.Uncertain:
			unsure = append(unsure, lensTitle[name])
		}
	}
	switch {
	case len(found) > 0:
		var which []string
		for _, name := range found {
			which = append(which, name+"~which")
		}
		c.enqueue(&job{prio: prioLenses, step: red.N, kind: red.Kind, ev: ev, gates: which,
			finish: func(ws map[string]judge.Verdict) []Verdict {
				return []Verdict{missedRefactorHint(green, found, ws, maxP)}
			}})
		return nil
	case len(unsure) > 0:
		return []Verdict{{Check: "missed refactor", Level: Uncertain, Text: "The judge could not decide whether the last Green left something to refactor (" + strings.Join(unsure, ", ") + ")."}}
	}
	return []Verdict{{Check: "missed refactor", Level: OK, Text: "Nothing worth refactoring after the last Green."}}
}

// missedRefactorHint names the problem each firing lens sees most clearly.
func missedRefactorHint(green int, lenses []string, which map[string]judge.Verdict, p float64) Verdict {
	var problems []string
	seen := map[string]bool{}
	for _, name := range lenses {
		w := which[name+"~which"]
		desc := ""
		for _, o := range judge.LensProblems[name] {
			if o.ID == w.Top {
				desc = o.Desc
			}
		}
		if desc == "" || seen[desc] {
			continue
		}
		seen[desc] = true
		if w.Answer == judge.Uncertain {
			desc = "probably " + lowerFirst(desc)
		}
		problems = append(problems, desc+" ("+lensTitle[name]+")")
	}
	where := "The last Green"
	if green > 0 {
		where = fmt.Sprintf("The last Green (step %d, see tddt show %d)", green, green)
	}
	return Verdict{Check: "missed refactor", Level: Hint, P: p, Text: fmt.Sprintf(
		"%s left something to refactor: %s Refactor before the next Red.", where, strings.Join(problems, " "))}
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}
