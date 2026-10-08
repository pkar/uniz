package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"os"
	"os/user"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/pkar/uniz/internal/fileops"
	"github.com/pkar/uniz/internal/format"
	"github.com/pkar/uniz/internal/pathutil"
	"github.com/pkar/uniz/internal/text"
)

// ExitError reports a specific non-zero exit status. Err, when set, is the
// diagnostic to show; a nil Err means the status alone is the result, as with
// false, test, or grep finding nothing.
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return fmt.Sprintf("exit status %d", e.Code)
}

func (e *ExitError) Unwrap() error { return e.Err }

type stdio struct {
	in  io.Reader
	out io.Writer
	err io.Writer // diagnostics from child processes only
}

type command struct {
	summary string
	usage   string
	literal bool // --help is recognized only as the sole argument
	run     func(context.Context, stdio, []string) error
}

var registry map[string]command

func init() {
	utility := func(name string) func(context.Context, stdio, []string) error {
		return func(ctx context.Context, s stdio, a []string) error { return runUtility(ctx, name, a, s.in, s.out) }
	}
	pathCommand := func(name string) func(context.Context, stdio, []string) error {
		return func(ctx context.Context, s stdio, a []string) error { return runPathCommand(ctx, name, a, s.out) }
	}
	registry = map[string]command{
		"basename": {"strip directory and suffix from a path", basenameHelp, false, pathCommand("basename")},
		"cat":      {"concatenate files or stdin", catHelp, false, utility("cat")},
		"cp":       {"copy files and directories", cpHelp, false, runCp},
		"dirname":  {"print the directory part of paths", dirnameHelp, false, pathCommand("dirname")},
		"echo":     {"print arguments", echoHelp, true, runEcho},
		"false":    {"exit with status 1", falseHelp, true, func(context.Context, stdio, []string) error { return &ExitError{Code: 1} }},
		"head":     {"print the first lines or bytes", headHelp, false, runSlice("head")},
		"hostname": {"print the host name", hostnameHelp, false, runHostname},
		"ln":       {"create hard or symbolic links", lnHelp, false, runLn},
		"ls":       {"list directory contents", lsHelp, false, runLs},
		"mkdir":    {"create directories", mkdirHelp, false, pathCommand("mkdir")},
		"mv":       {"move or rename files", mvHelp, false, runMv},
		"printenv": {"print environment variables", printenvHelp, false, runPrintenv},
		"pwd":      {"print the working directory", pwdHelp, false, utility("pwd")},
		"readlink": {"print symbolic link targets", readlinkHelp, false, runReadlink},
		"realpath": {"print resolved absolute paths", realpathHelp, false, runRealpath},
		"rm":       {"remove files and directories", rmHelp, false, runRm},
		"rmdir":    {"remove empty directories", rmdirHelp, false, runRmdir},
		"seq":      {"print a sequence of integers", seqHelp, false, runSeq},
		"sleep":    {"wait for a duration", sleepHelp, false, runSleep},
		"sort":     {"sort lines", sortHelp, false, runSort},
		"tail":     {"print the last lines or bytes", tailHelp, false, runSlice("tail")},
		"tee":      {"copy stdin to stdout and files", teeHelp, false, runTee},
		"touch":    {"create files or update timestamps", touchHelp, false, runTouch},
		"true":     {"exit with status 0", trueHelp, true, func(context.Context, stdio, []string) error { return nil }},
		"uniq":     {"collapse adjacent duplicate lines", uniqHelp, false, runUniq},
		"wc":       {"count lines, words, bytes, and characters", wcHelp, false, utility("wc")},
		"whoami":   {"print the current user name", whoamiHelp, false, runWhoami},
		"yes":      {"repeat a line until stopped", yesHelp, true, runYes},
	}
	registerTextTools()
	registerSystemTools()
	registerMoreText()
	registerMoreSystem()
}

func mainHelp() string {
	var b strings.Builder
	b.WriteString("Usage: uniz <command> [options] [operands...]\n\nCommands:\n")
	for _, name := range slices.Sorted(maps.Keys(registry)) {
		fmt.Fprintf(&b, "  %-9s %s\n", name, registry[name].summary)
	}
	b.WriteString("\nRun 'uniz <command> --help' for command options.\n")
	return b.String()
}

