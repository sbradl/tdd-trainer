package preset

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sbradl/tdd-trainer/internal/config"
)

func TestEveryPresetRendersAValidConfig(t *testing.T) {
	for _, p := range append(All(), Blank) {
		t.Run(p.Name, func(t *testing.T) {
			out, err := p.Render()
			if err != nil {
				t.Fatal(err)
			}
			c, err := config.Parse(out)
			if err != nil {
				t.Fatalf("%v\n%s", err, out)
			}
			if p.Name != Blank.Name && c.Preset != p.Name {
				t.Fatalf("preset field %q, want %q", c.Preset, p.Name)
			}
		})
	}
}

func TestElixirIsRecursionFirst(t *testing.T) {
	p, _ := ByName("elixir")
	out, _ := p.Render()
	c, _ := config.Parse(out)
	if c.TPPOrder != "recursion-first" {
		t.Fatalf("tpp_order %q", c.TPPOrder)
	}
}

func project(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, body := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(path), 0o755)
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func detected(t *testing.T, files map[string]string) []string {
	t.Helper()
	ms, err := Detect(project(t, files))
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, m := range ms {
		out = append(out, filepath.ToSlash(m.Dir)+":"+m.Preset.Name)
	}
	return out
}

func TestDetect(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{"go", map[string]string{"go.mod": "module x"}, ".:go"},
		{"python pyproject", map[string]string{"pyproject.toml": ""}, ".:python"},
		{"python pytest.ini", map[string]string{"pytest.ini": ""}, ".:python"},
		{"vitest", map[string]string{"package.json": `{"devDependencies":{"vitest":"^3"}}`}, ".:typescript-vitest"},
		{"jest", map[string]string{"package.json": `{"devDependencies":{"jest":"^30"}}`}, ".:typescript-jest"},
		{"js without runner offers both", map[string]string{"package.json": `{}`}, ".:typescript-vitest,.:typescript-jest"},
		{"nunit via csproj", map[string]string{"K.Tests/K.Tests.csproj": `<PackageReference Include="NUnit" Version="4" />`}, "K.Tests:dotnet-nunit"},
		{"sln covers projects", map[string]string{
			"K.sln":                  "",
			"K/K.csproj":             "",
			"K.Tests/K.Tests.csproj": `<PackageReference Include="MSTest.TestFramework" Version="3" />`,
		}, ".:dotnet-mstest"},
		{"dotnet defaults to xunit", map[string]string{"K.csproj": ""}, ".:dotnet-xunit"},
		{"elixir", map[string]string{"mix.exs": "def project do [app: :kata, version: \"0.1.0\"] end"}, ".:elixir"},
		{"lua .busted", map[string]string{".busted": ""}, ".:lua"},
		{"lua rockspec", map[string]string{"kata-1.0-1.rockspec": ""}, ".:lua"},
		{"none", map[string]string{"README.md": ""}, ""},
		{"monorepo", map[string]string{
			"api/go.mod":       "module api",
			"web/package.json": `{"devDependencies":{"vitest":"1"}}`,
		}, "api:go,web:typescript-vitest"},
		{"skips dependency folders", map[string]string{
			"go.mod":                        "module x",
			"node_modules/foo/package.json": "{}",
			".git/x/go.mod":                 "",
		}, ".:go"},
		{"max depth", map[string]string{"a/b/c/d/go.mod": ""}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := strings.Join(detected(t, c.files), ","); got != c.want {
				t.Fatalf("got %q want %q", got, c.want)
			}
		})
	}
}

func TestDetectFillsTemplateVars(t *testing.T) {
	root := project(t, map[string]string{
		"mix.exs":       "def project do\n  [app: :bowling, version: \"0.1.0\"]\nend",
		"package.json":  `{"devDependencies":{"jest":"1","jest-junit":"1"}}`,
		"tsconfig.json": "{}",
	})
	ms, _ := Detect(root)
	for _, m := range ms {
		out, _ := m.Preset.Render()
		switch m.Preset.Name {
		case "elixir":
			if !strings.Contains(string(out), "_build/test/lib/bowling/") {
				t.Errorf("elixir app not filled:\n%s", out)
			}
		case "typescript-jest":
			if !strings.Contains(string(out), "build: npx tsc --noEmit") {
				t.Errorf("tsc build missing:\n%s", out)
			}
			if len(m.Preset.Setup) != 0 {
				t.Errorf("jest-junit present, want no setup step, got %v", m.Preset.Setup)
			}
		}
	}
}

func TestElixirWithJUnitFormatterNeedsNoSetup(t *testing.T) {
	ms, _ := Detect(project(t, map[string]string{"mix.exs": `[app: :k, deps: [{:junit_formatter, "~> 3.4"}]]`}))
	if len(ms[0].Preset.Setup) != 0 {
		t.Fatalf("setup %v", ms[0].Preset.Setup)
	}
}

func TestNoTSConfigNoBuild(t *testing.T) {
	ms, _ := Detect(project(t, map[string]string{"package.json": `{"devDependencies":{"vitest":"1"}}`}))
	out, _ := ms[0].Preset.Render()
	if strings.Contains(string(out), "build:") {
		t.Fatalf("unexpected build step:\n%s", out)
	}
}
