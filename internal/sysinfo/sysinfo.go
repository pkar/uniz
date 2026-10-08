// Package sysinfo reports operating system identification.
package sysinfo

// Info mirrors the fields of uname(2).
type Info struct {
	Sysname  string // kernel name, e.g. "Linux" or "Darwin"
	Nodename string // network host name
	Release  string // kernel release
	Version  string // kernel version string
	Machine  string // hardware name, e.g. "x86_64" or "arm64"
}
