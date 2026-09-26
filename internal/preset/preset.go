// Package preset holds the built-in per-language configs and detects which
// one fits a project from its marker files.
package preset

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"text/template"
)

//go:embed presets/*.yml
var files embed.FS

// Preset is one built-in config.
type Preset struct {
	Name  string // as in the config's preset field
	Title string
	file  string
	vars  Vars
	// Setup lists steps the learner must do once before the config works.
	Setup []string
}

// Vars fill a preset's template; detection sets them from the project.
type Vars struct {
	TSConfig  bool   // tsconfig.json present: type-check as build step
	Framework string // .NET test framework: xunit | nunit | mstest
	App       string // Elixir OTP app name
}

const jestJUnitSetup = "Install the JUnit reporter: npm install --save-dev jest-junit"

var elixirSetup = []string{
	`Add {:junit_formatter, "~> 3.4", only: [:test]} to deps in mix.exs, then run mix deps.get`,
	"In test/test_helper.exs, before ExUnit.start(): ExUnit.configure(formatters: [JUnitFormatter, ExUnit.CLIFormatter])",
}

// All returns every preset with default template vars, for picking by hand.
func All() []Preset {
	return []Preset{
		{Name: "go", Title: "Go", file: "go"},
		{Name: "python", Title: "Python (pytest)", file: "python"},
		{Name: "typescript-jest", Title: "TypeScript (jest)", file: "typescript-jest", vars: Vars{TSConfig: true}, Setup: []string{jestJUnitSetup}},
		{Name: "typescript-vitest", Title: "TypeScript (vitest)", file: "typescript-vitest", vars: Vars{TSConfig: true}},
		{Name: "dotnet-xunit", Title: ".NET (xUnit)", file: "dotnet", vars: Vars{Framework: "xunit"}},
		{Name: "dotnet-nunit", Title: ".NET (NUnit)", file: "dotnet", vars: Vars{Framework: "nunit"}},
		{Name: "dotnet-mstest", Title: ".NET (MSTest)", file: "dotnet", vars: Vars{Framework: "mstest"}},
		{Name: "lua", Title: "Lua (busted)", file: "lua"},
		{Name: "elixir", Title: "Elixir (ExUnit)", file: "elixir", vars: Vars{App: "my_app"}, Setup: elixirSetup},
	}
}

// Blank is the config template for projects no preset fits.
var Blank = Preset{Name: "blank", Title: "Blank config (fill in yourself)", file: "blank"}

// ByName finds a preset in All.
func ByName(name string) (Preset, bool) {
	i := slices.IndexFunc(All(), func(p Preset) bool { return p.Name == name })
	if i < 0 {
		return Preset{}, false
	}
	return All()[i], true
}

// Render returns the preset's .tddtrainer.yml content.
func (p Preset) Render() ([]byte, error) {
	src, err := files.ReadFile("presets/" + p.file + ".yml")
	if err != nil {
		return nil, err
	}
	tmpl, err := template.New(p.file).Option("missingkey=error").Parse(string(src))
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	if err := tmpl.Execute(&b, p.vars); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// Match is a preset detected in a (sub)folder of the searched root.
type Match struct {
	Dir    string // relative to the root; "." for the root itself
	Preset Preset
}

func (m Match) String() string {
	if m.Dir == "." {
		return m.Preset.Title
	}
	return fmt.Sprintf("%s in %s", m.Preset.Title, filepath.ToSlash(m.Dir))
}

const maxDepth = 3

var skipDirs = map[string]bool{
	"node_modules": true, "vendor": true, "_build": true, "deps": true,
	"bin": true, "obj": true, "dist": true, "target": true, "venv": true,
	"lua_modules": true, "__pycache__": true, "coverage": true,
}

// Detect looks for marker files in root and its subfolders (up to three
// levels, skipping dependency and build folders).
func Detect(root string) ([]Match, error) {
	var matches []Match
	var dotnetDirs []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if rel != "." && (strings.HasPrefix(d.Name(), ".") || skipDirs[d.Name()] || strings.Count(rel, string(filepath.Separator)) >= maxDepth) {
			return filepath.SkipDir
		}
		// A .sln covers the projects below it.
		if slices.ContainsFunc(dotnetDirs, func(dd string) bool { return within(rel, dd) }) {
			return nil
		}
		found, isDotnet := detectDir(path)
		if isDotnet {
			dotnetDirs = append(dotnetDirs, rel)
		}
		for _, p := range found {
			matches = append(matches, Match{Dir: rel, Preset: p})
		}
		return nil
	})
	return matches, err
}

