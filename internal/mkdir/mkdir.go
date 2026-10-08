// Package mkdir creates directories using the host filesystem.
package mkdir

import (
	"context"
	"os"
)

// Options controls whether missing parents are created.
type Options struct{ Parents bool }

// Create creates path with permissions 0777 before the process umask. With
// Parents, existing directories are accepted and their permissions unchanged.
// Cancellation is checked before and after the filesystem operation; it cannot
// interrupt that operation. Created directories are never rolled back on error.
func Create(ctx context.Context, path string, opts Options) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	var err error
	if opts.Parents {
		err = os.MkdirAll(path, 0777)
	} else {
		err = os.Mkdir(path, 0777)
	}
	if err != nil {
		return err
	}
	return ctx.Err()
}
