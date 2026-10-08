// Package fileops implements file creation, removal, copying, and linking.
package fileops

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/pkar/uniz/internal/pathutil"
	"github.com/pkar/uniz/internal/text"
)

// Sentinel errors, wrapped in *fs.PathError or formatted errors.
var (
	ErrIsDirectory     = errors.New("is a directory")
	ErrNotDirectory    = errors.New("not a directory")
	ErrSameFile        = errors.New("source and destination are the same file")
	ErrIntoItself      = errors.New("cannot copy or move a directory into itself")
	ErrUnsupportedType = errors.New("unsupported file type")
	ErrRefused         = errors.New("refusing to remove '.', '..', or '/'")
)

func pathErr(op, path string, err error) error { return &fs.PathError{Op: op, Path: path, Err: err} }

// trimSlash removes trailing slashes so that symlinks are never resolved
// through a trailing slash. An all-slash path becomes "/".
func trimSlash(p string) string {
	t := strings.TrimRight(p, "/")
	if t == "" && p != "" {
		return "/"
	}
	return t
}

// TouchOptions controls Touch.
type TouchOptions struct {
	NoCreate bool      // Do not create missing files.
	Time     time.Time // Access and modification time; zero means now.
}

// Touch sets access and modification times, creating an empty file if needed.
// Existing content is never truncated. Symlinks are followed.
func Touch(ctx context.Context, path string, opts TouchOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	t := opts.Time
	if t.IsZero() {
		t = time.Now()
	}
	err := os.Chtimes(path, t, t)
	if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if opts.NoCreate {
		return nil
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0o666)
	if err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Chtimes(path, t, t)
}

// RemoveOptions controls Remove.
type RemoveOptions struct {
	Recursive   bool // Remove directories and their contents.
	Force       bool // Ignore missing paths.
	Directories bool // Remove empty directories.
}

// Remove deletes path without following symlinks, even with a trailing slash.
// It refuses '.', '..', and the root directory. Recursive removal uses
// os.RemoveAll; cancellation is checked only before it starts.
func Remove(ctx context.Context, path string, opts RemoveOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if last := pathutil.BaseName(path, ""); last == "." || last == ".." {
		return pathErr("remove", path, ErrRefused)
	}
	target := trimSlash(path)
	if abs, err := filepath.Abs(target); err == nil && filepath.Dir(abs) == abs {
		return pathErr("remove", path, ErrRefused)
	}
	info, err := os.Lstat(target)
	if err != nil {
		if opts.Force && errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	if info.IsDir() {
		switch {
		case opts.Recursive:
			return os.RemoveAll(target)
		case opts.Directories:
			return os.Remove(target)
		}
		return pathErr("remove", path, ErrIsDirectory)
	}
	return os.Remove(target)
}

// RemoveDirectoryOptions controls RemoveDirectory.
type RemoveDirectoryOptions struct {
	Parents bool // Also remove each parent named in path, stopping at the first failure.
}

// RemoveDirectory removes an empty directory. Symlinks and files are errors.
func RemoveDirectory(ctx context.Context, path string, opts RemoveDirectoryOptions) error {
	remove := func(p string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		info, err := os.Lstat(trimSlash(p))
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return pathErr("rmdir", p, ErrNotDirectory)
		}
		return os.Remove(trimSlash(p))
	}
	if err := remove(path); err != nil {
		return err
	}
	if opts.Parents {
		for p := pathutil.DirName(path); p != "." && p != "/"; p = pathutil.DirName(p) {
			if err := remove(p); err != nil {
				return err
			}
		}
	}
	return nil
}

// CopyOptions controls Copy.
type CopyOptions struct {
	Recursive bool // Copy directories; symlinks inside (and src itself) are copied as links.
	Preserve  bool // Preserve permission bits and modification times.
}

// Copy copies src to exactly dst; it does not copy into an existing directory
// by appending src's name. Without Recursive, src symlinks are followed.
// Existing regular files are overwritten in place; partial output is not
// removed on failure. Only regular files, directories, and symlinks are
// supported. Cancellation is checked between reads, writes, and entries.
func Copy(ctx context.Context, src, dst string, opts CopyOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	stat := os.Stat
	if opts.Recursive {
		stat = os.Lstat
		src = trimSlash(src)
	}
	info, err := stat(src)
	if err != nil {
		return err
	}
	dstStat := os.Stat
	if info.Mode()&fs.ModeSymlink != 0 {
		dstStat = os.Lstat
	}
	if dinfo, err := dstStat(dst); err == nil && os.SameFile(info, dinfo) {
		return fmt.Errorf("copy %s to %s: %w", src, dst, ErrSameFile)
	}
	if info.IsDir() {
		if !opts.Recursive {
			return pathErr("copy", src, ErrIsDirectory)
		}
		if inside(src, dst) {
			return fmt.Errorf("copy %s to %s: %w", src, dst, ErrIntoItself)
		}
		return copyTree(ctx, src, dst, info, opts)
	}
	return copyEntry(ctx, src, dst, info, opts)
}

func copyEntry(ctx context.Context, src, dst string, info fs.FileInfo, opts CopyOptions) error {
	switch {
	case info.Mode()&fs.ModeSymlink != 0:
		target, err := os.Readlink(src)
		if err != nil {
			return err
		}
		return os.Symlink(target, dst)
	case info.Mode().IsRegular():
		return copyFile(ctx, src, dst, info, opts)
	}
	return pathErr("copy", src, ErrUnsupportedType)
}

