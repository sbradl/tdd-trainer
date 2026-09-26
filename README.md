# tddt — a TDD coach for the terminal

`tddt` watches you practise test-driven development. On every save it runs your tests, works out whether you just finished a **Red**, **Green** or **Refactor** step, and gives advisory feedback:

- Is the new test small, and does it check one behaviour?
- Does it fail for the right reason (on its assertion, not a compile error)?
- Was the Green change the simplest one, following the Transformation Priority Premise?
- Did the refactoring keep behaviour and improve the code? Did you skip refactoring?

It never blocks or reverts anything. A small language model runs **locally** as the judge; your code never leaves your machine. At the end of a session you get a Markdown report.

Linux and Windows (amd64). Presets for Go, Python (pytest), TypeScript (jest, vitest), .NET (xUnit, NUnit, MSTest), Lua (busted) and Elixir (ExUnit); anything else works with a hand-written config.

## Install

**Release archive** (recommended): download `tddt-…-linux-amd64.tar.gz` or `tddt-…-windows-amd64.zip` from the releases page and unpack it anywhere. It contains `tddt` and the llama.cpp libraries it needs.

**With Go**: `go install github.com/sbradl/tdd-trainer/cmd/tddt@latest`; `tddt setup` then downloads the libraries too.

Then, once:

```sh
tddt setup
```

This downloads the judge model (Qwen3.5-4B, 3 GB, Apache-2.0) into your user cache folder, checks its SHA-256 and runs a self-check. It uses your GPU through Vulkan when there is one (about 1 s per verdict), otherwise the CPU (about 3–8 s).

Offline or behind a proxy: download the model file yourself and run `tddt setup --model-file path/to/Qwen_Qwen3.5-4B-Q4_K_M.gguf`, or point `TDDT_MODEL` at it. `TDDT_LIB` points at a folder with the llama.cpp libraries.

### Windows: SmartScreen

The binaries are not code-signed, so Windows SmartScreen may say it "protected your PC" the first time you run `tddt.exe`. Choose **More info → Run anyway**, or check the file against `SHA256SUMS` from the release first. The archive includes the Microsoft VC++ runtime DLLs next to `tddt.exe`; the Vulkan driver comes with your GPU driver.

## Use

In your project folder:

```sh
tddt
```

The first time, `tddt` detects your project type and proposes a `.tddtrainer.yml` (you can also run `tddt init`). Then write a failing test, make it pass, refactor — and watch the coach.

| Key | |
|---|---|
| Tab | switch between the quiet coach and the dashboard |
| r / g / f | tell the coach you are writing a test / making it pass / refactoring |
| b | reset the baseline to the current state |
| w | write the session report now |
| q, Ctrl-C | quit (writes the report) |

The **quiet coach** shows one status line and only the hints and warnings; each finished cycle folds into one summary line. The **dashboard** shows the cycle strip and every verdict of the current step, including pending ones.

Other commands:

```sh
tddt show 4          # step 4 of the last session: verdicts and full diff
tddt --once          # run the tests once and print the test state
tddt --no-judge      # exact checks only, no model
tddt judge --regress # check the judge against its labelled fixtures
```

Reports go to `.tddtrainer/reports/`. Everything the coach stores is under `.tddtrainer/`, which ignores itself in git; your own repository is never touched.

## Configuration

`.tddtrainer.yml` in the project root:

```yaml
preset: go
build: go vet ./...              # optional; a failure means "build broken"
test:
  cmd: go test -json ./...
  windows: go test -json ./...   # optional override for Windows
env: {}                          # extra environment for build and test
results: { format: go-json }     # junit-xml | go-json | trx, plus path: for files
tests:   ["**/*_test.go"]
sources: ["**/*.go"]
ignore:  [".tddtrainer/**", "vendor/**"]
tpp_order: iteration-first       # or recursion-first
slow_run_warning: 5s
```

Without `results`, only the exit code is used and the verdicts are weaker.

## Licence

MIT, see [LICENSE](LICENSE). The release archives also contain llama.cpp (MIT) and, on Windows, the Microsoft VC++ runtime DLLs; the judge model is Apache-2.0.