func within(rel, dir string) bool {
	return dir == "." || rel == dir || strings.HasPrefix(rel, dir+string(filepath.Separator))
}

func detectDir(dir string) (found []Preset, dotnetSln bool) {
	has := func(name string) bool {
		_, err := os.Stat(filepath.Join(dir, name))
		return err == nil
	}
	glob := func(pattern string) []string {
		m, _ := filepath.Glob(filepath.Join(dir, pattern))
		return m
	}
	get := func(name string, vars Vars) Preset {
		p, _ := ByName(name)
		p.vars = vars
		return p
	}

	if has("go.mod") {
		found = append(found, get("go", Vars{}))
	}
	if has("pyproject.toml") || has("pytest.ini") {
		found = append(found, get("python", Vars{}))
	}
	if has("package.json") {
		found = append(found, detectJS(dir, has("tsconfig.json"))...)
	}
	sln := glob("*.sln")
	if len(sln) > 0 || len(glob("*.slnx")) > 0 || len(glob("*.csproj")) > 0 {
		fw := dotnetFramework(dir)
		found = append(found, get("dotnet-"+fw, Vars{Framework: fw}))
	}
	if has("mix.exs") {
		mix, _ := os.ReadFile(filepath.Join(dir, "mix.exs"))
		p := get("elixir", Vars{App: elixirApp(mix)})
		if bytes.Contains(mix, []byte("junit_formatter")) {
			p.Setup = nil
		}
		found = append(found, p)
	}
	if has(".busted") || len(glob("*.rockspec")) > 0 {
		found = append(found, get("lua", Vars{}))
	}
	return found, len(sln) > 0 || len(glob("*.slnx")) > 0
}

// detectJS picks jest or vitest from package.json's dependencies; with
// neither, both are offered.
func detectJS(dir string, tsconfig bool) []Preset {
	var pkg struct {
		Deps    map[string]string `json:"dependencies"`
		DevDeps map[string]string `json:"devDependencies"`
	}
	data, _ := os.ReadFile(filepath.Join(dir, "package.json"))
	json.Unmarshal(data, &pkg)
	dep := func(n string) bool {
		_, a := pkg.Deps[n]
		_, b := pkg.DevDeps[n]
		return a || b
	}

	vars := Vars{TSConfig: tsconfig}
	var out []Preset
	for _, name := range []string{"vitest", "jest"} {
		if dep(name) {
			p, _ := ByName("typescript-" + name)
			p.vars = vars
			if name == "jest" && dep("jest-junit") {
				p.Setup = nil
			}
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		for _, name := range []string{"vitest", "jest"} {
			p, _ := ByName("typescript-" + name)
			p.vars = vars
			out = append(out, p)
		}
	}
	return out
}

var packageRef = regexp.MustCompile(`(?i)PackageReference\s+Include="([^"]+)"`)

// dotnetFramework reads the test framework from package references of the
// project files below dir; xunit if none is found.
func dotnetFramework(dir string) string {
	fw := "xunit"
	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && path != dir && (strings.HasPrefix(d.Name(), ".") || skipDirs[d.Name()]) {
			return filepath.SkipDir
		}
		if !strings.HasSuffix(path, ".csproj") && !strings.HasSuffix(path, ".fsproj") {
			return nil
		}
		data, _ := os.ReadFile(path)
		for _, m := range packageRef.FindAllStringSubmatch(string(data), -1) {
			ref := strings.ToLower(m[1])
			switch {
			case strings.HasPrefix(ref, "nunit"):
				fw = "nunit"
				return filepath.SkipAll
			case strings.HasPrefix(ref, "mstest"):
				fw = "mstest"
				return filepath.SkipAll
			case strings.HasPrefix(ref, "xunit"):
				fw = "xunit"
				return filepath.SkipAll
			}
		}
		return nil
	})
	return fw
}

var elixirAppRe = regexp.MustCompile(`app:\s*:(\w+)`)

func elixirApp(mixExs []byte) string {
	if m := elixirAppRe.FindSubmatch(mixExs); m != nil {
		return string(m[1])
	}
	return "my_app"
}
