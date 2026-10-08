package coach

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/sbradl/tdd-trainer/internal/config"
	"github.com/sbradl/tdd-trainer/internal/snapshot"
)

// newTestCode returns the code of a Red's new tests and how many test
// lines were added for them. Learners often tidy other tests in the same
// save, so the added lines alone would show the judge several tests: each
// new test is looked up by name and its whole block taken, counting only
// the block's added lines. A test whose name is not in the file (e.g. a
// templated name) falls back to the added lines that are not just
// re-indented.
func newTestCode(diffs []snapshot.FileDiff, files map[string]string, ids []string) (string, int) {
	var blocks []string
	n := 0
	seen := map[string]bool{}
	for _, id := range ids {
		code, added, ok := findTest(diffs, files, id)
		if !ok {
			return movedOut(diffs)
		}
		if !seen[code] {
			seen[code] = true
			blocks = append(blocks, code)
			n += added
		}
	}
	if len(blocks) == 0 {
		return movedOut(diffs)
	}
	return strings.Join(blocks, "\n\n"), n
}

// findTest finds the block of the test with the given ID among the
// changed test files, preferring the longest form of its name.
func findTest(diffs []snapshot.FileDiff, files map[string]string, id string) (code string, added int, ok bool) {
	for _, name := range testNames(id) {
		for _, d := range diffs {
			if d.Kind != config.Test {
				continue
			}
			lines := strings.Split(strings.TrimSuffix(files[d.Path], "\n"), "\n")
			isAdded := addedLineNumbers(d.Patch)
			for i, l := range lines {
				if !containsName(l, name) {
					continue
				}
				from, to := testBlock(lines, i)
				for j := from; j <= to; j++ {
					if isAdded[j+1] && strings.TrimSpace(lines[j]) != "" {
						added++
					}
				}
				if added > 0 {
					return strings.Join(lines[from:to+1], "\n"), added, true
				}
			}
		}
	}
	return "", 0, false
}

// containsName: the line holds name as a whole word, so TestB is not
// found in TestBig.
func containsName(line, name string) bool {
	for off := 0; ; {
		i := strings.Index(line[off:], name)
		if i < 0 {
			return false
		}
		i += off
		end := i + len(name)
		if (i == 0 || !isIdent(line[i-1])) && (end == len(line) || !isIdent(line[end])) {
			return true
		}
		off = i + 1
	}
}

func isIdent(c byte) bool {
	return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// testNames lists the forms a test's name may take in its file, longest
// first: the last part of the ID without parameters, Go subtests or a
// vitest path, then with leading words dropped (ExUnit and busted prefix
// the describe names).
func testNames(id string) []string {
	name := id[strings.LastIndex(id, "::")+len("::"):]
	if i := strings.LastIndex(name, " > "); i >= 0 {
		name = name[i+len(" > "):]
	}
	if i := strings.IndexAny(name, "[(/"); i > 0 {
		name = name[:i]
	}
	name = strings.TrimSpace(name)
	out := []string{name}
	words := strings.Fields(name)
	for i := 1; i < len(words)-1; i++ {
		out = append(out, strings.Join(words[i:], " "))
	}
	return out
}

// testBlock returns the first and last line of the test declared on line
// i: preceding annotations or decorators, then everything indented deeper,
// a brace on its own line and the closing line.
func testBlock(lines []string, i int) (from, to int) {
	ind := indent(lines[i])
	from = i
	for {
		j := from - 1
		for j >= 0 && strings.TrimSpace(lines[j]) != "" && indent(lines[j]) > ind {
			j--
		}
		if j < 0 || indent(lines[j]) != ind || !isAnnotation(lines[j]) {
			break
		}
		from = j
	}
	to = i
	for k := i + 1; k < len(lines); k++ {
		t := strings.TrimSpace(lines[k])
		switch {
		case t == "":
			continue
		case indent(lines[k]) > ind || t == "{":
			to = k
			continue
		case isCloser(t):
			to = k
		}
		break
	}
	return from, to
}

func indent(l string) int { return len(l) - len(strings.TrimLeft(l, " \t")) }

func isAnnotation(l string) bool {
	t := strings.TrimSpace(l)
	return strings.HasPrefix(t, "@") || strings.HasPrefix(t, "[")
}

func isCloser(t string) bool {
	return strings.HasPrefix(t, "}") || strings.HasPrefix(t, ")") || strings.HasPrefix(t, "]") ||
		t == "end" || strings.HasPrefix(t, "end ") || strings.HasPrefix(t, "end)")
}

var hunkHeader = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)`)

// addedLineNumbers returns the new-file line numbers (1-based) of a
// unified diff's added lines.
func addedLineNumbers(patch string) map[int]bool {
	out := map[int]bool{}
	n := 0
	for _, l := range strings.Split(patch, "\n") {
		switch {
		case strings.HasPrefix(l, "@@"):
			if m := hunkHeader.FindStringSubmatch(l); m != nil {
				n, _ = strconv.Atoi(m[1])
			}
		case strings.HasPrefix(l, "+++"), strings.HasPrefix(l, "---"):
		case strings.HasPrefix(l, "+"):
			out[n] = true
			n++
		case strings.HasPrefix(l, " "), l == "":
			n++
		}
	}
	return out
}

// movedOut returns the added test lines that are not just a removed line
// re-indented, and counts them like testLines.
func movedOut(diffs []snapshot.FileDiff) (string, int) {
	var kept []string
	for _, d := range diffs {
		if d.Kind != config.Test {
			continue
		}
		removed := map[string]int{}
		for _, l := range strings.Split(d.Patch, "\n") {
			if strings.HasPrefix(l, "-") && !strings.HasPrefix(l, "---") {
				removed[strings.TrimSpace(l[1:])]++
			}
		}
		for _, l := range strings.Split(d.Patch, "\n") {
			if !strings.HasPrefix(l, "+") || strings.HasPrefix(l, "+++") {
				continue
			}
			if t := strings.TrimSpace(l[1:]); t != "" && removed[t] > 0 {
				removed[t]--
				continue
			}
			kept = append(kept, l)
		}
	}
	patch := strings.Join(kept, "\n")
	return addedLines(patch), testLines(patch)
}
