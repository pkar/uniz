//go:build !darwin && !linux

package fileops

import (
	"errors"
	"io/fs"
)

// ErrUnsupported is returned where the platform lacks the operation.
var errUnsupported = errors.ErrUnsupported

// SyncAll is not supported on this platform; sync individual files instead.
func SyncAll() error { return errUnsupported }

// Mkfifo is not supported on this platform.
func Mkfifo(path string, _ fs.FileMode, _ bool) error {
	return &fs.PathError{Op: "mkfifo", Path: path, Err: errUnsupported}
}
