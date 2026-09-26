package judge

import (
	"context"
	"fmt"
	"math"
)

// Scorer returns option probabilities for gates sharing one evidence
// state, in order. Implementations may reuse the prefilled state.
type Scorer interface {
	Score(ctx context.Context, state string, gates []Gate) ([][]float64, error)
}

// Verdict is the judge's answer to one gate.
type Verdict struct {
	Gate   string
	Answer string  // an option ID, or Uncertain
	P      float64 // probability behind Answer (or the best option if uncertain)
	Top    string  // most likely option regardless of the floor
}

// Judge evaluates gates with a Scorer.
type Judge struct{ Scorer Scorer }

// Evaluate answers the named gates over the evidence. Gates that see the
// same evidence are scored together so a Scorer can reuse its state.
func (j Judge) Evaluate(ctx context.Context, ev Evidence, names []string) ([]Verdict, error) {
	gates := make([]Gate, len(names))
	for i, n := range names {
		g, ok := Gates[n]
		if !ok {
			return nil, fmt.Errorf("unknown gate %q", n)
		}
		gates[i] = g
	}
	return j.EvaluateGates(ctx, ev, gates)
}

// EvaluateGates is Evaluate for gates built on the fly, such as FixedGate.
func (j Judge) EvaluateGates(ctx context.Context, ev Evidence, gates []Gate) ([]Verdict, error) {
	type probe struct {
		gate  int // index into gates
		clean bool
		g     Gate
	}
	groups := map[string][]probe{}
	var order []string
	for i, g := range gates {
		st := g.State(ev)
		if _, seen := groups[st]; !seen {
			order = append(order, st)
		}
		groups[st] = append(groups[st], probe{i, false, g})
		if g.Clean != nil {
			groups[st] = append(groups[st], probe{i, true, *g.Clean})
		}
	}

	main := make([][]float64, len(gates))
	clean := make([][]float64, len(gates))
	for _, st := range order {
		ps := groups[st]
		gs := make([]Gate, len(ps))
		for i, p := range ps {
			gs[i] = p.g
		}
		probs, err := j.Scorer.Score(ctx, st, gs)
		if err != nil {
			return nil, err
		}
		for i, p := range ps {
			if p.clean {
				clean[p.gate] = probs[i]
			} else {
				main[p.gate] = probs[i]
			}
		}
	}

	out := make([]Verdict, len(gates))
	for i, g := range gates {
		out[i] = decide(g, main[i], clean[i])
	}
	return out, nil
}

// decide applies the floor and, for review lenses, the two-probe rule:
// "no" only when the clean probe is confident too.
func decide(g Gate, probs, cleanProbs []float64) Verdict {
	answer, p := argmax(g.Options, probs)
	v := Verdict{Gate: g.Name, Top: answer, P: p, Answer: Uncertain}
	if p >= g.Floor {
		v.Answer = answer
	}
	if g.Clean != nil && answer == "no" {
		ca, cp := argmax(g.Clean.Options, cleanProbs)
		pClean := cp
		if ca != "no" {
			pClean = round3(1 - cp)
		}
		v.P = pClean
		v.Answer = Uncertain
		if pClean >= g.Floor {
			v.Answer = "no"
		}
	}
	return v
}

func argmax(opts []Option, probs []float64) (string, float64) {
	bi := 0
	for i := range probs {
		if probs[i] > probs[bi] {
			bi = i
		}
	}
	return opts[bi].ID, round3(probs[bi])
}

func round3(p float64) float64 { return math.Round(p*1000) / 1000 }
