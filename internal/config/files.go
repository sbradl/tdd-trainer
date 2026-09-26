package config

import (
	"path"

	"github.com/bmatcuk/doublestar/v4"
)

// FileKind says what role a project file plays in step logic.
type FileKind int

const (
	Other  FileKind = iota // matches neither tests nor sources, or is ignored
	Test                   // test file; wins over Source when both match
	Source                 // production code
)

// FileKind classifies rel, a slash-separated path relative to the project root.
func (c Config) FileKind(rel string) FileKind {
	switch {
	case c.Ignored(rel):
		return Other
	case matchAny(c.Tests, rel):
		return Test
	case matchAny(c.Sources, rel):
		return Source
	}
	return Other
}

// Ignored reports whether rel (file or folder) is excluded from watching
// and snapshots. The coach's own .tddtrainer folder always is.
func (c Config) Ignored(rel string) bool {
	return rel == ".tddtrainer" || matchAny([]string{".tddtrainer/**"}, rel) || matchAny(c.Ignore, rel)
}

func matchAny(globs []string, rel string) bool {
	rel = path.Clean(rel)
	for _, g := range globs {
		if ok, _ := doublestar.Match(g, rel); ok {
			return true
		}
	}
	return false
}
