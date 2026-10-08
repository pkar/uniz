package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/pkar/uniz/internal/mkdir"
	"github.com/pkar/uniz/internal/pathutil"
)

const basenameHelp = `Usage: uniz basename [--] name [suffix]
Strip directories and an optional suffix from one Unix path.
A suffix equal to the entire basename is not removed.
Multiple-name (-a), suffix (-s), and NUL-output (-z) flags are not supported.
`
const dirnameHelp = `Usage: uniz dirname [--] names...
Print the directory portion of each Unix path, one per line.
No filesystem access or dot-component normalization is performed.
`
const mkdirHelp = `Usage: uniz mkdir [-p] [--] directories...
  -p  create missing parents and accept existing directories
Permissions are 0777 filtered by the process umask. Existing permissions are
unchanged with -p. Mode (-m) and verbose (-v) flags are not supported.
`

func runPathCommand(ctx context.Context, command string, args []string, stdout io.Writer) error {
	var operands []string
	var opts mkdir.Options
	literal := false
	for _, arg := range args {
		if !literal && arg == "--" {
			literal = true
			continue
		}
		if !literal && arg == "--help" {
			help := map[string]string{"basename": basenameHelp, "dirname": dirnameHelp, "mkdir": mkdirHelp}[command]
			_, err := io.WriteString(stdout, help)
			return err
		}
		if !literal && arg != "-" && strings.HasPrefix(arg, "-") {
			if command == "mkdir" && strings.Trim(arg[1:], "p") == "" {
				opts.Parents = true
				continue
			}
			return &UsageError{Message: fmt.Sprintf("%s: unknown option %q", command, arg)}
		}
		operands = append(operands, arg)
	}
	if len(operands) == 0 {
		return &UsageError{Message: command + ": missing operand"}
	}
	if command == "basename" {
		if len(operands) > 2 {
			return &UsageError{Message: "basename: too many operands"}
		}
		suffix := ""
		if len(operands) == 2 {
			suffix = operands[1]
		}
		_, err := fmt.Fprintln(stdout, pathutil.BaseName(operands[0], suffix))
		return err
	}
	var errs []error
	for _, operand := range operands {
		if err := ctx.Err(); err != nil {
			return errors.Join(append(errs, err)...)
		}
		if command == "dirname" {
			if _, err := fmt.Fprintln(stdout, pathutil.DirName(operand)); err != nil {
				return err
			}
			continue
		}
		if err := mkdir.Create(ctx, operand, opts); err != nil {
			errs = append(errs, fmt.Errorf("mkdir: %w", err))
		}
	}
	return errors.Join(errs...)
}
