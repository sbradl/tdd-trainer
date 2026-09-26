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

	"github.com/sbradl/tdd-trainer/internal/config"
	"github.com/sbradl/tdd-trainer/internal/runner"
	"github.com/sbradl/tdd-trainer/internal/session"
	"github.com/sbradl/tdd-trainer/internal/snapshot"
	"github.com/sbradl/tdd-trainer/internal/watch"
)

const usage = `usage:
  tddt [--once] [dir]                      watch the project in dir (default .)
  tddt init [--preset NAME] [--yes] [dir]  write .tddtrainer.yml
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
	fs := flag.NewFlagSet("tddt", flag.ContinueOnError)
	fs.Usage = func() { fmt.Fprint(fs.Output(), usage) }
	once := fs.Bool("once", false, "run the tests once, print the Test state and exit")
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
	return watchLoop(ctx, cfg, dir, out)
}

const exitCodeOnlyWarning = "warning: no test results read, using the exit code only; verdicts will be weaker"

// watchLoop prints plain event lines; the TUI replaces it later.
func watchLoop(ctx context.Context, cfg config.Config, dir string, out io.Writer) error {
	w, err := watch.New(dir, cfg, watch.DefaultDebounce)
	if err != nil {
		return err
	}
	batches, _ := w.Run(ctx)
	warnedExitCode := false
	fmt.Fprintln(out, "Watching; Ctrl-C to quit.")
	store, err := snapshot.Open(dir, cfg)
	if err != nil {
		return err
	}
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
				fmt.Fprintf(out, "changed: %s\n", strings.Join(e.Changed, ", "))
			}
		case session.SlowRun:
			fmt.Fprintf(out, "warning: test run is taking longer than %v\n", e.Limit)
		case session.RunFailed:
			fmt.Fprintln(out, "error:", e.Err)
		case session.RunDone:
			if e.Outcome.ExitCodeOnly && !warnedExitCode {
				warnedExitCode = true
				fmt.Fprintln(out, exitCodeOnlyWarning)
			}
			fmt.Fprintf(out, "%s (%.1fs)\n", e.Outcome.State, e.Duration.Seconds())
		}
	})
	return nil
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
