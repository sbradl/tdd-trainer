// Command tddt is a terminal coach for practising test-driven development.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/sbradl/tdd-trainer/internal/config"
	"github.com/sbradl/tdd-trainer/internal/runner"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "tddt:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	dir := "."
	switch len(args) {
	case 0:
	case 1:
		dir = args[0]
	default:
		return fmt.Errorf("usage: tddt [dir]")
	}

	cfg, err := config.Load(dir)
	if os.IsNotExist(err) {
		return fmt.Errorf("no %s in %s", config.FileName, dir)
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
	fmt.Println(o.State)
	return nil
}
