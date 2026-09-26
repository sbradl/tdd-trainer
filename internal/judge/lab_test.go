package judge

// Tuning lab: scores question/option variants of one gate against its
// fixtures, side by side, with the model loaded once. Adopt a variant only
// if `tddt judge --regress` then shows no regressed fixture on CPU and GPU.
//
//	TDDT_LAB=<gate> TDDT_LAB_VARIANTS=<variants.json> go test -count=1 -run TestLab -v ./internal/judge
//
// variants.json: [{"name", "question", "clean_question", "clean_options", "no_clean",
// "options": [{"ID","Desc"}], "parts"}],
// every field but name optional.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

type labVariant struct {
	Name     string   `json:"name"`
	Question string   `json:"question"`
	Options  []Option `json:"options"`
	Parts    []Part   `json:"parts"`
	Clean    string   `json:"clean_question"`
	CleanOpt []Option `json:"clean_options"`
	NoClean  bool     `json:"no_clean"`
}

func TestLab(t *testing.T) {
	gate := os.Getenv("TDDT_LAB")
	if gate == "" {
		t.Skip()
	}
	var vs []labVariant
	if f := os.Getenv("TDDT_LAB_VARIANTS"); f != "" {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, &vs); err != nil {
			t.Fatal(err)
		}
	}
	e := openTestEngine(t)
	base := Gates[gate]
	variants := []Gate{base}
	names := []string{"base"}
	for _, v := range vs {
		g := base
		if v.Question != "" {
			g.Question = v.Question
		}
		if v.Options != nil {
			g.Options = v.Options
		}
		if v.Parts != nil {
			g.Parts = v.Parts
		}
		if v.Clean != "" || v.CleanOpt != nil {
			c := *g.Clean
			if v.Clean != "" {
				c.Question = v.Clean
			}
			if v.CleanOpt != nil {
				c.Options = v.CleanOpt
			}
			g.Clean = &c
		}
		if v.NoClean {
			g.Clean = nil
		}
		variants = append(variants, g)
		names = append(names, v.Name)
	}
	fx, _ := Fixtures()
	correct := make([]int, len(variants))
	wrong := make([]int, len(variants))
	var b strings.Builder
	fmt.Fprintf(&b, "%-44s %-10s", "fixture", "expect")
	for _, n := range names {
		fmt.Fprintf(&b, " %-18s", n)
	}
	b.WriteString("\n")
	for _, f := range fx {
		if f.Gate != gate {
			continue
		}
		fmt.Fprintf(&b, "%-44s %-10s", f.Name, f.Expected)
		for i, g := range variants {
			st := g.State(f.Evidence)
			gs := []Gate{g}
			if g.Clean != nil {
				gs = append(gs, *g.Clean)
			}
			probs, err := e.Score(context.Background(), st, gs)
			if err != nil {
				t.Fatal(err)
			}
			var clean []float64
			if len(probs) > 1 {
				clean = probs[1]
			}
			v := decide(g, probs[0], clean)
			mark := " "
			switch {
			case strings.Contains("+"+f.Expected+"+", "+"+v.Answer+"+"):
				correct[i]++
				mark = "+"
			case v.Answer != Uncertain:
				wrong[i]++
				mark = "X"
			}
			fmt.Fprintf(&b, " %s%-9s %.3f  ", mark, v.Top, v.P)
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "%-55s", "correct / wrong")
	for i := range variants {
		fmt.Fprintf(&b, " %2d / %-13d", correct[i], wrong[i])
	}
	t.Log("\n" + b.String())
}
