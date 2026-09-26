//go:build !windows

package runner

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/sbradl/tdd-trainer/internal/config"
)

func TestRunCancelKillsProcessTree(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := Run(ctx, config.Config{Test: config.TestCmd{Cmd: "sh -c 'sleep 30 & echo $! > pid; wait'"}}, dir)
		done <- err
	}()
	pidFile := filepath.Join(dir, "pid")
	var pid int
	for deadline := time.Now().Add(5 * time.Second); pid == 0; time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("grandchild did not start")
		}
		if data, err := os.ReadFile(pidFile); err == nil && len(data) > 1 {
			pid, _ = strconv.Atoi(strings.TrimSpace(string(data)))
		}
	}
	start := time.Now()
	cancel()
	if err := <-done; err == nil {
		t.Fatal("want cancel error")
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Fatalf("cancel took %v", d)
	}
	time.Sleep(50 * time.Millisecond)
	if syscall.Kill(pid, 0) == nil {
		syscall.Kill(pid, syscall.SIGKILL)
		t.Fatal("grandchild survived cancel")
	}
}
