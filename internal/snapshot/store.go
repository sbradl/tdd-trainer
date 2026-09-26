// Package snapshot keeps a shadow git repository in .tddtrainer/ with one
// commit per test run, holding the project's test and source files. It
// never touches the learner's own repository and needs no system git.
package snapshot

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	fdiff "github.com/go-git/go-git/v5/plumbing/format/diff"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/filesystem"

	"github.com/sbradl/tdd-trainer/internal/config"
)

// Dir is the coach's own folder in the project root.
const Dir = ".tddtrainer"

const ref = plumbing.ReferenceName("refs/heads/snapshots")

// ID names one snapshot (a commit hash).
type ID string

type Store struct {
	root string
	cfg  config.Config
	st   *filesystem.Storage

	mu     sync.Mutex
	head   plumbing.Hash
	hashes map[string]fileStamp // rel path -> last seen stamp and blob
}

type fileStamp struct {
	mod  time.Time
	size int64
	blob plumbing.Hash
}

// Open creates or opens the store under root/.tddtrainer.
func Open(root string, cfg config.Config) (*Store, error) {
	dir := filepath.Join(root, Dir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	// Keep the coach's folder out of the learner's repository.
	gi := filepath.Join(dir, ".gitignore")
	if _, err := os.Stat(gi); os.IsNotExist(err) {
		if err := os.WriteFile(gi, []byte("*\n"), 0o644); err != nil {
			return nil, err
		}
	}
	st := filesystem.NewStorage(osfs.New(filepath.Join(dir, "snapshots.git")), cache.NewObjectLRUDefault())
	if _, err := st.Reference(plumbing.HEAD); errors.Is(err, plumbing.ErrReferenceNotFound) {
		if _, err := git.Init(st, nil); err != nil {
			return nil, err
		}
		if err := st.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, ref)); err != nil {
			return nil, err
		}
	}
	s := &Store{root: root, cfg: cfg, st: st, hashes: map[string]fileStamp{}}
	if r, err := st.Reference(ref); err == nil {
		s.head = r.Hash()
	} else if !errors.Is(err, plumbing.ErrReferenceNotFound) {
		return nil, err
	}
	return s, nil
}

