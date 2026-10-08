package uniz

import (
	"context"
	"io"
	"time"

	"github.com/pkar/uniz/internal/fileops"
	"github.com/pkar/uniz/internal/search"
	"github.com/pkar/uniz/internal/sysinfo"
)

// PatternSyntax selects how CompilePattern reads patterns.
type PatternSyntax = search.Syntax

// Pattern syntaxes.
const (
	BasicRegexp    = search.Basic    // POSIX basic (grep default)
	ExtendedRegexp = search.Extended // POSIX extended (grep -E)
	FixedStrings   = search.Fixed    // literal strings (grep -F)
)

// PatternOptions controls CompilePattern.
type PatternOptions = search.PatternOptions

// Matcher matches lines against grep patterns.
type Matcher = search.Matcher

// CompilePattern builds a Matcher that matches when any pattern matches.
// Patterns are translated to Go's RE2 syntax with leftmost-longest matching;
// back-references are not supported.
func CompilePattern(patterns []string, opts PatternOptions) (*Matcher, error) {
	return search.Compile(patterns, opts)
}

// GrepOptions controls Grep output.
type GrepOptions = search.GrepOptions

// Grep writes the lines of src selected by m and returns how many were
// selected. Output lines always end with a newline.
func Grep(ctx context.Context, dst io.Writer, src io.Reader, m *Matcher, opts GrepOptions) (int64, error) {
	return search.Grep(ctx, dst, src, m, opts)
}

// FindExpression is a compiled find expression.
type FindExpression = search.Expression

// ParseFindExpression compiles find expression arguments such as
// []string{"-name", "*.go", "-type", "f"}; now is the reference time for
// -mtime and -mmin.
func ParseFindExpression(args []string, now time.Time) (*FindExpression, error) {
	return search.ParseExpression(args, now)
}

// FindPaths walks roots without following symlinks and returns the paths
// that find would print for expression, in walk order. Errors for individual
// paths are joined and returned with the paths found.
func FindPaths(ctx context.Context, roots []string, expression []string) ([]string, error) {
	expr, err := search.ParseExpression(expression, time.Now())
	if err != nil {
		return nil, err
	}
	var paths []string
	err = expr.Walk(ctx, roots, func(p string, _ byte) error {
		paths = append(paths, p)
		return nil
	})
	return paths, err
}

// ModeChange computes a new mode from a file's current mode.
type ModeChange = fileops.ModeChange

// ParseMode parses an octal or symbolic chmod mode ("755", "u+x,go-w",
// "a=rX"). Without u/g/o/a a change applies to all classes; the umask is
// not consulted.
func ParseMode(spec string) (ModeChange, error) { return fileops.ParseMode(spec) }

// ChmodOptions controls Chmod.
type ChmodOptions = fileops.ChmodOptions

// Chmod applies change to path, following a symlink operand. With Recursive,
// entries below are changed without following symlinks.
func Chmod(ctx context.Context, path string, change ModeChange, opts ChmodOptions) error {
	return fileops.Chmod(ctx, path, change, opts)
}

// MakeTemp creates a unique file (0600) or directory (0700) from template,
// whose trailing run of at least three X characters is randomized, inside
// dir ("" keeps the template's directory). It returns the created path.
func MakeTemp(dir, template string, directory bool) (string, error) {
	return fileops.MakeTemp(dir, template, directory)
}

// SizeChange is a truncate size: absolute or relative to the current size.
type SizeChange = fileops.SizeChange

// ParseSize parses truncate sizes such as "100", "+1K", "<2MB", or "%4KiB".
func ParseSize(s string) (SizeChange, error) { return fileops.ParseSize(s) }

// TruncateOptions controls Truncate.
type TruncateOptions = fileops.TruncateOptions

// Truncate sets a file's size, creating it unless NoCreate is set.
func Truncate(ctx context.Context, path string, change SizeChange, opts TruncateOptions) error {
	return fileops.Truncate(ctx, path, change, opts)
}

// Unlink removes one non-directory without following symlinks.
func Unlink(path string) error { return fileops.Unlink(path) }

// SystemInfo mirrors uname(2).
type SystemInfo = sysinfo.Info

// Uname returns the kernel name, host name, release, version, and machine.
// Where uname(2) is unavailable, Release and Version are empty.
func Uname() (SystemInfo, error) { return sysinfo.Uname() }
