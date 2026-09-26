package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sbradl/tdd-trainer/internal/config"
)

func write(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(path), 0o755)
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func initIn(t *testing.T, root, input string, o initOpts) (initResult, string) {
	t.Helper()
	o.root = root
	var out bytes.Buffer
	res, err := runInit(strings.NewReader(input), &out, o)
	if err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	return res, out.String()
}

func presetIn(t *testing.T, dir string) string {
	t.Helper()
	c, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return c.Preset
}

func TestInitConfirmsSingleMatch(t *testing.T) {
	root := t.TempDir()
	write(t, root, map[string]string{"go.mod": "module k"})
	res, out := initIn(t, root, "\n", initOpts{})
	if !strings.Contains(out, "Detected Go.") || res.dir != root || presetIn(t, root) != "go" {
		t.Fatalf("res %+v out:\n%s", res, out)
	}
}

func TestInitDeclinedMatchOffersList(t *testing.T) {
	root := t.TempDir()
	write(t, root, map[string]string{"go.mod": "module k"})
	_, out := initIn(t, root, "n\n2\n", initOpts{})
	if !strings.Contains(out, "Choose a preset") || presetIn(t, root) != "python" {
		t.Fatalf("out:\n%s", out)
	}
}

func TestInitPicksAmongSeveralAndRootsConfigThere(t *testing.T) {
	root := t.TempDir()
	write(t, root, map[string]string{
		"api/go.mod":         "module api",
		"lib/pyproject.toml": "",
	})
	res, out := initIn(t, root, "x\n2\n", initOpts{})
	want := filepath.Join(root, "lib")
	if res.dir != want || presetIn(t, want) != "python" || !strings.Contains(out, "Please enter a number") {
		t.Fatalf("res %+v out:\n%s", res, out)
	}
	if _, err := os.Stat(filepath.Join(root, config.FileName)); !os.IsNotExist(err) {
		t.Fatal("config written at root")
	}
}

func TestInitNoMatchOffersAllAndBlank(t *testing.T) {
	root := t.TempDir()
	res, out := initIn(t, root, "10\n", initOpts{})
	if res.ready || !strings.Contains(out, "No known project type") || !strings.Contains(out, "Fill in the TODOs") {
		t.Fatalf("res %+v out:\n%s", res, out)
	}
}

func TestInitPrintsSetupSteps(t *testing.T) {
	root := t.TempDir()
	write(t, root, map[string]string{"mix.exs": "[app: :bowling]"})
	res, out := initIn(t, root, "\n", initOpts{})
	if res.ready || !strings.Contains(out, "junit_formatter") || !strings.Contains(out, "JUnitFormatter") {
		t.Fatalf("out:\n%s", out)
	}
	data, _ := os.ReadFile(filepath.Join(root, config.FileName))
	if !strings.Contains(string(data), "_build/test/lib/bowling/") {
		t.Fatalf("config:\n%s", data)
	}
}

func TestInitPresetFlagKeepsDetectedVars(t *testing.T) {
	root := t.TempDir()
	write(t, root, map[string]string{"mix.exs": "[app: :bowling]"})
	initIn(t, root, "", initOpts{preset: "elixir"})
	data, _ := os.ReadFile(filepath.Join(root, config.FileName))
	if !strings.Contains(string(data), "/bowling/") {
		t.Fatalf("config:\n%s", data)
	}
}

func TestInitYes(t *testing.T) {
	root := t.TempDir()
	write(t, root, map[string]string{"go.mod": "module k"})
	initIn(t, root, "", initOpts{yes: true})
	if presetIn(t, root) != "go" {
		t.Fatal("not written")
	}
	// existing config is never overwritten silently
	if _, err := runInit(strings.NewReader(""), &bytes.Buffer{}, initOpts{root: root, yes: true}); err == nil {
		t.Fatal("want error for existing config")
	}
}

func TestInitAsksBeforeOverwrite(t *testing.T) {
	root := t.TempDir()
	write(t, root, map[string]string{"go.mod": "module k", config.FileName: "test: {cmd: keep}\n"})
	res, _ := initIn(t, root, "\n\n", initOpts{})
	data, _ := os.ReadFile(filepath.Join(root, config.FileName))
	if res.dir != "" || !strings.Contains(string(data), "keep") {
		t.Fatalf("overwritten: %s", data)
	}
}

func TestInitUnknownPreset(t *testing.T) {
	if _, err := runInit(strings.NewReader(""), &bytes.Buffer{}, initOpts{root: t.TempDir(), preset: "cobol"}); err == nil {
		t.Fatal("want error")
	}
}

func TestRunWithoutConfigStartsInit(t *testing.T) {
	root := t.TempDir()
	write(t, root, map[string]string{
		"go.mod":    "module k\n\ngo 1.22\n",
		"k_test.go": "package k\n\nimport \"testing\"\n\nfunc TestA(t *testing.T) {}\n",
	})
	var out bytes.Buffer
	if err := run([]string{"--once", root}, strings.NewReader("\n"), &out); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "let's create one") || !strings.HasSuffix(out.String(), "all green\n") {
		t.Fatalf("out:\n%s", out.String())
	}
}
