//go:build !darwin && !linux

package sysinfo

import (
	"errors"
	"os"
)

var signalTable = []signalEntry{{"HUP", 1}, {"INT", 2}, {"QUIT", 3}, {"KILL", 9}, {"TERM", 15}}

// Kill supports only checking (signal 0) and terminating (KILL or TERM) a
// single process on this platform; both terminations kill outright.
func Kill(pid, sig int) error {
	if pid <= 0 {
		return errors.ErrUnsupported
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	defer p.Release()
	switch sig {
	case 0:
		return nil
	case 9, 15:
		return p.Kill()
	}
	return errors.ErrUnsupported
}

// Niceness is not supported on this platform.
func Niceness() (int, error) { return 0, errors.ErrUnsupported }

// SetNiceness is not supported on this platform.
func SetNiceness(int, int) error { return errors.ErrUnsupported }

// TerminalName is not supported on this platform.
func TerminalName(*os.File) (string, error) { return "", errors.ErrUnsupported }

// DiskFree is not supported on this platform.
func DiskFree(string) (FSUsage, error) { return FSUsage{}, errors.ErrUnsupported }

// Mounts is not supported on this platform.
func Mounts() ([]FSUsage, error) { return nil, errors.ErrUnsupported }
