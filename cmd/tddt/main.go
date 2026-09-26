// Command tddt is a terminal coach for practising test-driven development.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"

	"github.com/sbradl/tdd-trainer/internal/app"
	"github.com/sbradl/tdd-trainer/internal/coach"
	"github.com/sbradl/tdd-trainer/internal/config"
	"github.com/sbradl/tdd-trainer/internal/judge"
	"github.com/sbradl/tdd-trainer/internal/report"
	"github.com/sbradl/tdd-trainer/internal/runner"
	"github.com/sbradl/tdd-trainer/internal/session"
	"github.com/sbradl/tdd-trainer/internal/setup"
	"github.com/sbradl/tdd-trainer/internal/snapshot"
	"github.com/sbradl/tdd-trainer/internal/steps"
	"github.com/sbradl/tdd-trainer/internal/tui"
)

const usage = `usage:
  tddt [--once] [--no-judge] [--cpu] [dir] watch the project in dir (default .)
  tddt init [--preset NAME] [--yes] [dir]  write .tddtrainer.yml
  tddt setup [--model-file F] [--lib-dir D] download the judge's libraries and model
  tddt show STEP [dir]                     print a step of the last session with its full diff
  tddt judge --regress [--cpu] [--gate G]  check the judge against its fixtures
`

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		if !errors.Is(err, flag.ErrHelp) {
			fmt.Fprintln(os.Stderr, "tddt:", err)
		}
		os.Exit(1)
	}
}

func run(args []string, in io.Reader, out io.Writer) error {
	if len(args) > 0 && args[0] == "init" {
		return cmdInit(args[1:], in, out)
	}
	if len(args) > 0 && args[0] == "setup" {
		return cmdSetup(args[1:], out)
	}
	if len(args) > 0 && args[0] == "show" {
		return cmdShow(args[1:], out)
	}
	if len(args) > 0 && args[0] == "judge" {
		return cmdJudge(args[1:], out)
	}
	fs := flag.NewFlagSet("tddt", flag.ContinueOnError)
	fs.Usage = func() { fmt.Fprint(fs.Output(), usage) }
	once := fs.Bool("once", false, "run the tests once, print the Test state and exit")
	noJudge := fs.Bool("no-judge", false, "exact checks only; do not load the model")
	cpu := fs.Bool("cpu", false, "do not use the GPU for the judge")
	if err := fs.Parse(args); err != nil {
		return err
	}
	dir, err := dirArg(fs)
	if err != nil {
		return err
	}

	cfg, err := config.Load(dir)
	if os.IsNotExist(err) {
		fmt.Fprintf(out, "No %s here yet; let's create one.\n", config.FileName)
		res, initErr := runInit(in, out, initOpts{root: dir})
		if initErr != nil || !res.ready {
			return initErr
		}
		dir = res.dir
		cfg, err = config.Load(dir)
	}
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if *once {
		o, err := runner.Run(ctx, cfg, dir)
		if err != nil {
			return err
		}
		if o.ExitCodeOnly {
			fmt.Fprintln(out, exitCodeOnlyWarning)
		}
		fmt.Fprintln(out, o.State)
		return nil
	}
	return watchLoop(ctx, cfg, dir, out, judgeScorer(out, *noJudge, *cpu))
}

// judgeScorer starts loading the judge when its files are installed.
func judgeScorer(out io.Writer, disabled, cpu bool) *judge.Lazy {
	if disabled {
		return nil
	}
	lib, model := judge.DefaultLibDir(), judge.DefaultModel()
	for _, p := range []string{lib, model} {
		if _, err := os.Stat(p); err != nil {
			fmt.Fprintf(out, "Judge not installed (%s missing): exact checks only. Run tddt setup.\n", p)
			return nil
		}
	}
	return judge.OpenLazy(judge.EngineOptions{LibDir: lib, Model: model, CPU: cpu})
}

