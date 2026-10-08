//go:build !linux && !darwin && !freebsd && !netbsd && !openbsd && !dragonfly

package sysinfo

import (
	"os"
	"runtime"
	"strings"
)

// Uname approximates uname(2) where it is unavailable: the release and
// version are empty.
func Uname() (Info, error) {
	host, err := os.Hostname()
	name := runtime.GOOS
	if name != "" {
		name = strings.ToUpper(name[:1]) + name[1:]
	}
	return Info{Sysname: name, Nodename: host, Machine: runtime.GOARCH}, err
}
