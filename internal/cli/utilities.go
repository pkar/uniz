package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/pkar/uniz/internal/pwd"
	"github.com/pkar/uniz/internal/text"
)

const catHelp = `Usage: uniz cat [--] [files...]
Copy files unchanged to stdout. No files or '-' reads stdin.
Supported options: --help, --. Numbering and display flags are not yet supported.
`
const wcHelp = `Usage: uniz wc [-lwcm] [--] [files...]
  -l  count newline bytes
  -w  count words separated by Unicode whitespace
  -c  count bytes
  -m  count Unicode characters (invalid UTF-8 bytes count individually)
No flags selects lines, words, and bytes. Output order: lines, words, bytes, chars.
No files or '-' reads stdin. Multiple files also print a total.
`
const pwdHelp = `Usage: uniz pwd [-L|-P]
  -L  preserve a valid logical PWD (default)
  -P  resolve symbolic links
The last option wins. Does not change the process working directory.
`

type countFields struct{ lines, words, bytes, chars bool }

func runUtility(ctx context.Context, command string, args []string, stdin io.Reader, stdout io.Writer) error {
	var paths []string
	var fields countFields
	var dirOpts pwd.Options
	literal := false
	for _, arg := range args {
		if !literal && arg == "--" {
			literal = true
			continue
		}
		if !literal && arg == "--help" {
			help := map[string]string{"cat": catHelp, "wc": wcHelp, "pwd": pwdHelp}[command]
			_, err := io.WriteString(stdout, help)
			return err
		}
		if literal || arg == "-" || !strings.HasPrefix(arg, "-") {
			paths = append(paths, arg)
			continue
		}
		for _, flag := range arg[1:] {
			switch {
			case command == "wc" && flag == 'l':
				fields.lines = true
			case command == "wc" && flag == 'w':
				fields.words = true
			case command == "wc" && flag == 'c':
				fields.bytes = true
			case command == "wc" && flag == 'm':
				fields.chars = true
			case command == "pwd" && flag == 'L':
				dirOpts.Physical = false
			case command == "pwd" && flag == 'P':
				dirOpts.Physical = true
			default:
				return &UsageError{Message: fmt.Sprintf("%s: unknown option %q", command, arg)}
			}
		}
	}
	if command == "pwd" {
		if len(paths) > 0 {
			return &UsageError{Message: "pwd: unexpected operand"}
		}
		dir, err := pwd.Directory(ctx, dirOpts)
		if err != nil {
			return fmt.Errorf("pwd: %w", err)
		}
		_, err = fmt.Fprintln(stdout, dir)
		return err
	}
	if fields == (countFields{}) {
		fields = countFields{lines: true, words: true, bytes: true}
	}
	implicit := len(paths) == 0
	if implicit {
		paths = []string{"-"}
	}
	var errs []error
	var total text.Counts
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return errors.Join(append(errs, err)...)
		}
		var src io.Reader = stdin
		var file *os.File
		if path != "-" {
			var err error
			file, err = os.Open(path)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", command, err))
				continue
			}
			src = file
		}
		if command == "cat" {
			// Distinguish output failures from input failures: a broken destination
			// stops processing; a failed input must not skip later operands.
			dst := &outputWriter{w: stdout}
			_, err := text.Cat(ctx, dst, src)
			if file != nil {
				err = errors.Join(err, file.Close())
			}
			if err != nil {
				errs = append(errs, fmt.Errorf("cat: %s: %w", path, err))
			}
			if dst.err != nil || ctx.Err() != nil {
				return errors.Join(append(errs, ctx.Err())...)
			}
			continue
		}
		counts, err := text.Count(ctx, src)
		if file != nil {
			err = errors.Join(err, file.Close())
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("wc: %s: %w", path, err))
		}
		if ctx.Err() != nil {
			return errors.Join(append(errs, ctx.Err())...)
		}
		total.Lines += counts.Lines
		total.Words += counts.Words
		total.Bytes += counts.Bytes
		total.Chars += counts.Chars
		name := path
		if implicit {
			name = ""
		}
		if err := writeCounts(stdout, counts, fields, name); err != nil {
			return errors.Join(append(errs, err)...)
		}
	}
	if command == "wc" && len(paths) > 1 {
		if err := writeCounts(stdout, total, fields, "total"); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

type outputWriter struct {
	w   io.Writer
	err error
}

func (w *outputWriter) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	n, err := w.w.Write(p)
	if err == nil && n != len(p) {
		err = io.ErrShortWrite
	}
	w.err = err
	return n, err
}

func writeCounts(w io.Writer, counts text.Counts, fields countFields, name string) error {
	var values []string
	for _, field := range []struct {
		enabled bool
		value   int64
	}{{fields.lines, counts.Lines}, {fields.words, counts.Words}, {fields.bytes, counts.Bytes}, {fields.chars, counts.Chars}} {
		if field.enabled {
			values = append(values, fmt.Sprint(field.value))
		}
	}
	if name != "" {
		values = append(values, name)
	}
	_, err := fmt.Fprintln(w, strings.Join(values, " "))
	return err
}
