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

	"github.com/sbradl/tdd-trainer/internal/config"
	"github.com/sbradl/tdd-trainer/internal/runner"
)

const usage = `usage:
  tddt [dir]                               watch the project in dir (default .)
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
	o, err := runner.Run(ctx, cfg, dir)
	if err != nil {
		return err
	}
	if o.ExitCodeOnly {
		fmt.Fprintln(os.Stderr, "tddt: warning: no test results read, using the exit code only; verdicts will be weaker")
	}
	fmt.Fprintln(out, o.State)
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
