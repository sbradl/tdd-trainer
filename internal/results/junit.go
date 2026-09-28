package results

import (
	"encoding/xml"
	"io"
	"regexp"
	"strings"
)

// exUnitRaised finds the exception ExUnit reports for a test that raised,
// e.g. "** (UndefinedFunctionError) function Kata.add/2 is undefined";
// its JUnit formatter leaves the type attribute empty.
var exUnitRaised = regexp.MustCompile(`(?m)^\s*\*\* \(([\w.]+)\)`)

// pytestRaised finds the exception pytest names at the start of a failure
// message, e.g. "NameError: name 'scor' is not defined"; a bare "Error:"
// (jest's expect) is not one.
var pytestRaised = regexp.MustCompile(`^([A-Za-z_][\w.]*(?:Error|Exception))(?::|$)`)

// notReached are exceptions that mean the test never reached the code
// under test: a missing symbol, module or stub. A crash inside the code
// (an IndexError for a new input) stays for the judge to decide.
var notReached = map[string]bool{
	"NameError": true, "ImportError": true, "ModuleNotFoundError": true, "SyntaxError": true,
	"IndentationError": true, "NotImplementedError": true, // pytest
	"UndefinedFunctionError": true, "CompileError": true, // ExUnit
}

// raisedType types a JUnit failure without a type attribute from its text:
// a missing symbol or stub the runner names, or pytest's rewritten
// "assert ...", an AssertionError.
func raisedType(i junitIssue) string {
	msg := strings.TrimSpace(i.Message)
	if m := exUnitRaised.FindStringSubmatch(i.Text); m != nil && notReached[m[1]] {
		return m[1]
	}
	if m := pytestRaised.FindStringSubmatch(msg); m != nil && notReached[m[1]] {
		return m[1]
	}
	if strings.HasPrefix(msg, "assert ") {
		return "AssertionError"
	}
	return ""
}

type junitSuite struct {
	Suites []junitSuite `xml:"testsuite"`
	Cases  []junitCase  `xml:"testcase"`
	Errors []junitIssue `xml:"error"` // suite-level, e.g. busted load errors
}

type junitCase struct {
	File      string       `xml:"file,attr"`
	Classname string       `xml:"classname,attr"`
	Name      string       `xml:"name,attr"`
	Failures  []junitIssue `xml:"failure"`
	Errors    []junitIssue `xml:"error"`
	Skipped   *struct{}    `xml:"skipped"`
}

type junitIssue struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Text    string `xml:",chardata"`
}

func (i junitIssue) text() string {
	return strings.TrimSpace(strings.TrimSpace(i.Message) + "\n" + strings.TrimSpace(i.Text))
}

// ReadJUnit reads JUnit XML with either <testsuites> or <testsuite> as the
// root; suites may nest.
func ReadJUnit(r io.Reader) (Report, error) {
	var root junitSuite
	if err := xml.NewDecoder(r).Decode(&root); err != nil {
		return Report{}, err
	}
	var rep Report
	collectJUnit(root, &rep)
	return rep, nil
}

func collectJUnit(s junitSuite, rep *Report) {
	if len(s.Errors) > 0 {
		rep.BuildBroken = true
	}
	for _, c := range s.Cases {
		res := Result{File: c.File, Class: c.Classname, Name: c.Name}
		switch {
		case len(c.Errors) > 0:
			res.Status, res.Kind, res.Type, res.Message = Failed, KindError, c.Errors[0].Type, c.Errors[0].text()
		case len(c.Failures) > 0:
			res.Status, res.Kind, res.Type, res.Message = Failed, KindFailure, c.Failures[0].Type, c.Failures[0].text()
			if res.Type == "" {
				res.Type = raisedType(c.Failures[0])
			}
		case c.Skipped != nil:
			res.Status = Skipped
		}
		rep.Tests = append(rep.Tests, res)
	}
	for _, sub := range s.Suites {
		collectJUnit(sub, rep)
	}
}
