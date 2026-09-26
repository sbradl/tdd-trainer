//go:build !windows

package runner

import (
	"os/exec"
	"syscall"
)

// killTree makes cancelling cmd kill its whole process group, so test
// runners started by the shell die with it.
func killTree(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