func wantsHelp(args []string, literal bool) bool {
	if literal {
		return len(args) == 1 && args[0] == "--help"
	}
	for _, a := range args {
		if a == "--" {
			return false
		}
		if a == "--help" {
			return true
		}
	}
	return false
}

func usageErr(format string, a ...any) error { return &UsageError{Message: fmt.Sprintf(format, a...)} }

type options struct {
	set      map[rune]bool
	values   map[rune]string   // last value of each value flag
	lists    map[rune][]string // every value of each value flag, in order
	operands []string
}

// parseOptions parses combinable short options anywhere before "--".
// valueFlags take the rest of the argument or the next argument. A lone "-"
// is an operand. Long options other than --help are rejected.
func parseOptions(command string, args []string, boolFlags, valueFlags string) (options, error) {
	o := options{set: map[rune]bool{}, values: map[rune]string{}, lists: map[rune][]string{}}
	literal := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if literal || arg == "-" || !strings.HasPrefix(arg, "-") {
			o.operands = append(o.operands, arg)
			continue
		}
		if arg == "--" {
			literal = true
			continue
		}
		flags := []rune(arg[1:])
		for j, f := range flags {
			if strings.ContainsRune(valueFlags, f) {
				value := string(flags[j+1:])
				if value == "" {
					if i+1 >= len(args) {
						return o, usageErr("%s: option -%c requires a value", command, f)
					}
					i++
					value = args[i]
				}
				o.set[f], o.values[f] = true, value
				o.lists[f] = append(o.lists[f], value)
				break
			}
			if !strings.ContainsRune(boolFlags, f) {
				return o, usageErr("%s: unknown option %q", command, arg)
			}
			o.set[f] = true
		}
	}
	return o, nil
}

func inputPaths(operands []string) []string {
	if len(operands) == 0 {
		return []string{"-"}
	}
	return operands
}

// eachInput opens each path ('-' means stdin) and calls fn. Open and read
// errors are collected and later inputs still run; an output error recorded
// in out or cancellation stops immediately.
func eachInput(ctx context.Context, command string, paths []string, stdin io.Reader, out *outputWriter, fn func(path string, r io.Reader) error) error {
	var errs []error
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return errors.Join(append(errs, err)...)
		}
		var r io.Reader = stdin
		var f *os.File
		if path != "-" {
			var err error
			if f, err = os.Open(path); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", command, err))
				continue
			}
			r = f
		}
		err := fn(path, r)
		if f != nil {
			err = errors.Join(err, f.Close())
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %s: %w", command, path, err))
		}
		if out.err != nil || ctx.Err() != nil {
			return errors.Join(append(errs, ctx.Err())...)
		}
	}
	return errors.Join(errs...)
}

// forEach runs fn for each operand, collecting errors and stopping on cancellation.
func forEach(ctx context.Context, command string, operands []string, fn func(string) error) error {
	var errs []error
	for _, operand := range operands {
		if err := ctx.Err(); err != nil {
			return errors.Join(append(errs, err)...)
		}
		if err := fn(operand); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", command, err))
		}
	}
	return errors.Join(errs...)
}

const echoHelp = `Usage: uniz echo [-neE] [strings...]
  -n  do not print the trailing newline
  -e  interpret backslash escapes (\\ \a \b \c \e \f \n \r \t \v \0NNN \xHH)
  -E  do not interpret escapes (default)
Leading arguments made only of n, e, and E letters are options; everything
else, including "--", is printed. \c stops all further output.
`

func runEcho(_ context.Context, s stdio, args []string) error {
	newline, escapes := true, false
	for len(args) > 0 && len(args[0]) > 1 && args[0][0] == '-' && strings.Trim(args[0][1:], "neE") == "" {
		for _, f := range args[0][1:] {
			switch f {
			case 'n':
				newline = false
			case 'e':
				escapes = true
			case 'E':
				escapes = false
			}
		}
		args = args[1:]
	}
	out := strings.Join(args, " ")
	if escapes {
		var stop bool
		if out, stop = format.Escapes(out); stop {
			newline = false
		}
	}
	if newline {
		out += "\n"
	}
	_, err := io.WriteString(s.out, out)
	return err
}

