//go:build unix

package main

import (
	"os/exec"
	"syscall"
)

// confine puts the child in its own process group and, on a timeout, kills the whole
// group, so a hung jq or perl started by sitrep does not outlive the call.
func confine(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
}
