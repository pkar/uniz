// Package cli implements argument handling independently of process globals.
package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/pkar/uniz/internal/ls"
)

const lsHelp = `Usage: uniz ls [-aAdhlrt1] [--] [paths...]

  -a  include hidden entries, including . and ..
  -A  include hidden entries, except . and ..
  -l  show mode, byte size, modification time, and symlink target
  -d  list directories themselves
  -h  with -l, show sizes like 1.1K and 15M (powers of 1024)
  -r  reverse the sort order
  -t  sort by modification time, newest first
  -1  one entry per line (the default)
  --  treat remaining arguments as paths

Short options may be combined and may appear after paths.
Symlinks are listed without following them. Default path: current directory.
`

// UsageError indicates invalid command arguments.
type UsageError struct{ Message string }

func (e *UsageError) Error() string { return e.Message }

// Run dispatches a command without exiting or printing errors.
func Run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer, version string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	print := func(s string) error {
		_, err := io.WriteString(stdout, s)
		return err
	}
	if len(args) == 0 {
		return print(mainHelp())
	}
	switch args[0] {
	case "help", "-h", "--help":
		return print(mainHelp())
	case "--version":
		return print("uniz " + version + "\n")
	}
	cmd, ok := registry[args[0]]
	if !ok {
		return &UsageError{Message: fmt.Sprintf("unknown command %q", args[0])}
	}
	if wantsHelp(args[1:], cmd.literal) {
		return print(cmd.usage)
	}
	return cmd.run(ctx, stdio{in: stdin, out: stdout, err: stderr}, args[1:])
}

func runLs(ctx context.Context, s stdio, args []string) error {
	var opts ls.Options
	var paths []string
	literal := false
	for _, arg := range args {
		if literal || arg == "-" || !strings.HasPrefix(arg, "-") {
			paths = append(paths, arg)
			continue
		}
		if arg == "--" {
			literal = true
			continue
		}
		for _, flag := range arg[1:] {
			switch flag {
			case 'a':
				opts.All = true
			case 'A':
				opts.AlmostAll = true
			case 'l':
				opts.Long = true
			case 'h':
				opts.Human = true
			case 'd':
				opts.Directory = true
			case 'r':
				opts.Reverse = true
			case 't':
				opts.SortTime = true
			case '1':
			default:
				return &UsageError{Message: fmt.Sprintf("ls: unknown option %q", arg)}
			}
		}
	}
	if err := ls.RunContext(ctx, s.out, paths, opts); err != nil {
		return fmt.Errorf("ls: %w", err)
	}
	return nil
}
