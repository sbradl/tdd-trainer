package results

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Fixtures in testdata/ are real runner output (busted: written from its
// junit handler source). Paths were rewritten to /work.
func TestReadersNormaliseFixtures(t *testing.T) {
	cases := []struct {
		fixture, format string
		want            TestState
	}{
		{"pytest.xml", "junit-xml", TestState{Failing: []Failure{
			{ID: "tests.test_kata::test_add", Reason: ReasonUndecided},
			{ID: "tests.test_kata::test_error", Reason: ReasonUndecided},
		}}},
		{"pytest-collect-error.xml", "junit-xml", TestState{Failing: []Failure{
			{ID: "test_b", Reason: ReasonWrong},
		}}},
		{"vitest.xml", "junit-xml", TestState{Failing: []Failure{
			{ID: "src/kata.test.js::add > adds two numbers", Reason: ReasonAssertion},
			{ID: "src/kata.test.js::add > errors", Reason: ReasonWrong},
		}}},
		{"vitest-load-error.xml", "junit-xml", TestState{Failing: []Failure{
			{ID: "src/broken.test.js", Reason: ReasonWrong},
		}}},
		{"jest.xml", "junit-xml", TestState{Failing: []Failure{
			{ID: "src/broken.test.js::Test suite failed to run::src/broken.test.js", Reason: ReasonWrong},
			{ID: "src/kata.test.js::add::adds two numbers", Reason: ReasonUndecided},
			{ID: "src/kata.test.js::add::errors", Reason: ReasonUndecided},
		}}},
		{"busted.xml", "junit-xml", TestState{Failing: []Failure{
			{ID: "spec.kata_spec_lua::kata add adds two numbers", Reason: ReasonUndecided},
			{ID: "spec.kata_spec_lua::kata add errors", Reason: ReasonWrong},
		}}},
		{"busted-load-error.xml", "junit-xml", TestState{BuildBroken: true}},
		{"exunit.xml", "junit-xml", TestState{Failing: []Failure{
			{ID: "Elixir.KataTest::test add adds two numbers", Reason: ReasonUndecided},
			{ID: "Elixir.KataTest::test add raises", Reason: ReasonUndecided},
		}}},
		{"xunit.trx", "trx", TestState{Failing: []Failure{
			{ID: "Kata.Tests.CalcTests::Adds", Reason: ReasonUndecided},
			{ID: "Kata.Tests.CalcTests::Theory(x: 2)", Reason: ReasonUndecided},
			{ID: "Kata.Tests.CalcTests::Throws", Reason: ReasonUndecided},
		}}},
	}
	for _, c := range cases {
		t.Run(c.fixture, func(t *testing.T) {
			f, err := os.Open(filepath.Join("testdata", c.fixture))
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			rep, err := Read(c.format, f)
			if err != nil {
				t.Fatal(err)
			}
			got := rep.State()
			for i := range got.Failing {
				if got.Failing[i].Message == "" {
					t.Errorf("%s: empty message", got.Failing[i].ID)
				}
				got.Failing[i].Message = ""
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("\ngot  %+v\nwant %+v", got, c.want)
			}
		})
	}
}

func TestPassedAndSkippedAreNotFailing(t *testing.T) {
	rep := Report{Tests: []Result{
		{Class: "c", Name: "a", Status: Passed},
		{Class: "c", Name: "b", Status: Skipped},
	}}
	if st := rep.State(); !st.Green() {
		t.Fatalf("want green, got %v", st)
	}
}

func TestDuplicateIDsMergeWithErrorWinning(t *testing.T) {
	rep := Report{Tests: []Result{
		{Name: "x", Status: Failed, Kind: KindFailure, Type: "AssertionError", Message: "a"},
		{Name: "x", Status: Failed, Kind: KindError, Message: "b"},
	}}
	st := rep.State()
	if len(st.Failing) != 1 || st.Failing[0].Reason != ReasonWrong {
		t.Fatalf("got %+v", st)
	}
}

func TestClassify(t *testing.T) {
	cases := []struct {
		r    Result
		want FailReason
	}{
		{Result{Kind: KindError}, ReasonWrong},
		{Result{Kind: KindFailure}, ReasonUndecided},
		{Result{Kind: KindFailure, Type: "AssertionError"}, ReasonAssertion},
		{Result{Kind: KindFailure, Type: "ExUnit.AssertionError"}, ReasonAssertion},
		{Result{Kind: KindFailure, Type: "ReferenceError"}, ReasonWrong},
	}
	for _, c := range cases {
		if got := Classify(c.r); got != c.want {
			t.Errorf("%+v: got %v want %v", c.r, got, c.want)
		}
	}
}

func TestIDStripsBustedLineNumbers(t *testing.T) {
	a := Result{Class: "spec.kata_spec_lua:8", Name: "n"}
	b := Result{Class: "spec.kata_spec_lua:11", Name: "n"}
	if a.ID() != b.ID() {
		t.Fatalf("%q != %q", a.ID(), b.ID())
	}
}

func TestUnknownFormat(t *testing.T) {
	if _, err := Read("tap", nil); err == nil {
		t.Fatal("want error")
	}
}
