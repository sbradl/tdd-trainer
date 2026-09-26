package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

const goPreset = `preset: go
build: go vet ./...
test:
  cmd: go test -json ./...
  windows: go test -json -count=1 ./...
results: { format: go-json }
tests:   ["**/*_test.go"]
sources: ["**/*.go"]
ignore:  [".tddtrainer/**", "vendor/**"]
tpp_order: iteration-first
slow_run_warning: 5s
`

func TestLoadGoPreset(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, FileName), []byte(goPreset), 0o644)

	c, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if c.Build != "go vet ./..." || c.Test.Cmd != "go test -json ./..." || c.Results.Format != "go-json" {
		t.Fatalf("unexpected config %+v", c)
	}
	if c.SlowRunWarning != 5*time.Second || c.TPPOrder != "iteration-first" {
		t.Fatalf("unexpected config %+v", c)
	}
	if got := c.TestCmd("windows"); got != "go test -json -count=1 ./..." {
		t.Fatalf("windows cmd: %q", got)
	}
	if got := c.TestCmd("linux"); got != "go test -json ./..." {
		t.Fatalf("linux cmd: %q", got)
	}
}

func TestLoadRequiresTestCmd(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, FileName), []byte("preset: go\n"), 0o644)
	if _, err := Load(dir); err == nil {
		t.Fatal("want error for missing test.cmd")
	}
}

func TestLoadRejectsUnknownFields(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, FileName), []byte("test: {cmd: x}\nbiuld: y\n"), 0o644)
	if _, err := Load(dir); err == nil {
		t.Fatal("want error for typo'd field")
	}
}

func TestLoadRejectsUnknownFormat(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, FileName), []byte("test: {cmd: x}\nresults: {format: tap}\n"), 0o644)
	if _, err := Load(dir); err == nil {
		t.Fatal("want error for unknown format")
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load(t.TempDir()); !os.IsNotExist(err) {
		t.Fatalf("want not-exist error, got %v", err)
	}
}
