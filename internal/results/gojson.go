package results

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
)

type goEvent struct {
	Action      string
	Package     string
	Test        string
	Output      string
	FailedBuild string
}

// ReadGoJSON reads `go test -json` output; package = classname. A parent
// test whose failure only reflects a failing subtest counts as passing.
func ReadGoJSON(r io.Reader) (Report, error) {
	var order []goKey
	status := map[goKey]Status{}
	output := map[goKey]*strings.Builder{}
	pkgHasFailedTest := map[string]bool{}
	var rep Report

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
		k := goKey{ev.Package, ev.Test}
		switch {
		case ev.Action == "build-fail":
			rep.BuildBroken = true
		case ev.Test != "" && ev.Action == "run":
			order = append(order, k)
			output[k] = &strings.Builder{}
		case ev.Test != "" && ev.Action == "output":
			if b := output[k]; b != nil {
				b.WriteString(ev.Output)
			}
		case ev.Test != "" && ev.Action == "pass":
			status[k] = Passed
		case ev.Test != "" && ev.Action == "skip":
			status[k] = Skipped
		case ev.Test != "" && ev.Action == "fail":
			status[k] = Failed
			pkgHasFailedTest[ev.Package] = true
		case ev.Action == "fail" && (ev.FailedBuild != "" || !pkgHasFailedTest[ev.Package]):
			// build failure, or a package failing without a failing test:
			// panic in init, TestMain, ...
			rep.BuildBroken = true
		}
	}
	if err := sc.Err(); err != nil {
		return Report{}, err
	}

	for _, k := range order {
		st := status[k]
		if st == Failed && hasFailedSubtest(k, status) {
			st = Passed // its failure is reported by the failing subtest
		}
		res := Result{Class: k.pkg, Name: k.test, Status: st}
		if st == Failed {
			res.Message = strings.TrimSpace(output[k].String())
		}
		rep.Tests = append(rep.Tests, res)
	}
	return rep, nil
}

type goKey struct{ pkg, test string }

func hasFailedSubtest(parent goKey, status map[goKey]Status) bool {
	for k, st := range status {
		if st == Failed && k.pkg == parent.pkg && strings.HasPrefix(k.test, parent.test+"/") {
			return true
		}
	}
	return false
}
