package uniz

import (
	"context"
	"os"

	"github.com/pkar/uniz/internal/fileops"
)

// Errors returned (wrapped) by file operations; test with errors.Is.
var (
	ErrIsDirectory     = fileops.ErrIsDirectory
	ErrNotDirectory    = fileops.ErrNotDirectory
	ErrSameFile        = fileops.ErrSameFile
	ErrIntoItself      = fileops.ErrIntoItself
	ErrUnsupportedType = fileops.ErrUnsupportedType
	ErrRefused         = fileops.ErrRefused
)

// File operations use the process working directory for relative paths.
// Cancellation is checked between filesystem calls but cannot interrupt one.
// Destination arguments are exact: unlike the cp, mv, and ln commands, the
// typed functions never place a source inside an existing directory.

// TouchOptions controls Touch. A zero Time means now.
type TouchOptions = fileops.TouchOptions

// Touch updates access and modification times, creating an empty file unless
// NoCreate is set. Existing content is never truncated.
func Touch(ctx context.Context, path string, opts TouchOptions) error {
	return fileops.Touch(ctx, path, opts)
}

// RemoveOptions controls Remove.
type RemoveOptions = fileops.RemoveOptions

// Remove deletes path without following symlinks, even with a trailing slash.
// It refuses '.', '..', and '/' (ErrRefused). A directory requires Recursive
// or, if empty, Directories; otherwise ErrIsDirectory. Recursive removal uses
// os.RemoveAll and cannot be canceled once started.
func Remove(ctx context.Context, path string, opts RemoveOptions) error {
	return fileops.Remove(ctx, path, opts)
}

// RemoveDirectoryOptions controls RemoveDirectory.
type RemoveDirectoryOptions = fileops.RemoveDirectoryOptions

// RemoveDirectory removes an empty directory; files and symlinks return
// ErrNotDirectory. Parents also removes each parent named in path.
func RemoveDirectory(ctx context.Context, path string, opts RemoveDirectoryOptions) error {
	return fileops.RemoveDirectory(ctx, path, opts)
}

// CopyOptions controls Copy.
type CopyOptions = fileops.CopyOptions

// Copy copies src to exactly dst. Without Recursive, directories return
// ErrIsDirectory and src symlinks are followed; with it, symlinks are copied
// as links. Copying onto the same file (ErrSameFile) or a directory into
// itself (ErrIntoItself) is refused. Partial output is not removed on error.
func Copy(ctx context.Context, src, dst string, opts CopyOptions) error {
	return fileops.Copy(ctx, src, dst, opts)
}

// Move renames src to exactly dst, falling back to copy-then-remove across
// filesystems. On fallback failure src is kept; partial output may remain.
func Move(ctx context.Context, src, dst string) error { return fileops.Move(ctx, src, dst) }

// LinkOptions controls Link.
type LinkOptions = fileops.LinkOptions

// Link creates a hard or symbolic link named link pointing at target. Force
// atomically replaces an existing non-directory.
func Link(ctx context.Context, target, link string, opts LinkOptions) error {
	return fileops.Link(ctx, target, link, opts)
}

// RealPath returns an absolute path with symlinks resolved; every component
// must exist.
func RealPath(path string) (string, error) { return fileops.RealPath(path) }

// ReadLink returns a symbolic link's target without resolving it.
func ReadLink(path string) (string, error) { return os.Readlink(path) }
