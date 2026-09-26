package watch

import (
	"context"
	"os"
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

const debounce = 100 * time.Millisecond

func start(t *testing.T) (string, <-chan []string) {
	t.Helper()
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "vendor"), 0o755)
	os.MkdirAll(filepath.Join(root, "pkg"), 0o755)
	w, err := New(root, cfg, debounce)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	batches, _ := w.Run(ctx)
	return root, batches
}

func touch(t *testing.T, root, rel string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, []byte(time.Now().String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

func next(t *testing.T, batches <-chan []string) string {
	t.Helper()
	select {
	case b := <-batches:
		return strings.Join(b, ",")
	case <-time.After(2 * time.Second):
		t.Fatal("no batch")
		return ""
	}
}

func none(t *testing.T, batches <-chan []string) {
	t.Helper()
	select {
	case b := <-batches:
		t.Fatalf("unexpected batch %v", b)
	case <-time.After(3 * debounce):
	}
}

func TestRapidSavesMakeOneBatch(t *testing.T) {
	root, batches := start(t)
	for i := 0; i < 5; i++ {
		touch(t, root, "kata.go")
		touch(t, root, "kata_test.go")
		time.Sleep(debounce / 4)
	}
	if got := next(t, batches); got != "kata.go,kata_test.go" {
		t.Fatalf("got %q", got)
	}
	none(t, batches)
}

func TestIgnoredAndOtherFilesDontTrigger(t *testing.T) {
	root, batches := start(t)
	touch(t, root, "vendor/x.go")
	touch(t, root, "README.md")
	touch(t, root, ".tddtrainer/x.go")
	none(t, batches)
}

func TestSubfolderAndNewFolder(t *testing.T) {
	root, batches := start(t)
	touch(t, root, "pkg/a.go")
	if got := next(t, batches); got != "pkg/a.go" {
		t.Fatalf("got %q", got)
	}
	touch(t, root, "newpkg/deep/b_test.go")
	if got := next(t, batches); !strings.Contains(got, "newpkg/deep/b_test.go") {
		t.Fatalf("got %q", got)
	}
	// the new folder is watched from now on
	time.Sleep(debounce)
	touch(t, root, "newpkg/deep/b_test.go")
	if got := next(t, batches); got != "newpkg/deep/b_test.go" {
		t.Fatalf("got %q", got)
	}
}

func TestRemoveTriggers(t *testing.T) {
	root, batches := start(t)
	touch(t, root, "a.go")
	next(t, batches)
	os.Remove(filepath.Join(root, "a.go"))
	if got := next(t, batches); got != "a.go" {
		t.Fatalf("got %q", got)
	}
}