const exitCodeOnlyWarning = "warning: no test results read, using the exit code only; verdicts will be weaker"

// watchLoop runs the session with the TUI, or with plain lines when
// stdout is not a terminal.
func watchLoop(ctx context.Context, cfg config.Config, dir string, out io.Writer, jd *judge.Lazy) error {
	if jd != nil {
		defer jd.Close()
	}
	status := "exact checks only"
	if jd != nil {
		status = "judge loading…"
	}
	if f, ok := out.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		var p *tea.Program
		a, err := app.New(dir, cfg, jd, func(m any) {
			if p != nil {
				p.Send(m)
			}
		})
		if err != nil {
			return err
		}
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		p = tea.NewProgram(tui.New(controller{a, dir}, status), tea.WithAltScreen(), tea.WithContext(ctx))
		go func() {
			a.Run(ctx)
			p.Quit()
		}()
		_, err = p.Run()
		if errors.Is(err, tea.ErrProgramKilled) || errors.Is(err, context.Canceled) {
			err = nil
		}
		return errors.Join(err, finish(a, dir, out))
	}

	var mu sync.Mutex
	printf := func(format string, a ...any) {
		mu.Lock()
		defer mu.Unlock()
		fmt.Fprintf(out, format, a...)
	}
	warnedExitCode := false
	a, err := app.New(dir, cfg, jd, func(m any) {
		switch e := m.(type) {
		case session.RunStarted:
			if len(e.Changed) > 0 {
				printf("changed: %s\n", strings.Join(e.Changed, ", "))
			}
		case session.SlowRun:
			printf("warning: test run is taking longer than %v\n", e.Limit)
		case session.RunFailed:
			printf("error: %v\n", e.Err)
		case session.RunDone:
			if e.Outcome.ExitCodeOnly && !warnedExitCode {
				warnedExitCode = true
				printf("%s\n", exitCodeOnlyWarning)
			}
			printf("tests (%.1fs): %s\n", e.Duration.Seconds(), e.Outcome.State)
		case steps.Event:
			mu.Lock()
			printStepEvent(out, e)
			mu.Unlock()
		case coach.Verdict:
			if e.Level == coach.OK && strings.HasPrefix(e.Text, "Resolved") {
				printf("  RESOLVED step %d (%s) %s: %s\n", e.Step, e.Kind, coach.Label(e.Check), e.Text)
			}
			if e.Level == coach.Hint || e.Level == coach.Warn {
				printf("  %s step %d (%s) %s: %s\n", strings.ToUpper(e.Level.String()), e.Step, e.Kind, coach.Label(e.Check), e.Text)
			}
		case app.JudgeMsg:
			if e.Err != nil {
				printf("judge unavailable: %v\n", e.Err)
			} else {
				printf("judge ready (%s)\n", map[bool]string{true: "GPU", false: "CPU"}[e.GPU])
			}
		case app.ErrorMsg:
			printf("error: %v\n", e.Err)
		}
	})
	if err != nil {
		return err
	}
	printf("Watching (%s); Ctrl-C to quit.\n", status)
	err = a.Run(ctx)
	return errors.Join(err, finish(a, dir, out))
}

// finish writes the session report and prints the summary.
func finish(a *app.App, dir string, out io.Writer) error {
	h := a.History()
	if len(h.Steps) == 0 {
		return nil
	}
	pending := len(a.PendingGates())
	path, err := report.Write(dir, h, pending, a.Store())
	if err != nil {
		return fmt.Errorf("writing report: %w", err)
	}
	fmt.Fprint(out, report.Summary(h, pending, path))
	return nil
}

