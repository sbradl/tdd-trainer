package judge

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// openTestEngine skips unless the libraries and the model are installed
// (tddt setup) and -short is off.
func openTestEngine(t *testing.T) *Engine {
	t.Helper()
	if testing.Short() {
		t.Skip("needs the model")
	}
	lib, model := DefaultLibDir(), DefaultModel()
	if _, err := os.Stat(filepath.Join(lib, libName())); err != nil {
		t.Skipf("no llama.cpp libraries in %s", lib)
	}
	if _, err := os.Stat(model); err != nil {
		t.Skipf("no model at %s", model)
	}
	e, err := OpenEngine(EngineOptions{LibDir: lib, Model: model, CPU: os.Getenv("TDDT_CPU") != ""})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(e.Close)
	return e
}

func TestTokensMatchPythonReference(t *testing.T) {
	e := openTestEngine(t)
	ref := loadRef(t)
	for id, p := range probes(t) {
		got, want := e.Tokenize(p), ref[id].IDs
		same := len(got) == len(want)
		for i := 0; same && i < len(got); i++ {
			same = got[i] == want[i]
		}
		if !same {
			t.Errorf("%s: %d tokens, reference %d, or ids differ", id, len(got), len(want))
		}
	}
}

func TestEngineScoresOneFixture(t *testing.T) {
	e := openTestEngine(t)
	fx, _ := Fixtures()
	for _, f := range fx {
		if f.Name != "red-check.no.cs-build" {
			continue
		}
		vs, err := Judge{e}.Evaluate(context.Background(), f.Evidence, []string{"red-check"})
		if err != nil {
			t.Fatal(err)
		}
		if vs[0].Answer != "no" {
			t.Fatalf("got %+v", vs[0])
		}
		return
	}
	t.Fatal("fixture not found")
}
