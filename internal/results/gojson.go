package results

import (
	"bufio"
	"encoding/json"
	"io"
	"sort"
	"strings"
)

type goEvent struct {
	Action      string
	Package     string
	Test        string
	FailedBuild string
}

// ReadGoJSON reads `go test -json` output. A test ID is "<package>.<test>".
// Parent tests whose failure only reflects a failing subtest are dropped.
func ReadGoJSON(r io.Reader) (TestState, error) {
	failed := map[string]bool{}
	pkgFailedTests := map[string]bool{}
	var st TestState

	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 || line[0] != '{' {
			continue // non-JSON noise, e.g. linker output
		}
		var ev goEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}
		switch {
		case ev.Action == "build-fail":
			st.BuildBroken = true
		case ev.Action == "fail" && ev.Test != "":
			failed[ev.Package+"."+ev.Test] = true
			pkgFailedTests[ev.Package] = true
		case ev.Action == "fail" && ev.FailedBuild != "":
			st.BuildBroken = true
		case ev.Action == "fail" && !pkgFailedTests[ev.Package]:
			// package failed without any failing test: panic in init, TestMain, ...
			st.BuildBroken = true
		}
	}
	if err := sc.Err(); err != nil {
		return TestState{}, err
	}
	if st.BuildBroken {
		return st, nil
	}

	for id := range failed {
		if !hasFailedSubtest(id, failed) {
			st.Failing = append(st.Failing, id)
		}
	}
	sort.Strings(st.Failing)
	return st, nil
}

func hasFailedSubtest(id string, failed map[string]bool) bool {
	for other := range failed {
		if strings.HasPrefix(other, id+"/") {
			return true
		}
	}
	return false
}
