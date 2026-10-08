//go:build darwin || linux || freebsd || netbsd || openbsd || dragonfly

package cli

import (
	"os/exec"
	"syscall"
)

// detach starts the command in a new session, away from the controlling
// terminal, so a terminal hangup does not reach it.
func detach(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
