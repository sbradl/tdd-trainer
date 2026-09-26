package config

import "testing"

func TestFileKind(t *testing.T) {
	c := Config{
		Tests:   []string{"**/*_test.go"},
		Sources: []string{"**/*.go"},
		Ignore:  []string{"vendor/**"},
	}
	cases := map[string]FileKind{
		"kata_test.go":     Test,
		"pkg/kata_test.go": Test,
		"kata.go":          Source,
		"pkg/sub/kata.go":  Source,
		"README.md":        Other,
		"vendor/x/y.go":    Other,
		".tddtrainer/HEAD": Other,
		"./kata.go":        Source,
	}
	for rel, want := range cases {
		if got := c.FileKind(rel); got != want {
			t.Errorf("%s: got %v want %v", rel, got, want)
		}
	}
}

func TestIgnoredFolders(t *testing.T) {
	c := Config{Ignore: []string{"node_modules/**", "**/bin/**"}}
	for _, rel := range []string{"node_modules", "node_modules/x", "a/bin", ".tddtrainer"} {
		if !c.Ignored(rel) {
			t.Errorf("%s not ignored", rel)
		}
	}
	if c.Ignored("src") {
		t.Error("src ignored")
	}
}
