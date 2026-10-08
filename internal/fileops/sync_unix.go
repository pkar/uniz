//go:build darwin || linux

package fileops

import (
	"io/fs"
	"os"
	"syscall"
)

// SyncAll asks the kernel to flush all cached file data.
func SyncAll() error {
	syscall.Sync()
	return nil
}

// Mkfifo creates a named pipe. With exact, mode is applied after creation so
// the umask does not reduce it.
func Mkfifo(path string, mode fs.FileMode, exact bool) error {
	if err := syscall.Mkfifo(path, uint32(mode.Perm())); err != nil {
		return &fs.PathError{Op: "mkfifo", Path: path, Err: err}
	}
	if exact {
		return os.Chmod(path, mode)
	}
	return nil
}