func copyFile(ctx context.Context, src, dst string, info fs.FileInfo, opts CopyOptions) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	defer func() {
		if cerr := out.Close(); err == nil {
			err = cerr
		}
	}()
	if _, err = text.Cat(ctx, out, in); err != nil {
		return err
	}
	if opts.Preserve {
		if err = out.Chmod(info.Mode().Perm()); err != nil {
			return err
		}
		return os.Chtimes(dst, info.ModTime(), info.ModTime())
	}
	return nil
}

func copyTree(ctx context.Context, src, dst string, info fs.FileInfo, opts CopyOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	created := true
	if err := os.Mkdir(dst, info.Mode().Perm()|0o700); err != nil {
		if !errors.Is(err, fs.ErrExist) {
			return err
		}
		dinfo, serr := os.Stat(dst)
		if serr != nil {
			return serr
		}
		if !dinfo.IsDir() {
			return pathErr("copy", dst, ErrNotDirectory)
		}
		created = false
	}
	entries, err := os.ReadDir(src)
	var errs []error
	if err != nil {
		errs = append(errs, err)
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			errs = append(errs, err)
			break
		}
		s, d := filepath.Join(src, entry.Name()), filepath.Join(dst, entry.Name())
		child, err := os.Lstat(s)
		if err == nil {
			if child.IsDir() {
				err = copyTree(ctx, s, d, child, opts)
			} else {
				err = copyEntry(ctx, s, d, child, opts)
			}
		}
		if err != nil {
			errs = append(errs, err)
		}
	}
	if created || opts.Preserve {
		if err := os.Chmod(dst, info.Mode().Perm()); err != nil {
			errs = append(errs, err)
		}
	}
	if opts.Preserve {
		if err := os.Chtimes(dst, info.ModTime(), info.ModTime()); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// inside reports whether dst is src or lies within it, resolving symlinks in
// src and in dst's parent where possible.
func inside(src, dst string) bool {
	s, err := filepath.Abs(src)
	if err != nil {
		return false
	}
	if resolved, err := filepath.EvalSymlinks(s); err == nil {
		s = resolved
	}
	d, err := filepath.Abs(dst)
	if err != nil {
		return false
	}
	d = resolveExisting(d)
	rel, err := filepath.Rel(s, d)
	return err == nil && (rel == "." || rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// resolveExisting resolves symlinks in the deepest existing ancestor of the
// absolute path abs and re-appends the missing components.
func resolveExisting(abs string) string {
	var missing []string
	for p := abs; ; p = filepath.Dir(p) {
		if resolved, err := filepath.EvalSymlinks(p); err == nil {
			return filepath.Join(append([]string{resolved}, missing...)...)
		}
		if filepath.Dir(p) == p {
			return abs
		}
		missing = append([]string{filepath.Base(p)}, missing...)
	}
}

// Move renames src to exactly dst. Across filesystems it copies recursively
// (preserving modes, times, and symlinks) and then removes src; if copying
// fails, src is left in place and partial output may remain at dst.
func Move(ctx context.Context, src, dst string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	src = trimSlash(src)
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if dinfo, err := os.Lstat(dst); err == nil && os.SameFile(info, dinfo) {
		return fmt.Errorf("move %s to %s: %w", src, dst, ErrSameFile)
	}
	if info.IsDir() && inside(src, dst) {
		return fmt.Errorf("move %s to %s: %w", src, dst, ErrIntoItself)
	}
	err = os.Rename(src, dst)
	if err == nil || !errors.Is(err, syscall.EXDEV) {
		return err
	}
	if err := Copy(ctx, src, dst, CopyOptions{Recursive: true, Preserve: true}); err != nil {
		return fmt.Errorf("move %s to %s across filesystems (source kept): %w", src, dst, err)
	}
	return os.RemoveAll(src)
}

// LinkOptions controls Link.
type LinkOptions struct {
	Symbolic bool // Create a symbolic link instead of a hard link.
	Force    bool // Atomically replace an existing non-directory at link.
}

// Link creates link pointing at target. A relative symlink target is
// interpreted relative to link's directory. Force replaces via rename so the
// link name never disappears.
func Link(ctx context.Context, target, link string, opts LinkOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	create := func(name string) error {
		if opts.Symbolic {
			return os.Symlink(target, name)
		}
		return os.Link(target, name)
	}
	if !opts.Force {
		return create(link)
	}
	existing, err := os.Lstat(link)
	if errors.Is(err, fs.ErrNotExist) {
		return create(link)
	}
	if err != nil {
		return err
	}
	if existing.IsDir() {
		return pathErr("link", link, ErrIsDirectory)
	}
	if !opts.Symbolic {
		if t, err := os.Stat(target); err == nil && os.SameFile(t, existing) {
			return fmt.Errorf("link %s to %s: %w", link, target, ErrSameFile)
		}
	}
	var tmp string
	for range 100 {
		tmp = filepath.Join(filepath.Dir(link), fmt.Sprintf(".uniz-link-%d-%d", os.Getpid(), rand.Int64()))
		if err = create(tmp); !errors.Is(err, fs.ErrExist) {
			break
		}
	}
	if err != nil {
		return err
	}
	if err := os.Rename(tmp, link); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// RealPath returns an absolute path with all symlinks resolved. Every
// component must exist.
func RealPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}