const trueHelp = "Usage: uniz true\nExit with status 0, ignoring arguments.\n"
const falseHelp = "Usage: uniz false\nExit with status 1, ignoring arguments.\n"
const yesHelp = `Usage: uniz yes [strings...]
Print the arguments joined by spaces (default "y") until output fails or the
command is canceled. All arguments are printed literally.
`

func runYes(ctx context.Context, s stdio, args []string) error {
	line := "y"
	if len(args) > 0 {
		line = strings.Join(args, " ")
	}
	return text.Yes(ctx, s.out, line)
}

const headHelp = `Usage: uniz head [-n lines | -c bytes] [--] [files...]
  -n N  print the first N lines (default 10); -N is shorthand
  -c N  print the first N bytes
No files or '-' reads stdin. Multiple files get "==> name <==" headers.
Negative counts (all but the last N) are not supported.
`
const tailHelp = `Usage: uniz tail [-n lines | -c bytes] [--] [files...]
  -n N   print the last N lines (default 10); -N is shorthand
  -n +N  print starting at line N
  -c N   print the last N bytes; -c +N starts at byte N
No files or '-' reads stdin. Multiple files get "==> name <==" headers.
Following (-f) is not supported. Input is read to the end.
`

func runSlice(command string) func(context.Context, stdio, []string) error {
	return func(ctx context.Context, s stdio, args []string) error {
		args = slices.Clone(args)
		for i, a := range args {
			if a == "--" {
				break
			}
			afterValueFlag := i > 0 && (args[i-1] == "-n" || args[i-1] == "-c")
			if !afterValueFlag && len(a) > 1 && a[0] == '-' && strings.Trim(a[1:], "0123456789") == "" {
				args[i] = "-n" + a[1:]
			}
		}
		o, err := parseOptions(command, args, "", "nc")
		if err != nil {
			return err
		}
		if o.set['n'] && o.set['c'] {
			return usageErr("%s: -n and -c cannot be combined", command)
		}
		opts := text.SliceOptions{Count: 10}
		if value, ok := o.values['n']; ok || o.set['c'] {
			if !ok {
				value, opts.Bytes = o.values['c'], true
			}
			v := value
			if command == "tail" {
				if strings.HasPrefix(v, "+") {
					opts.FromStart, v = true, v[1:]
				} else {
					v = strings.TrimPrefix(v, "-")
				}
			}
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil || n < 0 {
				return usageErr("%s: invalid count %q", command, value)
			}
			opts.Count = n
		}
		paths := inputPaths(o.operands)
		out := &outputWriter{w: s.out}
		first := true
		return eachInput(ctx, command, paths, s.in, out, func(path string, r io.Reader) error {
			if len(paths) > 1 {
				prefix := "\n"
				if first {
					prefix = ""
				}
				first = false
				name := path
				if path == "-" {
					name = "standard input"
				}
				if _, err := fmt.Fprintf(out, "%s==> %s <==\n", prefix, name); err != nil {
					return err
				}
			}
			if command == "head" {
				return text.Head(ctx, out, r, opts)
			}
			return text.Tail(ctx, out, r, opts)
		})
	}
}

const teeHelp = `Usage: uniz tee [-a] [--] [files...]
  -a  append to files instead of truncating them
Copy stdin to stdout and every file. A file that cannot be opened or written
is reported and skipped; the others continue. Output failure on stdout stops.
`

