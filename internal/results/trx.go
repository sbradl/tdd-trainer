package results

import (
	"bufio"
	"encoding/xml"
	"io"
	"strings"
)

type trxRun struct {
	Results []struct {
		TestID   string `xml:"testId,attr"`
		TestName string `xml:"testName,attr"`
		Outcome  string `xml:"outcome,attr"`
		Message  string `xml:"Output>ErrorInfo>Message"`
	} `xml:"Results>UnitTestResult"`
	Definitions []struct {
		ID     string `xml:"id,attr"`
		Method struct {
			ClassName string `xml:"className,attr"`
		} `xml:"TestMethod"`
	} `xml:"TestDefinitions>UnitTest"`
}

// ReadTRX reads a Visual Studio .trx file as written by `dotnet test
// --logger trx`. TRX has no <failure>/<error> split, so outcome "Error"
// counts as an error and "Failed" as a failure.
func ReadTRX(r io.Reader) (Report, error) {
	br := bufio.NewReader(r)
	if b, err := br.Peek(3); err == nil && string(b) == "\xef\xbb\xbf" {
		br.Discard(3)
	}
	var run trxRun
	if err := xml.NewDecoder(br).Decode(&run); err != nil {
		return Report{}, err
	}
	class := map[string]string{}
	for _, d := range run.Definitions {
		class[d.ID] = d.Method.ClassName
	}
	var rep Report
	for _, u := range run.Results {
		c := class[u.TestID]
		res := Result{Class: c, Name: strings.TrimPrefix(u.TestName, c+"."), Message: strings.TrimSpace(u.Message)}
		switch u.Outcome {
		case "Passed":
			res.Status = Passed
		case "Failed", "Timeout", "Aborted":
			res.Status, res.Kind = Failed, KindFailure
		case "Error":
			res.Status, res.Kind = Failed, KindError
		default: // NotExecuted, Inconclusive, ...
			res.Status = Skipped
		}
		rep.Tests = append(rep.Tests, res)
	}
	return rep, nil
}
