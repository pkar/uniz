// Package uniz provides Unix-style commands and typed filesystem operations.
// Both interfaces run in-process; no external uniz executable is required.
package uniz

import (
	"context"
	"errors"
	"io"
	"strings"

	"github.com/pkar/uniz/internal/cli"
	"github.com/pkar/uniz/internal/ls"
)

// Config supplies the streams used by a Runner. Nil input is empty; nil output
// streams discard output. No process-global streams are used implicitly.
type Config struct {
	Stdin   io.Reader
	Stdout  io.Writer
	Stderr  io.Writer
	Version string // Defaults to "dev".
}

// Runner dispatches Unix commands. Its configuration is fixed at construction.
// Concurrent callers must supply streams safe for concurrent use.
// The zero value is usable with empty input and discarded output.
type Runner struct{ config Config }

// New constructs a runner. Run returns uniz's own diagnostics as errors and
// never writes them to Stderr. Stderr receives only the standard error of
// external programs started by env, xargs, timeout, nice, nohup, and time,
// and the timings that time reports.
func New(config Config) *Runner { return &Runner{config: config} }

// Run executes arguments such as []string{"ls", "-al", "."}. Do not include the
// executable name. It never calls os.Exit or prints returned errors. Cancellation
// is cooperative: it cannot interrupt an in-flight filesystem call or stream write.
func (r *Runner) Run(ctx context.Context, args []string) error {
	config := r.config
	if config.Stdout == nil {
		config.Stdout = io.Discard
	}
	if config.Stderr == nil {
		config.Stderr = io.Discard
	}
	if config.Version == "" {
		config.Version = "dev"
	}
	if config.Stdin == nil {
		config.Stdin = strings.NewReader("")
	}
	return cli.Run(ctx, args, config.Stdin, config.Stdout, config.Stderr, config.Version)
}

// UsageError indicates an unknown command or invalid command arguments.
type UsageError = cli.UsageError

// ExitError reports a specific exit status. When Err is nil the status is the
// whole result (false, test, grep with no match) and callers usually should
// not print anything; otherwise Err is the diagnostic.
type ExitError = cli.ExitError

// ExitCode maps nil to 0, *ExitError to its Code, usage errors to 2, and
// other errors to 1. Underlying filesystem and context errors remain
// available via errors.Is/As.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exit *ExitError
	if errors.As(err, &exit) {
		return exit.Code
	}
	var usage *UsageError
	if errors.As(err, &usage) {
		return 2
	}
	return 1
}

// ListOptions controls entry selection and ordering. Long affects only rendered
// command output; typed List always returns the available metadata.
type ListOptions = ls.Options

// Entry contains a name, path, lstat-style Info, and symlink LinkTarget.
type Entry = ls.Entry

// List returns structured entries without rendering output. Empty path means
// the current directory. Symlinks are not followed. Partial results may accompany
// an error. Cancellation is checked between filesystem operations.
func List(ctx context.Context, path string, opts ListOptions) ([]Entry, error) {
	return ls.ListContext(ctx, path, opts)
}
