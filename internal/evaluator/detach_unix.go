//go:build !windows

package evaluator

import (
	"os/exec"
	"syscall"
)

// detach puts the child in its own session so it survives the hook process
// and its process group.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
