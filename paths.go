package uniz

import (
	"context"

	"github.com/pkar/uniz/internal/mkdir"
	"github.com/pkar/uniz/internal/pathutil"
)

// BaseName returns the final component of a Unix path, stripping trailing
// slashes and an optional suffix. A suffix matching the entire basename is not
// removed. Empty input yields ""; all-slash input yields "/". It does not access
// the filesystem or clean dot components; '/' is the only separator.
func BaseName(path, suffix string) string { return pathutil.BaseName(path, suffix) }

// DirName returns the directory portion of a Unix path without filesystem
// access. Empty paths and paths without a directory yield ".". All-slash paths
// yield "/". Dot components are preserved; '/' is the only separator.
func DirName(path string) string { return pathutil.DirName(path) }

// MakeDirectoryOptions controls creation of missing parent directories.
type MakeDirectoryOptions = mkdir.Options

// MakeDirectory creates path with mode 0777 filtered by the process umask.
// With Parents, missing ancestors are created and existing directories accepted
// without changing permissions. Filesystem errors remain inspectable with
// errors.Is/As. Cancellation cannot interrupt an in-flight operation, and no
// directories are rolled back on failure or cancellation.
func MakeDirectory(ctx context.Context, path string, opts MakeDirectoryOptions) error {
	return mkdir.Create(ctx, path, opts)
}