// Snapshot records the current test and source files as a new commit.
func (s *Store) Snapshot(msg string) (ID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	files := map[string]plumbing.Hash{}
	seen := map[string]bool{}
	err := filepath.WalkDir(s.root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == s.root {
				return err
			}
			return nil // vanished meanwhile
		}
		rel, _ := filepath.Rel(s.root, p)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel != "." && (strings.HasPrefix(d.Name(), ".") || s.cfg.Ignored(rel)) {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() || s.cfg.FileKind(rel) == config.Other {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		seen[rel] = true
		if old, ok := s.hashes[rel]; ok && old.mod.Equal(info.ModTime()) && old.size == info.Size() {
			files[rel] = old.blob
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		h, err := s.writeObject(plumbing.BlobObject, data)
		if err != nil {
			return err
		}
		s.hashes[rel] = fileStamp{info.ModTime(), info.Size(), h}
		files[rel] = h
		return nil
	})
	if err != nil {
		return "", err
	}
	for rel := range s.hashes {
		if !seen[rel] {
			delete(s.hashes, rel)
		}
	}

	tree, err := s.writeTree(files, "")
	if err != nil {
		return "", err
	}
	sig := object.Signature{Name: "tddt", Email: "tddt@localhost", When: time.Now()}
	c := &object.Commit{Author: sig, Committer: sig, Message: msg, TreeHash: tree}
	if !s.head.IsZero() {
		c.ParentHashes = []plumbing.Hash{s.head}
	}
	obj := s.st.NewEncodedObject()
	if err := c.Encode(obj); err != nil {
		return "", err
	}
	h, err := s.st.SetEncodedObject(obj)
	if err != nil {
		return "", err
	}
	if err := s.st.SetReference(plumbing.NewHashReference(ref, h)); err != nil {
		return "", err
	}
	s.head = h
	return ID(h.String()), nil
}

func (s *Store) writeObject(t plumbing.ObjectType, data []byte) (plumbing.Hash, error) {
	obj := s.st.NewEncodedObject()
	obj.SetType(t)
	obj.SetSize(int64(len(data)))
	w, err := obj.Writer()
	if err != nil {
		return plumbing.ZeroHash, err
	}
	if _, err := w.Write(data); err != nil {
		return plumbing.ZeroHash, err
	}
	if err := w.Close(); err != nil {
		return plumbing.ZeroHash, err
	}
	if s.st.HasEncodedObject(obj.Hash()) == nil {
		return obj.Hash(), nil
	}
	return s.st.SetEncodedObject(obj)
}

// writeTree writes the tree for the files under prefix (slash-terminated
// or empty) and returns its hash.
func (s *Store) writeTree(files map[string]plumbing.Hash, prefix string) (plumbing.Hash, error) {
	subdirs := map[string]bool{}
	var entries []object.TreeEntry
	for rel, h := range files {
		if !strings.HasPrefix(rel, prefix) {
			continue
		}
		name, _, isDir := strings.Cut(rel[len(prefix):], "/")
		if isDir {
			subdirs[name] = true
			continue
		}
		entries = append(entries, object.TreeEntry{Name: name, Mode: filemode.Regular, Hash: h})
	}
	for name := range subdirs {
		h, err := s.writeTree(files, prefix+name+"/")
		if err != nil {
			return plumbing.ZeroHash, err
		}
		entries = append(entries, object.TreeEntry{Name: name, Mode: filemode.Dir, Hash: h})
	}
	// git orders tree entries as if folder names ended in "/".
	key := func(e object.TreeEntry) string {
		if e.Mode == filemode.Dir {
			return e.Name + "/"
		}
		return e.Name
	}
	sort.Slice(entries, func(i, j int) bool { return key(entries[i]) < key(entries[j]) })

	t := &object.Tree{Entries: entries}
	obj := s.st.NewEncodedObject()
	if err := t.Encode(obj); err != nil {
		return plumbing.ZeroHash, err
	}
	if s.st.HasEncodedObject(obj.Hash()) == nil {
		return obj.Hash(), nil
	}
	return s.st.SetEncodedObject(obj)
}

func (s *Store) tree(id ID) (*object.Tree, error) {
	c, err := object.GetCommit(s.st, plumbing.NewHash(string(id)))
	if err != nil {
		return nil, fmt.Errorf("snapshot %s: %w", id, err)
	}
	return c.Tree()
}

// FileDiff is the change of one file between two snapshots.
type FileDiff struct {
	Path           string // new path; old path for deletions
	Kind           config.FileKind
	Patch          string // unified diff
	Added, Removed int    // lines
}

// Diff returns the changed files from one snapshot to another, sorted by
// path.
func (s *Store) Diff(from, to ID) ([]FileDiff, error) {
	a, err := s.tree(from)
	if err != nil {
		return nil, err
	}
	b, err := s.tree(to)
	if err != nil {
		return nil, err
	}
	p, err := a.Patch(b)
	if err != nil {
		return nil, err
	}
	var out []FileDiff
	for _, fp := range p.FilePatches() {
		fromF, toF := fp.Files()
		d := FileDiff{}
		if toF != nil {
			d.Path = toF.Path()
		} else {
			d.Path = fromF.Path()
		}
		d.Kind = s.cfg.FileKind(d.Path)
		for _, c := range fp.Chunks() {
			n := strings.Count(c.Content(), "\n")
			if !strings.HasSuffix(c.Content(), "\n") && c.Content() != "" {
				n++
			}
			switch c.Type() {
			case fdiff.Add:
				d.Added += n
			case fdiff.Delete:
				d.Removed += n
			}
		}
		var buf bytes.Buffer
		if err := fdiff.NewUnifiedEncoder(&buf, fdiff.DefaultContextLines).Encode(onePatch{fp}); err != nil {
			return nil, err
		}
		d.Patch = buf.String()
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

type onePatch struct{ fp fdiff.FilePatch }

func (p onePatch) FilePatches() []fdiff.FilePatch { return []fdiff.FilePatch{p.fp} }
func (p onePatch) Message() string                { return "" }

// Files returns the content of the snapshot's files of the given kind,
// keyed by slash-separated path.
func (s *Store) Files(id ID, kind config.FileKind) (map[string]string, error) {
	t, err := s.tree(id)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	err = t.Files().ForEach(func(f *object.File) error {
		if s.cfg.FileKind(f.Name) != kind {
			return nil
		}
		r, err := f.Reader()
		if err != nil {
			return err
		}
		defer r.Close()
		data, err := io.ReadAll(r)
		if err != nil {
			return err
		}
		out[path.Clean(f.Name)] = string(data)
		return nil
	})
	return out, err
}
