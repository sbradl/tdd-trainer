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
	"strings"
	"sync"

	"github.com/sbradl/tdd-trainer/internal/coach"
	"github.com/sbradl/tdd-trainer/internal/config"
	"github.com/sbradl/tdd-trainer/internal/judge"
	"github.com/sbradl/tdd-trainer/internal/runner"
	"github.com/sbradl/tdd-trainer/internal/session"
	"github.com/sbradl/tdd-trainer/internal/snapshot"
	"github.com/sbradl/tdd-trainer/internal/steps"
	"github.com/sbradl/tdd-trainer/internal/watch"
)

const usage = `usage:
  tddt [--once] [--no-judge] [--cpu] [dir] watch the project in dir (default .)
  tddt init [--preset NAME] [--yes] [dir]  write .tddtrainer.yml
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

// watchLoop prints plain event lines; the TUI replaces it later.
func watchLoop(ctx context.Context, cfg config.Config, dir string, out io.Writer, jd *judge.Lazy) error {
	w, err := watch.New(dir, cfg, watch.DefaultDebounce)
	if err != nil {
		return err
	}
	batches, _ := w.Run(ctx)
	store, err := snapshot.Open(dir, cfg)
	if err != nil {
		return err
	}
	var mu sync.Mutex // serialises output from the loop and the judge worker
	printf := func(format string, a ...any) {
		mu.Lock()
		defer mu.Unlock()
		fmt.Fprintf(out, format, a...)
	}
	var scorer judge.Scorer
	if jd != nil {
		scorer = jd
		defer jd.Close()
		go func() {
			if e, err := jd.Wait(ctx); err != nil && ctx.Err() == nil {
				printf("judge unavailable: %v\n", err)
			} else if e != nil {
				printf("judge ready (%s)\n", map[bool]string{true: "GPU", false: "CPU"}[e.GPU])
			}
		}()
	}
	okCount := 0
	c := coach.New(store, scorer, cfg.TPPOrder, func(v coach.Verdict) {
		switch v.Level {
		case coach.OK:
			mu.Lock()
			okCount++
			mu.Unlock()
		case coach.Uncertain:
		default:
			printf("  %s step %d (%s) %s: %s\n", strings.ToUpper(v.Level.String()), v.Step, v.Kind, v.Check, v.Text)
		}
	})
	go c.Run(ctx)
	machine := steps.New()
	var prev snapshot.ID
	warnedExitCode := false
	fmt.Fprintln(out, "Watching; Ctrl-C to quit.")
	session.Loop(ctx, batches, func(ctx context.Context) (session.Result, error) {
		id, err := store.Snapshot("test run")
		if err != nil {
			return session.Result{}, fmt.Errorf("snapshot: %w", err)
		}
		o, err := runner.Run(ctx, cfg, dir)
		return session.Result{Outcome: o, Snapshot: id}, err
	}, cfg.SlowRunWarning, func(e session.Event) {
		switch e := e.(type) {
		case session.RunStarted:
			if e.Changed != nil {
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
			printf("%s (%.1fs)\n", e.Outcome.State, e.Duration.Seconds())
			obs := steps.Observation{Snapshot: e.Snapshot, State: e.Outcome.State}
			if prev != "" {
				diff, err := store.Diff(prev, e.Snapshot)
				if err != nil {
					printf("error: %v\n", err)
				}
				for _, d := range diff {
					obs.TestsChanged = obs.TestsChanged || d.Kind == config.Test
					obs.SourceChanged = obs.SourceChanged || d.Kind == config.Source
				}
			}
			prev = e.Snapshot
			for _, ev := range machine.Observe(obs) {
				mu.Lock()
				printStepEvent(out, ev)
				mu.Unlock()
				if sd, ok := ev.(steps.StepDone); ok {
					if err := c.Step(sd.Step); err != nil {
						printf("error: %v\n", err)
					}
				}
			}
		}
	})
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
		fmt.Fprintln(out, "» Red in progress: the new test does not fail on an assertion yet")
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
			line += " [no refactor after the last Green]"
		}
		fmt.Fprintln(out, line)
	}
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