func runTee(ctx context.Context, s stdio, args []string) error {
	o, err := parseOptions("tee", args, "a", "")
	if err != nil {
		return err
	}
	flag := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	if o.set['a'] {
		flag = os.O_WRONLY | os.O_CREATE | os.O_APPEND
	}
	var errs []error
	type sink struct {
		name string
		f    *os.File
	}
	var sinks []sink
	for _, name := range o.operands {
		f, err := os.OpenFile(name, flag, 0o666)
		if err != nil {
			errs = append(errs, fmt.Errorf("tee: %w", err))
			continue
		}
		sinks = append(sinks, sink{name, f})
	}
	out := &outputWriter{w: s.out}
	buf := make([]byte, 32*1024)
	for {
		if err := ctx.Err(); err != nil {
			errs = append(errs, err)
			break
		}
		n, rerr := s.in.Read(buf)
		if n > 0 {
			if _, err := out.Write(buf[:n]); err != nil {
				errs = append(errs, err)
				break
			}
			for i := 0; i < len(sinks); {
				if _, err := sinks[i].f.Write(buf[:n]); err != nil {
					errs = append(errs, fmt.Errorf("tee: %s: %w", sinks[i].name, err))
					_ = sinks[i].f.Close()
					sinks = slices.Delete(sinks, i, i+1)
					continue
				}
				i++
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			errs = append(errs, fmt.Errorf("tee: %w", rerr))
			break
		}
	}
	for _, sk := range sinks {
		if err := sk.f.Close(); err != nil {
			errs = append(errs, fmt.Errorf("tee: %s: %w", sk.name, err))
		}
	}
	return errors.Join(errs...)
}

const sortHelp = `Usage: uniz sort [-fnru] [--] [files...]
  -f  fold lower case to upper case when comparing
  -n  compare leading numbers (as float64); non-numeric lines sort as 0
  -r  reverse the order
  -u  output only the first line of each run of equal keys
Ties are broken by byte order (except with -u). Inputs are concatenated and
held in memory. If any input cannot be read, nothing is written.
Keys (-k), separators (-t), and output files (-o) are not supported.
`

func runSort(ctx context.Context, s stdio, args []string) error {
	o, err := parseOptions("sort", args, "fnru", "")
	if err != nil {
		return err
	}
	var lines []string
	err = eachInput(ctx, "sort", inputPaths(o.operands), s.in, &outputWriter{w: io.Discard}, func(_ string, r io.Reader) error {
		got, err := text.ReadLines(ctx, r)
		lines = append(lines, got...)
		return err
	})
	if err != nil {
		return err
	}
	opts := text.SortOptions{Reverse: o.set['r'], Numeric: o.set['n'], Unique: o.set['u'], IgnoreCase: o.set['f']}
	return text.WriteLines(ctx, s.out, text.SortLines(lines, opts))
}

const uniqHelp = `Usage: uniz uniq [-cdiu] [--] [file]
  -c  prefix lines with their repeat count
  -d  print only repeated lines
  -i  ignore case when comparing
  -u  print only lines that are not repeated
Only adjacent lines are compared; sort first to find all duplicates.
An output-file operand and field/character skipping are not supported.
`

func runUniq(ctx context.Context, s stdio, args []string) error {
	o, err := parseOptions("uniq", args, "cdiu", "")
	if err != nil {
		return err
	}
	if len(o.operands) > 1 {
		return usageErr("uniq: output file operand is not supported")
	}
	opts := text.UniqOptions{Count: o.set['c'], Repeated: o.set['d'], Unique: o.set['u'], IgnoreCase: o.set['i']}
	out := &outputWriter{w: s.out}
	return eachInput(ctx, "uniq", inputPaths(o.operands), s.in, out, func(_ string, r io.Reader) error {
		return text.Uniq(ctx, out, r, opts)
	})
}

const seqHelp = `Usage: uniz seq [-s separator] [first [step]] last
Print integers from first (default 1) to last by step (default 1).
  -s SEP  separate numbers with SEP instead of a newline
Negative numbers are operands, not options. Floating-point values,
-w, and -f are not supported.
`

func runSeq(ctx context.Context, s stdio, args []string) error {
	sep := "\n"
	var nums []string
	literal := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case literal || !strings.HasPrefix(a, "-") || len(a) > 1 && a[1] >= '0' && a[1] <= '9':
			nums = append(nums, a)
		case a == "--":
			literal = true
		case a == "-s":
			if i+1 >= len(args) {
				return usageErr("seq: option -s requires a value")
			}
			i++
			sep = args[i]
		case strings.HasPrefix(a, "-s"):
			sep = a[2:]
		default:
			return usageErr("seq: unknown option %q", a)
		}
	}
	if len(nums) == 0 || len(nums) > 3 {
		return usageErr("seq: expected 1 to 3 operands")
	}
	values := make([]int64, len(nums))
	for i, n := range nums {
		v, err := strconv.ParseInt(n, 10, 64)
		if err != nil {
			return usageErr("seq: invalid integer %q", n)
		}
		values[i] = v
	}
	opts := text.SeqOptions{First: 1, Step: 1, Separator: sep}
	switch len(values) {
	case 1:
		opts.Last = values[0]
	case 2:
		opts.First, opts.Last = values[0], values[1]
	case 3:
		opts.First, opts.Step, opts.Last = values[0], values[1], values[2]
	}
	if opts.Step == 0 {
		return usageErr("seq: step must not be zero")
	}
	return text.Seq(ctx, s.out, opts)
}

