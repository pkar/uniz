package sysinfo

import (
	"errors"
	"os"
	"sort"
	"strconv"
	"strings"
)

// FSUsage describes a mounted filesystem. Sizes are in bytes.
type FSUsage struct {
	Filesystem string // device or source, e.g. /dev/disk3s1
	MountPoint string
	Type       string
	Total      uint64
	Free       uint64 // free space, including space reserved for root
	Avail      uint64 // space available to unprivileged users
}

// Used returns Total minus Free.
func (u FSUsage) Used() uint64 { return u.Total - u.Free }

// ErrNotTerminal reports that a file is not a terminal.
var ErrNotTerminal = errors.New("not a tty")

// LoginName returns the login name recorded in the LOGNAME environment
// variable, which login(1) and sshd set. getlogin(3) needs cgo, so the
// utmp record is not consulted.
func LoginName() (string, error) {
	if name := os.Getenv("LOGNAME"); name != "" {
		return name, nil
	}
	return "", errors.New("no login name")
}

// SignalNumber parses a signal name ("TERM", "SIGTERM", any case) or number.
func SignalNumber(s string) (int, error) {
	if n, err := strconv.Atoi(s); err == nil {
		if _, ok := SignalName(n); ok || n == 0 {
			return n, nil
		}
		return 0, errors.New("invalid signal number " + s)
	}
	name := strings.TrimPrefix(strings.ToUpper(s), "SIG")
	for _, e := range signalTable {
		if e.name == name {
			return e.num, nil
		}
	}
	return 0, errors.New("unknown signal " + strconv.Quote(s))
}

// SignalName returns the name of signal n without the SIG prefix.
func SignalName(n int) (string, bool) {
	for _, e := range signalTable {
		if e.num == n {
			return e.name, true
		}
	}
	return "", false
}

// SignalNames lists the known signal names in numeric order.
func SignalNames() []string {
	table := append([]signalEntry(nil), signalTable...)
	sort.SliceStable(table, func(i, j int) bool { return table[i].num < table[j].num })
	var names []string
	for _, e := range table {
		names = append(names, e.name)
	}
	return names
}

type signalEntry struct {
	name string
	num  int
}
