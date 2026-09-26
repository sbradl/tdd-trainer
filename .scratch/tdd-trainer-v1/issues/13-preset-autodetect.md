# Preset auto-detection

Type: grilling
Status: resolved
Blocked by: none
Map: ../map.md

## Question

Does v1 detect the project's language and pick a preset automatically (go.mod, *.csproj, mix.exs, package.json with jest or vitest, pyproject/pytest, .busted/rockspec), or does 'tddt init' always ask? What happens with several matches (monorepo) or none?

## Answer

- **Auto-detection in v1**: `tddt init` detects marker files and suggests a preset:
  - `go.mod` → Go
  - `*.csproj`/`*.sln` → .NET; xUnit, NUnit or MSTest from the package references
  - `mix.exs` → Elixir
  - `package.json` → TypeScript; jest or vitest from its dependencies
  - `pyproject.toml`/`pytest.ini` → Python
  - `.busted`/`*.rockspec` → Lua

  After the learner confirms, `init` writes `.tddtrainer.yml`.
- **Several matches** (monorepo): list them and let the learner pick; the config is rooted at the chosen folder; one project per session. **No match**: offer all presets plus a blank config.
- **Missing config**: running `tddt` without a `.tddtrainer.yml` starts `init` automatically.