func printStepEvent(out io.Writer, ev steps.Event) {
	switch ev := ev.(type) {
	case steps.Baseline:
		if ev.StartsRed {
			fmt.Fprintln(out, "» baseline set; warning: the session starts with failing tests (treated as Red in progress)")
		} else {
			fmt.Fprintln(out, "» baseline set")
		}
	case steps.RedInProgress:
		fmt.Fprintln(out, "» Red in progress: add the smallest stub so the new test compiles and fails on its assertion")
	case steps.PhaseChanged:
		fmt.Fprintln(out, "» phase set by hand:", ev.Phase)
	case steps.StepDone:
		s := ev.Step
		line := fmt.Sprintf("» step %d: %s", s.N, s.Kind)
		if len(s.NewTests) > 0 {
			line += " — " + strings.Join(s.NewTests, ", ")
		}
		for _, a := range s.Anomalies {
			line += " [" + a.String() + "]"
		}
		if s.AfterGreen {
			line += " (directly after a Green: checking for a missed refactor)"
		}
		fmt.Fprintln(out, line)
	}
}

// controller adapts the app to the TUI's keys.
type controller struct {
	*app.App
	dir string
}

func (c controller) WriteReport() (string, error) {
	return report.Write(c.dir, c.History(), len(c.PendingGates()), c.Store())
}

