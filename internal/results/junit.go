package results

import (
	"encoding/xml"
	"io"
	"strings"
)

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
		case c.Skipped != nil:
			res.Status = Skipped
		}
		rep.Tests = append(rep.Tests, res)
	}
	for _, sub := range s.Suites {
		collectJUnit(sub, rep)
	}
}