const sleepHelp = `Usage: uniz sleep duration...
Wait for the sum of the durations. Each is a non-negative number with an
optional suffix: s (seconds, default), m (minutes), h (hours), d (days).
Fractions are allowed. Cancellation ends the wait early with an error.
`

// parseInterval parses a non-negative number of seconds with an optional
// s, m, h, or d suffix.
func parseInterval(command, a string) (time.Duration, error) {
	num, unit := a, 1.0
	if n := len(a); n > 0 {
		if u, ok := map[byte]float64{'s': 1, 'm': 60, 'h': 3600, 'd': 86400}[a[n-1]]; ok {
			num, unit = a[:n-1], u
		}
	}
	v, err := strconv.ParseFloat(num, 64)
	if err != nil || v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, usageErr("%s: invalid time interval %q", command, a)
	}
	secs := v * unit
	if secs >= float64(math.MaxInt64)/float64(time.Second) {
		return 0, usageErr("%s: time interval %q is too large", command, a)
	}
	return time.Duration(secs * float64(time.Second)), nil
}

func runSleep(ctx context.Context, _ stdio, args []string) error {
	var total time.Duration
	var operands []string
	literal := false
	for _, a := range args {
		switch {
		case !literal && a == "--":
			literal = true
		case !literal && strings.HasPrefix(a, "-"):
			return usageErr("sleep: invalid time interval %q", a)
		default:
			operands = append(operands, a)
		}
	}
	if len(operands) == 0 {
		return usageErr("sleep: missing operand")
	}
	for _, a := range operands {
		d, err := parseInterval("sleep", a)
		if err != nil {
			return err
		}
		if d > math.MaxInt64-total {
			return usageErr("sleep: time interval %q is too large", a)
		}
		total += d
	}
	timer := time.NewTimer(total)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

const printenvHelp = `Usage: uniz printenv [--] [names...]
Print all NAME=value pairs, or the value of each named variable.
Exits with status 1 (without a message) if any named variable is unset.
Reads the process environment.
`

func runPrintenv(_ context.Context, s stdio, args []string) error {
	o, err := parseOptions("printenv", args, "", "")
	if err != nil {
		return err
	}
	out := &outputWriter{w: s.out}
	if len(o.operands) == 0 {
		for _, kv := range os.Environ() {
			if _, err := fmt.Fprintln(out, kv); err != nil {
				return err
			}
		}
		return nil
	}
	missing := false
	for _, name := range o.operands {
		v, ok := os.LookupEnv(name)
		if !ok || strings.Contains(name, "=") {
			missing = true
			continue
		}
		if _, err := fmt.Fprintln(out, v); err != nil {
			return err
		}
	}
	if missing {
		return &ExitError{Code: 1}
	}
	return nil
}

const whoamiHelp = "Usage: uniz whoami\nPrint the user name of the current process.\n"
const hostnameHelp = "Usage: uniz hostname\nPrint the host name reported by the kernel. Setting it is not supported.\n"

func noOperands(command string, args []string) error {
	o, err := parseOptions(command, args, "", "")
	if err == nil && len(o.operands) > 0 {
		err = usageErr("%s: unexpected operand", command)
	}
	return err
}

func runWhoami(_ context.Context, s stdio, args []string) error {
	if err := noOperands("whoami", args); err != nil {
		return err
	}
	u, err := user.Current()
	if err != nil {
		return fmt.Errorf("whoami: %w", err)
	}
	_, err = fmt.Fprintln(s.out, u.Username)
	return err
}

func runHostname(_ context.Context, s stdio, args []string) error {
	if err := noOperands("hostname", args); err != nil {
		return err
	}
	name, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("hostname: %w", err)
	}
	_, err = fmt.Fprintln(s.out, name)
	return err
}