func cmdSetup(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("tddt setup", flag.ContinueOnError)
	fs.Usage = func() { fmt.Fprint(fs.Output(), usage) }
	modelFile := fs.String("model-file", "", "use this already downloaded model file (offline install)")
	libDir := fs.String("lib-dir", "", "install the libraries here (default: user cache)")
	skipModel := fs.Bool("skip-model", false, "only install the libraries")
	cpu := fs.Bool("cpu", false, "check with the CPU backend")
	if err := fs.Parse(args); err != nil || fs.NArg() > 0 {
		fs.Usage()
		return errors.Join(err, errors.New("unexpected arguments"))
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	lib := *libDir
	switch {
	case lib != "":
	case judge.HasLibs(judge.DefaultLibDir()):
		lib = judge.DefaultLibDir()
	default:
		lib = filepath.Join(judge.CacheDir(), "lib")
	}
	if judge.HasLibs(lib) {
		fmt.Fprintf(out, "llama.cpp libraries: %s\n", lib)
	} else {
		fmt.Fprintf(out, "Installing llama.cpp libraries into %s\n", lib)
		if err := setup.InstallLibs(ctx, runtime.GOOS, lib, out); err != nil {
			return err
		}
	}
	if *skipModel {
		return nil
	}

	model := judge.DefaultModel()
	switch {
	case *modelFile != "":
		if err := setup.Verify(*modelFile, setup.Model.SHA256, out); err != nil {
			return err
		}
		if err := placeModel(*modelFile, model); err != nil {
			return err
		}
	case fileSize(model) == setup.Model.Size:
		if err := setup.Verify(model, setup.Model.SHA256, out); err != nil {
			return fmt.Errorf("%w; delete it and run setup again", err)
		}
	default:
		fmt.Fprintf(out, "Downloading the judge model (%.1f GB) into %s\n", float64(setup.Model.Size)/1e9, model)
		if err := setup.Download(ctx, setup.Model, model, out); err != nil {
			return err
		}
	}
	fmt.Fprintf(out, "Model: %s\n", model)

	e, err := judge.OpenEngine(judge.EngineOptions{LibDir: lib, Model: model, CPU: *cpu})
	if err != nil {
		return err
	}
	defer e.Close()
	fx, err := judge.Fixtures()
	if err != nil {
		return err
	}
	for _, f := range fx {
		if f.Name != "red-check.no.cs-build" {
			continue
		}
		t0 := time.Now()
		vs, err := judge.Judge{Scorer: e}.Evaluate(ctx, f.Evidence, []string{f.Gate})
		if err != nil {
			return err
		}
		backend := "CPU"
		if e.GPU {
			backend = "GPU"
		}
		if vs[0].Answer != f.Expected {
			return fmt.Errorf("judge self-check failed: got %s (p=%.2f), want %s", vs[0].Answer, vs[0].P, f.Expected)
		}
		fmt.Fprintf(out, "Judge ready on %s (%.1fs per gate). Run tddt in your project.\n", backend, time.Since(t0).Seconds())
	}
	return nil
}

// placeModel links (or copies) an offline model file into place.
func placeModel(src, dest string) error {
	if abs, _ := filepath.Abs(src); abs == dest {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	os.Remove(dest)
	if err := os.Link(src, dest); err == nil {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	outF, err := os.Create(dest + ".part")
	if err != nil {
		return err
	}
	if _, err := io.Copy(outF, in); err != nil {
		outF.Close()
		return err
	}
	if err := outF.Close(); err != nil {
		return err
	}
	return os.Rename(dest+".part", dest)
}

func fileSize(p string) int64 {
	fi, err := os.Stat(p)
	if err != nil {
		return -1
	}
	return fi.Size()
}

func cmdShow(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("tddt show", flag.ContinueOnError)
	fs.Usage = func() { fmt.Fprint(fs.Output(), usage) }
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 || fs.NArg() > 2 {
		fs.Usage()
		return errors.New("which step?")
	}
	n, err := strconv.Atoi(fs.Arg(0))
	if err != nil {
		return fmt.Errorf("step must be a number: %q", fs.Arg(0))
	}
	dir := "."
	if fs.NArg() == 2 {
		dir = fs.Arg(1)
	}
	cfg, err := config.Load(dir)
	if err != nil {
		return err
	}
	h, _, err := app.LoadLatestSession(dir)
	if err != nil {
		return err
	}
	store, err := snapshot.Open(dir, cfg)
	if err != nil {
		return err
	}
	return report.Show(out, h, n, store)
}

func cmdInit(args []string, in io.Reader, out io.Writer) error {
	fs := flag.NewFlagSet("tddt init", flag.ContinueOnError)
	fs.Usage = func() { fmt.Fprint(fs.Output(), usage) }
	name := fs.String("preset", "", "preset to use instead of detecting one")
	yes := fs.Bool("yes", false, "accept the detected preset without asking")
	if err := fs.Parse(args); err != nil {
		return err
	}
	dir, err := dirArg(fs)
	if err != nil {
		return err
	}
	_, err = runInit(in, out, initOpts{root: dir, preset: *name, yes: *yes})
	return err
}

func cmdJudge(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("tddt judge", flag.ContinueOnError)
	fs.Usage = func() { fmt.Fprint(fs.Output(), usage) }
	regress := fs.Bool("regress", false, "judge every fixture and compare with its label")
	cpu := fs.Bool("cpu", false, "do not use the GPU")
	gates := fs.String("gate", "", "comma-separated gates to restrict to")
	lib := fs.String("lib", judge.DefaultLibDir(), "llama.cpp library folder")
	model := fs.String("model", judge.DefaultModel(), "model file (GGUF)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if !*regress || fs.NArg() > 0 {
		fs.Usage()
		return errors.New("nothing to do")
	}
	e, err := judge.OpenEngine(judge.EngineOptions{LibDir: *lib, Model: *model, CPU: *cpu})
	if err != nil {
		return err
	}
	defer e.Close()
	backend := "CPU"
	if e.GPU {
		backend = "GPU"
	}
	fmt.Fprintf(out, "# backend %s, model %s\n", backend, *model)
	var only []string
	if *gates != "" {
		only = strings.Split(*gates, ",")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	r, err := judge.Regress(ctx, judge.Judge{Scorer: e}, only, out)
	if err != nil {
		return err
	}
	if r.ConfidentWrong > 0 {
		return fmt.Errorf("%d confident and wrong verdicts", r.ConfidentWrong)
	}
	return nil
}

func dirArg(fs *flag.FlagSet) (string, error) {
	switch fs.NArg() {
	case 0:
		return ".", nil
	case 1:
		return fs.Arg(0), nil
	}
	fs.Usage()
	return "", errors.New("too many arguments")
}
