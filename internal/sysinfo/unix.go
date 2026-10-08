//go:build darwin || linux

package sysinfo

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"
)

var signalTable = []signalEntry{
	{"HUP", int(syscall.SIGHUP)}, {"INT", int(syscall.SIGINT)}, {"QUIT", int(syscall.SIGQUIT)},
	{"ILL", int(syscall.SIGILL)}, {"TRAP", int(syscall.SIGTRAP)}, {"ABRT", int(syscall.SIGABRT)},
	{"BUS", int(syscall.SIGBUS)}, {"FPE", int(syscall.SIGFPE)}, {"KILL", int(syscall.SIGKILL)},
	{"USR1", int(syscall.SIGUSR1)}, {"SEGV", int(syscall.SIGSEGV)}, {"USR2", int(syscall.SIGUSR2)},
	{"PIPE", int(syscall.SIGPIPE)}, {"ALRM", int(syscall.SIGALRM)}, {"TERM", int(syscall.SIGTERM)},
	{"CHLD", int(syscall.SIGCHLD)}, {"CONT", int(syscall.SIGCONT)}, {"STOP", int(syscall.SIGSTOP)},
	{"TSTP", int(syscall.SIGTSTP)}, {"TTIN", int(syscall.SIGTTIN)}, {"TTOU", int(syscall.SIGTTOU)},
	{"URG", int(syscall.SIGURG)}, {"XCPU", int(syscall.SIGXCPU)}, {"XFSZ", int(syscall.SIGXFSZ)},
	{"VTALRM", int(syscall.SIGVTALRM)}, {"PROF", int(syscall.SIGPROF)}, {"WINCH", int(syscall.SIGWINCH)},
	{"IO", int(syscall.SIGIO)}, {"SYS", int(syscall.SIGSYS)},
}

// Kill sends signal sig to pid. As with kill(2), pid 0 means the process
// group, negative pids name a process group, and signal 0 only checks that
// the process exists.
func Kill(pid, sig int) error { return syscall.Kill(pid, syscall.Signal(sig)) }

// Niceness returns this process's scheduling niceness (-20..19).
func Niceness() (int, error) {
	p, err := syscall.Getpriority(syscall.PRIO_PROCESS, 0)
	return kernelPriority(p), err
}

// SetNiceness sets the niceness of process pid. Lowering it needs privilege.
func SetNiceness(pid, n int) error { return syscall.Setpriority(syscall.PRIO_PROCESS, pid, n) }

// TerminalName returns the device path of the terminal open as f, or an
// error wrapping ErrNotTerminal.
func TerminalName(f *os.File) (string, error) {
	conn, err := f.SyscallConn()
	if err != nil {
		return "", ErrNotTerminal
	}
	var errno syscall.Errno
	var st syscall.Stat_t
	var statErr error
	var fd uintptr
	if err := conn.Control(func(f uintptr) {
		var t syscall.Termios
		_, _, errno = syscall.Syscall(syscall.SYS_IOCTL, f, ioctlGetTermios, uintptr(unsafe.Pointer(&t)))
		statErr, fd = syscall.Fstat(int(f), &st), f
	}); err != nil {
		return "", err
	}
	if errno != 0 {
		return "", ErrNotTerminal
	}
	if runtime.GOOS == "linux" {
		if p, err := os.Readlink(fmt.Sprintf("/proc/self/fd/%d", fd)); err == nil {
			return p, nil
		}
	}
	if statErr != nil {
		return "", statErr
	}
	for _, dir := range []string{"/dev/pts", "/dev"} {
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if e.Type()&fs.ModeCharDevice == 0 || e.Name() == "tty" {
				continue
			}
			p := filepath.Join(dir, e.Name())
			info, err := os.Stat(p)
			if err != nil {
				continue
			}
			if s, ok := info.Sys().(*syscall.Stat_t); ok && uint64(s.Rdev) == uint64(st.Rdev) {
				return p, nil
			}
		}
	}
	return "", errors.New("terminal device not found")
}
