// Package ls lists filesystem entries without invoking an external command.
package ls

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Options controls selection, ordering, and rendering. The zero value lists
// visible entries in name order, one per line.
type Options struct {
	All       bool // Include hidden entries, including . and ...
	AlmostAll bool // Include hidden entries, except . and ...
	Directory bool // List a directory itself, not its contents.
	Long      bool // Print mode, byte size, modification time, and symlink target.
	Human     bool // With Long, print sizes like 1.1K, 15M (powers of 1024, rounded up).
	Reverse   bool // Reverse the selected ordering.
	SortTime  bool // Sort by modification time, newest first; break ties by name.
}

// Entry describes an entry without following symbolic links.
type Entry struct {
	Name       string
	Path       string
	Info       fs.FileInfo
	LinkTarget string
}

// List returns entries for path (an empty path means "."). Explicit hidden
// operands are always included. Symlinks are listed, not traversed. On partial
// failure, it returns both the entries it could read and a joined error.
func List(path string, opts Options) ([]Entry, error) {
	return ListContext(context.Background(), path, opts)
}

// ListContext is List with cooperative cancellation between filesystem operations.
func ListContext(ctx context.Context, path string, opts Options) ([]Entry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if path == "" {
		path = "."
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	var entries []Entry
	var errs []error
	add := func(name, path string, info fs.FileInfo) {
		entry := Entry{Name: name, Path: path, Info: info}
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				errs = append(errs, err)
			} else {
				entry.LinkTarget = target
			}
		}
		entries = append(entries, entry)
	}
	if !info.IsDir() || opts.Directory {
		add(path, path, info)
	} else {
		children, err := os.ReadDir(path)
		if err != nil {
			errs = append(errs, err)
		}
		if opts.All {
			for _, name := range []string{".", ".."} {
				p := filepath.Join(path, name)
				i, err := os.Lstat(p)
				if err != nil {
					errs = append(errs, err)
					continue
				}
				add(name, p, i)
			}
		}
		for _, child := range children {
			if err := ctx.Err(); err != nil {
				errs = append(errs, err)
				break
			}
			if strings.HasPrefix(child.Name(), ".") && !opts.All && !opts.AlmostAll {
				continue
			}
			i, err := child.Info()
			if err != nil {
				errs = append(errs, err)
				continue
			}
			add(child.Name(), filepath.Join(path, child.Name()), i)
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if opts.Reverse {
			a, b = b, a
		}
		if opts.SortTime && !a.Info.ModTime().Equal(b.Info.ModTime()) {
			return a.Info.ModTime().After(b.Info.ModTime())
		}
		return a.Name < b.Name
	})
	return entries, errors.Join(errs...)
}

// Run writes listings to w. With no paths it lists the current directory.
// Multiple operands are processed in argument order with directory headers.
// Filesystem errors do not prevent later operands from being listed; output
// errors stop processing immediately. Errors are returned, never printed.
func Run(w io.Writer, paths []string, opts Options) error {
	return RunContext(context.Background(), w, paths, opts)
}

// RunContext is Run with cooperative cancellation between operands and writes.
func RunContext(ctx context.Context, w io.Writer, paths []string, opts Options) error {
	if len(paths) == 0 {
		paths = []string{"."}
	}
	var errs []error
	for idx, path := range paths {
		if err := ctx.Err(); err != nil {
			return errors.Join(append(errs, err)...)
		}
		entries, err := ListContext(ctx, path, opts)
		if err != nil {
			errs = append(errs, err)
		}
		if err := ctx.Err(); err != nil {
			return errors.Join(append(errs, err)...)
		}
		// Even an empty directory needs a header when there are several operands.
		info, statErr := os.Lstat(path)
		if len(paths) > 1 && statErr == nil && info.IsDir() && !opts.Directory {
			prefix := ""
			if idx > 0 {
				prefix = "\n"
			}
			if _, err := fmt.Fprintf(w, "%s%s:\n", prefix, display(path)); err != nil {
				return errors.Join(append(errs, err)...)
			}
		}
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return errors.Join(append(errs, err)...)
			}
			name := display(entry.Name)
			var err error
			if opts.Long {
				if entry.Info.Mode()&os.ModeSymlink != 0 {
					name += " -> " + display(entry.LinkTarget)
				}
				size := strconv.FormatInt(entry.Info.Size(), 10)
				if opts.Human {
					size = HumanSize(entry.Info.Size())
				}
				_, err = fmt.Fprintf(w, "%s %8s %s %s\n", entry.Info.Mode(), size, entry.Info.ModTime().Format("2006-01-02 15:04"), name)
			} else {
				_, err = fmt.Fprintln(w, name)
			}
			if err != nil {
				return errors.Join(append(errs, err)...)
			}
		}
	}
	return errors.Join(errs...)
}

// Quote control characters to keep filenames from injecting terminal commands
// or splitting a one-entry-per-line listing. Ordinary names remain unchanged.
func display(s string) string {
	for _, r := range s {
		if r < 32 || r == 127 || (r >= 128 && r <= 159) || r == '\ufffd' {
			return fmt.Sprintf("%q", s)
		}
	}
	return s
}
