package uniz

import (
	"context"
	"io/fs"

	"github.com/pkar/uniz/internal/expr"
	"github.com/pkar/uniz/internal/fileops"
	"github.com/pkar/uniz/internal/pathutil"
	"github.com/pkar/uniz/internal/sysinfo"
)

// StatInfo is file metadata; Unix-only fields are valid when HasSys is set.
type StatInfo = fileops.StatInfo

// Stat returns metadata for path, following a final symlink when follow is set.
func Stat(path string, follow bool) (StatInfo, error) { return fileops.Stat(path, follow) }

// DefaultStatFormat is the layout stat uses without -c.
const DefaultStatFormat = fileops.DefaultStatFormat

// FormatStat expands stat -c directives such as %n, %s, %a, %A, %U, and %y.
func FormatStat(si StatInfo, format string) string { return fileops.FormatStat(si, format) }

// FileModeString renders a mode like ls -l, e.g. "drwxr-xr-x".
func FileModeString(m fs.FileMode) string { return fileops.ModeString(m) }

// DuOptions controls DiskUsage.
type DuOptions = fileops.DuOptions

// DuEntry is one DiskUsage report.
type DuEntry = fileops.DuEntry

// DiskUsage totals the space under root without following symlinks, calling
// report for each directory (and file with All), children first.
func DiskUsage(ctx context.Context, root string, opts DuOptions, report func(DuEntry) error) (int64, error) {
	return fileops.DiskUsage(ctx, root, opts, report)
}

// FSUsage describes a mounted filesystem's size and free space in bytes.
type FSUsage = sysinfo.FSUsage

// DiskFree reports the filesystem holding path. Not supported on Windows.
func DiskFree(path string) (FSUsage, error) { return sysinfo.DiskFree(path) }

// Mounts reports all mounted filesystems with a non-zero size.
func Mounts() ([]FSUsage, error) { return sysinfo.Mounts() }

// InstallOptions controls Install.
type InstallOptions = fileops.InstallOptions

// Install copies a regular file to dst with a mode, replacing dst atomically.
func Install(ctx context.Context, src, dst string, opts InstallOptions) error {
	return fileops.Install(ctx, src, dst, opts)
}

// ShredOptions controls Shred.
type ShredOptions = fileops.ShredOptions

// Shred overwrites a regular file in place; it refuses symlinks.
func Shred(ctx context.Context, path string, opts ShredOptions) error {
	return fileops.Shred(ctx, path, opts)
}

// SyncFile flushes one file's data to storage.
func SyncFile(path string) error { return fileops.SyncFile(path) }

// Mkfifo creates a named pipe; exact applies mode regardless of the umask.
func Mkfifo(path string, mode fs.FileMode, exact bool) error {
	return fileops.Mkfifo(path, mode, exact)
}

// ErrExprSyntax is wrapped by Expr for malformed expressions.
var ErrExprSyntax = expr.ErrSyntax

// Expr evaluates expr(1) arguments such as []string{"2", "+", "3"}.
func Expr(args []string) (string, error) { return expr.Evaluate(args) }

// ExprNull reports whether an Expr result counts as false (empty or 0).
func ExprNull(v string) bool { return expr.IsNull(v) }

// PathCheckOptions controls CheckPath.
type PathCheckOptions = pathutil.CheckOptions

// CheckPath reports whether a file name is valid and, optionally, portable.
func CheckPath(path string, opts PathCheckOptions) error { return pathutil.CheckPath(path, opts) }

// SignalNumber parses a signal name ("TERM", "SIGKILL") or number.
func SignalNumber(s string) (int, error) { return sysinfo.SignalNumber(s) }

// SignalName returns the name of a signal number without the SIG prefix.
func SignalName(n int) (string, bool) { return sysinfo.SignalName(n) }