const touchHelp = `Usage: uniz touch [-c] [--] files...
  -c  do not create missing files
Set access and modification times to now, creating empty files as needed.
Existing content is never changed. Explicit times (-d, -t, -r) are not supported.
`

func runTouch(ctx context.Context, _ stdio, args []string) error {
	o, err := parseOptions("touch", args, "c", "")
	if err != nil {
		return err
	}
	if len(o.operands) == 0 {
		return usageErr("touch: missing file operand")
	}
	return forEach(ctx, "touch", o.operands, func(p string) error {
		return fileops.Touch(ctx, p, fileops.TouchOptions{NoCreate: o.set['c']})
	})
}

const rmHelp = `Usage: uniz rm [-dfrR] [--] paths...
  -d      remove empty directories
  -f      ignore missing paths and missing operands
  -r, -R  remove directories and their contents
Never prompts. Symlinks are removed, never followed, even with a trailing
slash. Refuses '.', '..', and '/'. Interactive (-i) is not supported.
`

func runRm(ctx context.Context, _ stdio, args []string) error {
	o, err := parseOptions("rm", args, "dfrR", "")
	if err != nil {
		return err
	}
	if len(o.operands) == 0 {
		if o.set['f'] {
			return nil
		}
		return usageErr("rm: missing operand")
	}
	opts := fileops.RemoveOptions{Recursive: o.set['r'] || o.set['R'], Force: o.set['f'], Directories: o.set['d']}
	return forEach(ctx, "rm", o.operands, func(p string) error { return fileops.Remove(ctx, p, opts) })
}

const rmdirHelp = `Usage: uniz rmdir [-p] [--] directories...
  -p  also remove each named parent directory (a/b/c removes c, b, then a)
Only empty directories are removed. Symlinks are not followed.
`

func runRmdir(ctx context.Context, _ stdio, args []string) error {
	o, err := parseOptions("rmdir", args, "p", "")
	if err != nil {
		return err
	}
	if len(o.operands) == 0 {
		return usageErr("rmdir: missing operand")
	}
	opts := fileops.RemoveDirectoryOptions{Parents: o.set['p']}
	return forEach(ctx, "rmdir", o.operands, func(p string) error { return fileops.RemoveDirectory(ctx, p, opts) })
}

// targets pairs each source with its destination. The final operand is the
// destination; when it is an existing directory, sources go inside it under
// their base names. Multiple sources require an existing directory.
func targets(command string, operands []string, followDest bool) ([][2]string, error) {
	if len(operands) < 2 {
		return nil, usageErr("%s: missing destination operand", command)
	}
	sources, dest := operands[:len(operands)-1], operands[len(operands)-1]
	stat := os.Stat
	if !followDest {
		stat = os.Lstat
	}
	info, err := stat(dest)
	isDir := err == nil && info.IsDir()
	if len(sources) > 1 && !isDir {
		return nil, usageErr("%s: target %q is not a directory", command, dest)
	}
	pairs := make([][2]string, 0, len(sources))
	for _, src := range sources {
		to := dest
		if isDir {
			to = filepath.Join(dest, pathutil.BaseName(src, ""))
		}
		pairs = append(pairs, [2]string{src, to})
	}
	return pairs, nil
}

func runPairs(ctx context.Context, command string, pairs [][2]string, fn func(src, dst string) error) error {
	var errs []error
	for _, p := range pairs {
		if err := ctx.Err(); err != nil {
			return errors.Join(append(errs, err)...)
		}
		if err := fn(p[0], p[1]); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", command, err))
		}
	}
	return errors.Join(errs...)
}

