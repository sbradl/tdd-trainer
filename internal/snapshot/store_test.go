package snapshot

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sbradl/tdd-trainer/internal/config"
)

var cfg = config.Config{
	Tests:   []string{"**/*_test.go"},
	Sources: []string{"**/*.go"},
	Ignore:  []string{"vendor/**"},
}

func write(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	// mtime granularity: make sure a rewrite is seen as changed
	future := time.Now().Add(time.Duration(len(body)) * time.Millisecond)
	os.Chtimes(p, future, future)
}

func snap(t *testing.T, s *Store) ID {
	t.Helper()
	id, err := s.Snapshot("run")
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestDiffSplitsTestAndSource(t *testing.T) {
	root := t.TempDir()
	write(t, root, "kata.go", "package kata\n")
	write(t, root, "kata_test.go", "package kata\n")
	s, err := Open(root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	a := snap(t, s)

	write(t, root, "kata_test.go", "package kata\n\nfunc TestAdd() {}\n")
	b := snap(t, s)
	write(t, root, "kata.go", "package kata\n\nfunc Add() int { return 3 }\n")
	write(t, root, "pkg/new.go", "package pkg\n")
	os.Remove(filepath.Join(root, "kata_test.go"))
	c := snap(t, s)

	d, err := s.Diff(a, b)
	if err != nil {
		t.Fatal(err)
	}
	if len(d) != 1 || d[0].Path != "kata_test.go" || d[0].Kind != config.Test || d[0].Added != 2 || d[0].Removed != 0 {
		t.Fatalf("a→b: %+v", d)
	}
	if !strings.Contains(d[0].Patch, "+func TestAdd() {}") {
		t.Fatalf("patch:\n%s", d[0].Patch)
	}

	// any two snapshots, not only neighbours
	d, err = s.Diff(a, c)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range d {
		got = append(got, f.Path)
	}
	if strings.Join(got, ",") != "kata.go,kata_test.go,pkg/new.go" {
		t.Fatalf("a→c: %v", got)
	}
	if d[0].Kind != config.Source || d[1].Removed != 1 || d[2].Added != 1 {
		t.Fatalf("a→c: %+v", d)
	}
}

func TestOnlyTestAndSourceFilesAreSnapshotted(t *testing.T) {
	root := t.TempDir()
	write(t, root, "kata.go", "package kata\n")
	write(t, root, "README.md", "hi\n")
	write(t, root, "vendor/x/x.go", "package x\n")
	write(t, root, ".hidden/y.go", "package y\n")
	s, _ := Open(root, cfg)
	id := snap(t, s)
	files, err := s.Files(id, config.Source)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files["kata.go"] != "package kata\n" {
		t.Fatalf("files %v", files)
	}
}

func TestNeverTouchesLearnersGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git to check with")
	}
	root := t.TempDir()
	gitc := func(args ...string) string {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	gitc("init", "-q")
	write(t, root, "kata.go", "package kata\n")
	gitc("add", ".")
	gitc("-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "init")
	before := gitc("rev-parse", "HEAD") + gitc("status", "--porcelain")

	s, _ := Open(root, cfg)
	snap(t, s)
	write(t, root, "kata.go", "package kata\n\nvar X = 1\n")
	snap(t, s)

	after := gitc("rev-parse", "HEAD") + gitc("status", "--porcelain")
	if after != before+" M kata.go\n" {
		t.Fatalf("learner repo changed:\nbefore %q\nafter  %q", before, after)
	}
	if strings.Contains(after, ".tddtrainer") {
		t.Fatal(".tddtrainer shows up in git status")
	}
	// the shadow repository is well-formed for real git too
	write(t, root, "pkg/sub/a_test.go", "package sub\n")
	write(t, root, "pkg.go", "package kata\n")
	snap(t, s)
	gitc("--git-dir", filepath.Join(Dir, "snapshots.git"), "fsck", "--strict", "--no-dangling")
}

func TestReopenContinuesHistory(t *testing.T) {
	root := t.TempDir()
	write(t, root, "kata.go", "package kata\n")
	s, _ := Open(root, cfg)
	a := snap(t, s)
	s2, err := Open(root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	write(t, root, "kata.go", "package kata\n\nvar X = 1\n")
	b := snap(t, s2)
	if d, err := s2.Diff(a, b); err != nil || len(d) != 1 {
		t.Fatalf("%v %+v", err, d)
	}
}
