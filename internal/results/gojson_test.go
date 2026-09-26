package results

import (
	"strings"
	"testing"
)

func readGo(t *testing.T, out string) TestState {
	t.Helper()
	rep, err := ReadGoJSON(strings.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	return rep.State()
}

func TestGoJSONAllPassing(t *testing.T) {
	st := readGo(t, `{"Action":"run","Package":"kata","Test":"TestAdd"}
{"Action":"pass","Package":"kata","Test":"TestAdd","Elapsed":0}
{"Action":"pass","Package":"kata","Elapsed":0.01}
`)
	if !st.Green() {
		t.Fatalf("want all green, got %+v", st)
	}
}

func TestGoJSONFailingTests(t *testing.T) {
	st := readGo(t, `{"Action":"run","Package":"kata","Test":"TestAdd"}
{"Action":"output","Package":"kata","Test":"TestAdd","Output":"    kata_test.go:7: want 3\n"}
{"Action":"fail","Package":"kata","Test":"TestAdd","Elapsed":0}
{"Action":"run","Package":"kata","Test":"TestSub"}
{"Action":"pass","Package":"kata","Test":"TestSub","Elapsed":0}
{"Action":"run","Package":"kata/sub","Test":"TestX"}
{"Action":"run","Package":"kata/sub","Test":"TestX/case_1"}
{"Action":"fail","Package":"kata/sub","Test":"TestX/case_1","Elapsed":0}
{"Action":"fail","Package":"kata/sub","Test":"TestX","Elapsed":0}
{"Action":"fail","Package":"kata","Elapsed":0.01}
`)
	ids := []string{}
	for _, f := range st.Failing {
		ids = append(ids, f.ID)
	}
	want := "kata/sub::TestX/case_1,kata::TestAdd"
	if st.BuildBroken || strings.Join(ids, ",") != want {
		t.Fatalf("want failing %v, got %+v", want, st)
	}
	if got := strings.Join(st.Tests, ","); got != "kata/sub::TestX,kata/sub::TestX/case_1,kata::TestAdd,kata::TestSub" {
		t.Fatalf("tests %s", got)
	}
	if !strings.Contains(st.Failing[1].Message, "want 3") {
		t.Fatalf("want test output as message, got %q", st.Failing[1].Message)
	}
}

func TestGoJSONBuildFailure(t *testing.T) {
	st := readGo(t, `{"ImportPath":"kata [kata.test]","Action":"build-output","Output":"./kata_test.go:5:2: undefined: Add\n"}
{"ImportPath":"kata [kata.test]","Action":"build-fail"}
{"Action":"start","Package":"kata"}
{"Action":"output","Package":"kata","Output":"FAIL\tkata [build failed]\n"}
{"Action":"fail","Package":"kata","Elapsed":0,"FailedBuild":"kata [kata.test]"}
`)
	if !st.BuildBroken {
		t.Fatalf("want build broken, got %+v", st)
	}
}

func TestGoJSONPackageFailWithoutTests(t *testing.T) {
	// e.g. TestMain exits non-zero or a panic outside a test
	st := readGo(t, `{"Action":"start","Package":"kata"}
{"Action":"fail","Package":"kata","Elapsed":0}
`)
	if !st.BuildBroken {
		t.Fatalf("want build broken, got %+v", st)
	}
}