const cpHelp = `Usage: uniz cp [-pRr] [--] source... destination
  -r, -R  copy directories recursively; symlinks are copied as links
  -p      preserve permission bits and modification times
If destination is an existing directory, sources are copied into it.
Copying a file onto itself or a directory into itself is refused.
Existing files are overwritten without prompting. Only regular files,
directories, and symlinks are supported; ownership is not preserved.
`

func runCp(ctx context.Context, _ stdio, args []string) error {
	o, err := parseOptions("cp", args, "pRr", "")
	if err != nil {
		return err
	}
	pairs, err := targets("cp", o.operands, true)
	if err != nil {
		return err
	}
	opts := fileops.CopyOptions{Recursive: o.set['r'] || o.set['R'], Preserve: o.set['p']}
	return runPairs(ctx, "cp", pairs, func(src, dst string) error { return fileops.Copy(ctx, src, dst, opts) })
}

const mvHelp = `Usage: uniz mv [-f] [--] source... destination
  -f  accepted for compatibility; mv never prompts
If destination is an existing directory, sources are moved into it.
Across filesystems, mv copies (preserving modes, times, and symlinks) and
then removes the source; on copy failure the source is kept.
`

func runMv(ctx context.Context, _ stdio, args []string) error {
	o, err := parseOptions("mv", args, "f", "")
	if err != nil {
		return err
	}
	pairs, err := targets("mv", o.operands, true)
	if err != nil {
		return err
	}
	return runPairs(ctx, "mv", pairs, func(src, dst string) error { return fileops.Move(ctx, src, dst) })
}

const lnHelp = `Usage: uniz ln [-fns] [--] target... [link | directory]
  -s  create symbolic links instead of hard links
  -f  atomically replace existing non-directory links
  -n  treat a destination symlink to a directory as a file
With one operand, the link is created in the current directory. If the last
operand is an existing directory, links are created inside it.
`

func runLn(ctx context.Context, _ stdio, args []string) error {
	o, err := parseOptions("ln", args, "fns", "")
	if err != nil {
		return err
	}
	operands := o.operands
	if len(operands) == 1 {
		operands = append(operands, ".")
	}
	pairs, err := targets("ln", operands, !o.set['n'])
	if err != nil {
		return err
	}
	opts := fileops.LinkOptions{Symbolic: o.set['s'], Force: o.set['f']}
	return runPairs(ctx, "ln", pairs, func(target, link string) error { return fileops.Link(ctx, target, link, opts) })
}

const realpathHelp = `Usage: uniz realpath [--] paths...
Print each path as an absolute path with symlinks, '.', and '..' resolved.
Every path component must exist.
`
const readlinkHelp = `Usage: uniz readlink [-f] [--] links...
  -f  print the fully resolved absolute path instead (like realpath)
Without -f, each operand must be a symbolic link.
`

func runResolve(ctx context.Context, s stdio, command string, operands []string, resolve func(string) (string, error)) error {
	if len(operands) == 0 {
		return usageErr("%s: missing operand", command)
	}
	out := &outputWriter{w: s.out}
	err := forEach(ctx, command, operands, func(p string) error {
		if out.err != nil {
			return nil
		}
		resolved, err := resolve(p)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(out, resolved)
		return err
	})
	if out.err != nil {
		return errors.Join(err, out.err)
	}
	return err
}

func runRealpath(ctx context.Context, s stdio, args []string) error {
	o, err := parseOptions("realpath", args, "", "")
	if err != nil {
		return err
	}
	return runResolve(ctx, s, "realpath", o.operands, fileops.RealPath)
}

func runReadlink(ctx context.Context, s stdio, args []string) error {
	o, err := parseOptions("readlink", args, "f", "")
	if err != nil {
		return err
	}
	resolve := os.Readlink
	if o.set['f'] {
		resolve = fileops.RealPath
	}
	return runResolve(ctx, s, "readlink", o.operands, resolve)
}
