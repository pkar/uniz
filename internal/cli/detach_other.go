//go:build !(darwin || linux || freebsd || netbsd || openbsd || dragonfly)

package cli

import "os/exec"

func detach(*exec.Cmd) {}
