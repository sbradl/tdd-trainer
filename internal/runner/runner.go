// Package runner runs a project's build and test commands and turns the
// result into a Test state.
package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/sbradl/tdd-trainer/internal/config"
	"github.com/sbradl/tdd-trainer/internal/results"
)

// Outcome is the result of one build + test run.
type Outcome struct {
	State results.TestState
	// Output is what the build or test command printed (stdout + stderr);
	// judge evidence for the runner output.
	Output string
	// ExitCodeOnly is set when the state comes from the exit code alone
	// (no results format, or a run that passed without writing results);
	// verdicts will be weaker.
	ExitCodeOnly bool
}

// ExitCodeOnlyID names the single failing "test" of an exit-code-only run.
const ExitCodeOnlyID = "(test run)"

// Run runs cfg's build command (if any), then its test command, in dir.
// A failing build command means build broken. Results are read from
// cfg.Results.Path if set, else from stdout. Cancelling ctx kills the
// command in flight.
func Run(ctx context.Context, cfg config.Config, dir string) (Outcome, error) {
	if cfg.Build != "" {
		out, err := shell(ctx, dir, cfg.Build, nil)
		if err := exitOK(ctx, err); err != nil {
			return Outcome{}, fmt.Errorf("build: %w", err)
		}
		if err != nil {
			return Outcome{State: results.TestState{BuildBroken: true}, Output: out}, nil
		}
	}

	var resultFile string
	if cfg.Results.Path != "" {
		resultFile = filepath.Join(dir, cfg.Results.Path)
		// A stale file from an earlier run must not be mistaken for this one.
		if err := os.Remove(resultFile); err != nil && !os.IsNotExist(err) {
			return Outcome{}, err
		}
	}

	var stdout bytes.Buffer
	out, runErr := shell(ctx, dir, cfg.TestCmd(runtime.GOOS), &stdout)
	if err := exitOK(ctx, runErr); err != nil {
		return Outcome{}, fmt.Errorf("test: %w", err)
	}
	o := Outcome{Output: out}

	var data []byte
	switch {
	case cfg.Results.Format == "":
	case resultFile != "":
		var err error
		if data, err = os.ReadFile(resultFile); err != nil && !os.IsNotExist(err) {
			return Outcome{}, err
		}
	default:
		data = stdout.Bytes()
	}
	switch {
	case len(data) == 0 && runErr != nil && cfg.Results.Format != "":
		// Results are configured but the runner wrote none: the tests
		// could not run, e.g. a compile error in the test files.
		o.State.BuildBroken = true
		return o, nil
	case len(data) == 0:
		o.ExitCodeOnly = true
		if runErr != nil {
			o.State.Failing = []results.Failure{{ID: ExitCodeOnlyID, Message: out}}
		}
		return o, nil
	}
	rep, err := results.Read(cfg.Results.Format, bytes.NewReader(data))
	if err != nil {
		return Outcome{}, fmt.Errorf("reading %s results: %w", cfg.Results.Format, err)
	}
	o.State = rep.State()
	return o, nil
}

// exitOK returns nil for success or a non-zero exit, which is expected
// when tests fail, and an error when the command could not run at all or
// ctx was cancelled.
func exitOK(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var exitErr *exec.ExitError
	if err == nil || errors.As(err, &exitErr) {
		return nil
	}
	return err
}

// shell runs cmdline through the platform shell and returns stdout and
// stderr interleaved; stdout is also copied to extra if non-nil.
func shell(ctx context.Context, dir, cmdline string, extra *bytes.Buffer) (string, error) {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/C", cmdline)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", cmdline)
	}
	cmd.Dir = dir
	var combined bytes.Buffer
	cmd.Stderr = &combined
	cmd.Stdout = &combined
	if extra != nil {
		cmd.Stdout = io.MultiWriter(&combined, extra)
	}
	err := cmd.Run()
	return combined.String(), err
}
