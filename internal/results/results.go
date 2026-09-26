// Package results reads test runner output into a normalised Report and
// derives the Test state from it.
package results

import (
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
)

type Status int

const (
	Passed Status = iota
	Failed
	Skipped
)

// Kind tells a JUnit <failure> from an <error>.
type Kind int

const (
	KindFailure Kind = iota
	KindError
)

// Result is one test case as reported by a runner.
type Result struct {
	File, Class, Name string
	Status            Status
	Kind              Kind   // only for Failed
	Type              string // exception type, if the runner reports one
	Message           string
}

var trailingLine = regexp.MustCompile(`:\d+$`)

// ID identifies a test across runs: file + classname + name. Line numbers
// that some runners (busted) put into the classname are dropped, so moving
// a test does not make it a new one.
func (r Result) ID() string {
	var parts []string
	for _, p := range []string{r.File, trailingLine.ReplaceAllString(r.Class, ""), r.Name} {
		if p != "" && (len(parts) == 0 || parts[len(parts)-1] != p) {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, "::")
}

// Report is everything one test run reported.
type Report struct {
	Tests []Result
	// BuildBroken is set when the tests could not run at all, e.g. a Go
	// compile error or a busted spec file that failed to load.
	BuildBroken bool
}

// FailReason says why a test fails, as far as exact checks can tell.
type FailReason int

const (
	ReasonUndecided FailReason = iota // left to the judge's red-check gate
	ReasonAssertion                   // right reason: the behaviour is missing
	ReasonWrong                       // compile error, typo, broken setup, ...
)

func (r FailReason) String() string {
	return [...]string{"undecided", "assertion", "wrong reason"}[r]
}

// Classify decides the fail reason from exact signals only: a JUnit
// <error> is the wrong reason; an exception type, where the runner reports
// one, tells assertions from other exceptions. Everything else is undecided;
// e.g. pytest reports a NameError inside a test as <failure>.
func Classify(r Result) FailReason {
	switch {
	case r.Kind == KindError:
		return ReasonWrong
	case r.Type == "":
		return ReasonUndecided
	case strings.Contains(r.Type, "AssertionError"):
		return ReasonAssertion
	default:
		return ReasonWrong
	}
}

// Failure is one failing test in a Test state.
type Failure struct {
	ID      string
	Reason  FailReason
	Message string
}

// TestState is the set of currently failing tests, or build broken when
// the tests cannot run at all.
type TestState struct {
	BuildBroken bool
	Failing     []Failure // sorted by ID
}

func (s TestState) Green() bool { return !s.BuildBroken && len(s.Failing) == 0 }

func (s TestState) String() string {
	switch {
	case s.BuildBroken:
		return "build broken"
	case len(s.Failing) == 0:
		return "all green"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d failing:", len(s.Failing))
	for _, f := range s.Failing {
		fmt.Fprintf(&b, "\n  %s (%s)", f.ID, f.Reason)
	}
	return b.String()
}

// State derives the Test state. Results sharing an ID are merged; the
// wrong reason wins over the others.
func (rep Report) State() TestState {
	if rep.BuildBroken {
		return TestState{BuildBroken: true}
	}
	byID := map[string]Failure{}
	for _, r := range rep.Tests {
		if r.Status != Failed {
			continue
		}
		f := Failure{ID: r.ID(), Reason: Classify(r), Message: r.Message}
		if prev, ok := byID[f.ID]; ok {
			if prev.Reason == ReasonWrong || f.Reason != ReasonWrong {
				f.Reason = prev.Reason
			}
			f.Message = prev.Message + "\n" + f.Message
		}
		byID[f.ID] = f
	}
	var st TestState
	for _, f := range byID {
		st.Failing = append(st.Failing, f)
	}
	sort.Slice(st.Failing, func(i, j int) bool { return st.Failing[i].ID < st.Failing[j].ID })
	return st
}

// Formats lists the supported result formats.
var Formats = []string{"go-json", "junit-xml", "trx"}

// Read parses runner output in the given format.
func Read(format string, r io.Reader) (Report, error) {
	switch format {
	case "go-json":
		return ReadGoJSON(r)
	case "junit-xml":
		return ReadJUnit(r)
	case "trx":
		return ReadTRX(r)
	}
	return Report{}, fmt.Errorf("unknown results format %q (want one of %s)", format, strings.Join(Formats, ", "))
}
