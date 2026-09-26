package runner

import (
	"os/exec"
	"strconv"
)

// killTree makes cancelling cmd kill its whole process tree, so test
// runners started by cmd.exe die with it.
func killTree(cmd *exec.Cmd) {
	cmd.Cancel = func() error {
		return exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
	}
}
