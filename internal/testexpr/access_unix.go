//go:build unix

package testexpr

import "syscall"

// access reports whether the real user may access path with mode (4 read,
// 2 write, 1 execute), following symlinks.
func access(path string, mode uint32) bool { return syscall.Access(path, mode) == nil }
