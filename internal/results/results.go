// Package results reads test runner output into a Test state.
package results

import "strings"

// TestState is the set of currently failing test IDs, or build broken
// when the tests cannot run at all.
type TestState struct {
	BuildBroken bool
	Failing     []string // sorted test IDs
}

func (s TestState) Green() bool { return !s.BuildBroken && len(s.Failing) == 0 }

func (s TestState) String() string {
	switch {
	case s.BuildBroken:
		return "build broken"
	case len(s.Failing) == 0:
		return "all green"
	default:
		return "failing: " + strings.Join(s.Failing, ", ")
	}
}
