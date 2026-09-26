// Package runner runs a project's build and test commands and turns the
// result into a Test state.
package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"

	"github.com/sbradl/tdd-trainer/internal/config"
	"github.com/sbradl/tdd-trainer/internal/results"
)

// Run runs cfg's build command (if any), then its test command, in dir.
// A failing build command means build broken. Cancelling ctx kills the
// command in flight.
func Run(ctx context.Context, cfg config.Config, dir string) (results.TestState, error) {
	if cfg.Results.Format != "go-json" {
		return results.TestState{}, fmt.Errorf("results format %q not supported yet", cfg.Results.Format)
	}

	if cfg.Build != "" {
		if _, err := shell(ctx, dir, cfg.Build); err != nil {
			if ctx.Err() != nil {
				return results.TestState{}, ctx.Err()
			}
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				return results.TestState{BuildBroken: true}, nil
			}
			return results.TestState{}, fmt.Errorf("build: %w", err)
		}
	}

	out, err := shell(ctx, dir, cfg.TestCmd(runtime.GOOS))
	if ctx.Err() != nil {
		return results.TestState{}, ctx.Err()
	}
	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		return results.TestState{}, fmt.Errorf("test: %w", err)
	}
	// A non-zero exit is expected when tests fail; the output decides.
	return results.ReadGoJSON(bytes.NewReader(out))
}

func shell(ctx context.Context, dir, cmdline string) ([]byte, error) {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/C", cmdline)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", cmdline)
	}
	cmd.Dir = dir
	return cmd.Output()
}
