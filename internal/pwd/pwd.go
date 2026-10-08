// Package pwd looks up the process working directory without changing it.
package pwd

import (
	"context"
	"os"
	"path/filepath"
)

// Options controls symbolic-link resolution.
type Options struct{ Physical bool }

// Directory returns os.Getwd's logical path by default (using a valid PWD), or
// resolves symbolic links for a physical path. It never changes directories.
func Directory(ctx context.Context, opts Options) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	if opts.Physical {
		dir, err = filepath.EvalSymlinks(dir)
	}
	if err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return dir, nil
}
