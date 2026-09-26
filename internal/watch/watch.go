// Package watch reports debounced batches of changed test and source files.
package watch

import (
	"context"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/sbradl/tdd-trainer/internal/config"
)

// DefaultDebounce is how long the project must be quiet before a batch is
// reported.
const DefaultDebounce = 400 * time.Millisecond

type Watcher struct {
	root     string
	cfg      config.Config
	fsw      *fsnotify.Watcher
	debounce time.Duration
}

// New watches root and all its folders that are neither hidden nor ignored.
func New(root string, cfg config.Config, debounce time.Duration) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	w := &Watcher{root: root, cfg: cfg, fsw: fsw, debounce: debounce}
	if _, err := w.addTree(root); err != nil {
		fsw.Close()
		return nil, err
	}
	return w, nil
}

// Run sends each debounced batch of changed test and source files
// (slash-separated, relative to root, sorted) until ctx ends; then it
// closes the channel and the watcher.
func (w *Watcher) Run(ctx context.Context) (<-chan []string, <-chan error) {
	batches := make(chan []string)
	errs := make(chan error, 1)
	go func() {
		defer close(batches)
		defer w.fsw.Close()
		pending := map[string]bool{}
		timer := time.NewTimer(time.Hour)
		timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case err, ok := <-w.fsw.Errors:
				if !ok {
					return
				}
				select {
				case errs <- err:
				default:
				}
			case ev, ok := <-w.fsw.Events:
				if !ok {
					return
				}
				changed := w.handle(ev)
				for _, rel := range changed {
					pending[rel] = true
				}
				if len(changed) > 0 {
					timer.Reset(w.debounce)
				}
			case <-timer.C:
				batch := make([]string, 0, len(pending))
				for rel := range pending {
					batch = append(batch, rel)
				}
				slices.Sort(batch)
				clear(pending)
				select {
				case batches <- batch:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return batches, errs
}

// handle returns the relevant files an event touched. A new folder is
// watched too, and the files already inside it count as changed.
func (w *Watcher) handle(ev fsnotify.Event) []string {
	if ev.Op == fsnotify.Chmod {
		return nil
	}
	rel, ok := w.rel(ev.Name)
	if !ok {
		return nil
	}
	if ev.Has(fsnotify.Create) {
		if files, err := w.addTree(ev.Name); err == nil && files != nil {
			return files
		}
	}
	if w.cfg.FileKind(rel) == config.Other {
		return nil
	}
	return []string{rel}
}

// addTree watches dir and its subfolders; it returns the relevant files
// found, or nil if path is not a folder.
func (w *Watcher) addTree(dir string) ([]string, error) {
	var files []string
	isDir := false
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path == dir {
				return err
			}
			return nil // vanished meanwhile
		}
		rel, ok := w.rel(path)
		if !ok {
			return nil
		}
		if !d.IsDir() {
			if w.cfg.FileKind(rel) != config.Other {
				files = append(files, rel)
			}
			return nil
		}
		if path == dir {
			isDir = true
		}
		if rel != "." && (strings.HasPrefix(d.Name(), ".") || w.cfg.Ignored(rel)) {
			return filepath.SkipDir
		}
		return w.fsw.Add(path)
	})
	if err != nil || !isDir {
		return nil, err
	}
	if files == nil {
		files = []string{}
	}
	return files, nil
}

func (w *Watcher) rel(path string) (string, bool) {
	rel, err := filepath.Rel(w.root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(rel), true
}
