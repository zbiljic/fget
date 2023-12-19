//go:build unix

package gitexec

import (
	"os/exec"
	"syscall"
)

func withSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}
}
