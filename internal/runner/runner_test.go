package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/sbradl/tdd-trainer/internal/config"
)

var goCfg = config.Config{
	Build:   "go vet ./...",
	Test:    config.TestCmd{Cmd: "go test -json ./..."},
	Results: config.Results{Format: "go-json"},
}

func kata(t *testing.T, src, test string) string {
	t.Helper()
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module kata\n\ngo 1.22\n")
	write("kata.go", "package kata\n\n"+src)
	write("kata_test.go", "package kata\n\nimport \"testing\"\n\n"+test)
	return dir
}

const addTest = `func TestAdd(t *testing.T) {
	if Add(1, 2) != 3 {
		t.Fatal("want 3")
	}
}
`

func TestRunGreen(t *testing.T) {
	dir := kata(t, "func Add(a, b int) int { return a + b }\n", addTest)
	st, err := Run(context.Background(), goCfg, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Green() {
		t.Fatalf("want green, got %v", st)
	}
}

func TestRunRed(t *testing.T) {
	dir := kata(t, "func Add(a, b int) int { return 0 }\n", addTest)
	st, err := Run(context.Background(), goCfg, dir)
	if err != nil {
		t.Fatal(err)
	}
	if st.BuildBroken || len(st.Failing) != 1 || st.Failing[0] != "kata.TestAdd" {
		t.Fatalf("want kata.TestAdd failing, got %v", st)
	}
}

func TestRunBuildBrokenByBuildCmd(t *testing.T) {
	dir := kata(t, "", addTest) // Add undefined
	st, err := Run(context.Background(), goCfg, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !st.BuildBroken {
		t.Fatalf("want build broken, got %v", st)
	}
}

func TestRunBuildBrokenWithoutBuildCmd(t *testing.T) {
	cfg := goCfg
	cfg.Build = ""
	dir := kata(t, "", addTest)
	st, err := Run(context.Background(), cfg, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !st.BuildBroken {
		t.Fatalf("want build broken, got %v", st)
	}
}

func TestRunUnknownFormat(t *testing.T) {
	cfg := goCfg
	cfg.Results.Format = "tap"
	if _, err := Run(context.Background(), cfg, t.TempDir()); err == nil {
		t.Fatal("want error for unsupported format")
	}
}
