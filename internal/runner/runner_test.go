package runner

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/sbradl/tdd-trainer/internal/config"
	"github.com/sbradl/tdd-trainer/internal/results"
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
	st := mustRun(t, goCfg, dir)
	if !st.Green() {
		t.Fatalf("want green, got %v", st)
	}
}

func TestRunRed(t *testing.T) {
	dir := kata(t, "func Add(a, b int) int { return 0 }\n", addTest)
	st := mustRun(t, goCfg, dir)
	if st.BuildBroken || len(st.Failing) != 1 || st.Failing[0].ID != "kata::TestAdd" {
		t.Fatalf("want kata.TestAdd failing, got %v", st)
	}
}

func TestRunBuildBrokenByBuildCmd(t *testing.T) {
	dir := kata(t, "", addTest) // Add undefined
	st := mustRun(t, goCfg, dir)
	if !st.BuildBroken {
		t.Fatalf("want build broken, got %v", st)
	}
}

func TestRunBuildBrokenWithoutBuildCmd(t *testing.T) {
	cfg := goCfg
	cfg.Build = ""
	dir := kata(t, "", addTest)
	st := mustRun(t, cfg, dir)
	if !st.BuildBroken {
		t.Fatalf("want build broken, got %v", st)
	}
}

func mustRun(t *testing.T, cfg config.Config, dir string) results.TestState {
	t.Helper()
	o, err := Run(context.Background(), cfg, dir)
	if err != nil {
		t.Fatal(err)
	}
	return o.State
}

func TestRunUnknownFormat(t *testing.T) {
	cfg := goCfg
	cfg.Build = ""
	cfg.Results.Format = "tap"
	if _, err := Run(context.Background(), cfg, t.TempDir()); err == nil {
		t.Fatal("want error for unsupported format")
	}
}

func shOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
}

const junitRed = `<testsuite><testcase classname="c" name="t"><failure>boom</failure></testcase></testsuite>`

func TestRunReadsResultFile(t *testing.T) {
	shOnly(t)
	dir := t.TempDir()
	cfg := config.Config{
		Test:    config.TestCmd{Cmd: "mkdir -p out && echo '" + junitRed + "' > out/r.xml; exit 1"},
		Results: config.Results{Format: "junit-xml", Path: "out/r.xml"},
	}
	o, err := Run(context.Background(), cfg, dir)
	if err != nil {
		t.Fatal(err)
	}
	if o.ExitCodeOnly || len(o.State.Failing) != 1 || o.State.Failing[0].ID != "c::t" {
		t.Fatalf("got %+v", o)
	}
}

func TestRunReadsResultsFromStdout(t *testing.T) {
	shOnly(t)
	cfg := config.Config{
		Test:    config.TestCmd{Cmd: "echo '" + junitRed + "'; exit 1"},
		Results: config.Results{Format: "junit-xml"},
	}
	if st := mustRun(t, cfg, t.TempDir()); len(st.Failing) != 1 {
		t.Fatalf("got %v", st)
	}
}

func TestRunIgnoresStaleResultFile(t *testing.T) {
	shOnly(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "r.xml"), []byte(junitRed), 0o644)
	cfg := config.Config{
		Test:    config.TestCmd{Cmd: "echo compile error >&2; exit 1"},
		Results: config.Results{Format: "junit-xml", Path: "r.xml"},
	}
	o, err := Run(context.Background(), cfg, dir)
	if err != nil {
		t.Fatal(err)
	}
	if o.ExitCodeOnly || !o.State.BuildBroken {
		t.Fatalf("got %+v", o)
	}
	if o.Output != "compile error\n" {
		t.Fatalf("output %q", o.Output)
	}
}

func TestRunExitCodeOnlyRed(t *testing.T) {
	shOnly(t)
	o, err := Run(context.Background(), config.Config{Test: config.TestCmd{Cmd: "false"}}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !o.ExitCodeOnly || len(o.State.Failing) != 1 || o.State.Failing[0].ID != ExitCodeOnlyID {
		t.Fatalf("got %+v", o)
	}
}

func TestRunExitCodeOnlyGreen(t *testing.T) {
	shOnly(t)
	o, err := Run(context.Background(), config.Config{Test: config.TestCmd{Cmd: "true"}}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !o.ExitCodeOnly || !o.State.Green() {
		t.Fatalf("got %+v", o)
	}
}

func TestRunCancelled(t *testing.T) {
	shOnly(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Run(ctx, config.Config{Test: config.TestCmd{Cmd: "sleep 5"}}, t.TempDir()); err == nil {
		t.Fatal("want error")
	}
}
