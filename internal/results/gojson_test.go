package results

import (
	"strings"
	"testing"
)

func TestGoJSONAllPassing(t *testing.T) {
	out := `{"Action":"run","Package":"kata","Test":"TestAdd"}
{"Action":"pass","Package":"kata","Test":"TestAdd","Elapsed":0}
{"Action":"pass","Package":"kata","Elapsed":0.01}
`
	got, err := ReadGoJSON(strings.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if got.BuildBroken || len(got.Failing) != 0 {
		t.Fatalf("want all green, got %+v", got)
	}
}

func TestGoJSONFailingTests(t *testing.T) {
	out := `{"Action":"run","Package":"kata","Test":"TestAdd"}
{"Action":"fail","Package":"kata","Test":"TestAdd","Elapsed":0}
{"Action":"run","Package":"kata","Test":"TestSub"}
{"Action":"pass","Package":"kata","Test":"TestSub","Elapsed":0}
{"Action":"run","Package":"kata/sub","Test":"TestX/case_1"}
{"Action":"fail","Package":"kata/sub","Test":"TestX/case_1","Elapsed":0}
{"Action":"fail","Package":"kata/sub","Test":"TestX","Elapsed":0}
{"Action":"fail","Package":"kata","Elapsed":0.01}
`
	got, err := ReadGoJSON(strings.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"kata.TestAdd", "kata/sub.TestX/case_1"}
	if got.BuildBroken || strings.Join(got.Failing, ",") != strings.Join(want, ",") {
		t.Fatalf("want failing %v, got %+v", want, got)
	}
}

func TestGoJSONBuildFailure(t *testing.T) {
	out := `{"ImportPath":"kata [kata.test]","Action":"build-output","Output":"./kata_test.go:5:2: undefined: Add\n"}
{"ImportPath":"kata [kata.test]","Action":"build-fail"}
{"Action":"start","Package":"kata"}
{"Action":"output","Package":"kata","Output":"FAIL\tkata [build failed]\n"}
{"Action":"fail","Package":"kata","Elapsed":0,"FailedBuild":"kata [kata.test]"}
`
	got, err := ReadGoJSON(strings.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if !got.BuildBroken {
		t.Fatalf("want build broken, got %+v", got)
	}
}

func TestGoJSONPackageFailWithoutTests(t *testing.T) {
	// e.g. TestMain exits non-zero or a panic outside a test
	out := `{"Action":"start","Package":"kata"}
{"Action":"fail","Package":"kata","Elapsed":0}
`
	got, err := ReadGoJSON(strings.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if !got.BuildBroken {
		t.Fatalf("want build broken, got %+v", got)
	}
}
