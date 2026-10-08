package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/pkar/uniz/internal/fileops"
	"github.com/pkar/uniz/internal/search"
	"github.com/pkar/uniz/internal/sysinfo"
)

func registerSystemTools() {
	add := func(name, summary, usage string, literal bool, run func(context.Context, stdio, []string) error) {
		registry[name] = command{summary, usage, literal, run}
	}
	add("grep", "print lines that match patterns", grepHelp, false, runGrep)
	add("find", "search for files in a directory hierarchy", findHelp, false, runFind)
	add("chmod", "change file mode bits", chmodHelp, false, runChmod)
	add("mktemp", "create a unique temporary file or directory", mktempHelp, false, runMktemp)
	add("truncate", "shrink or extend files to a size", truncateHelp, false, runTruncate)
	add("link", "create one hard link (no options)", linkHelp, false, runLink)
	add("unlink", "remove one non-directory (no options)", unlinkHelp, false, runUnlink)
	add("nproc", "print the number of CPUs", nprocHelp, false, runNproc)
	add("uname", "print system information", unameHelp, false, runUname)
	add("id", "print user and group IDs", idHelp, false, runID)
	add("groups", "print group names", groupsHelp, false, runGroups)
	add("env", "run a program in a modified environment", envHelp, false, runEnv)
	add("xargs", "build and run commands from stdin", xargsHelp, false, runXargs)
	add("timeout", "run a program with a time limit", timeoutHelp, false, runTimeout)
}

const grepHelp = `Usage: uniz grep [-EFGHLRchilnoqrsvwxa] [-e PATTERN]... [-f FILE]... [-m NUM] [PATTERN] [--] [files...]
  -G  basic regular expressions (default)    -E  extended regular expressions
  -F  fixed strings                          -e  add a pattern; -f  read patterns from FILE
  -i  ignore case                            -v  select non-matching lines
  -w  match whole words                      -x  match whole lines
  -c  print a count per file                 -l / -L  list files with / without matches
  -n  print line numbers                     -o  print only the matching parts
  -H / -h  always / never print file names   -q  quiet; exit 0 on the first match
  -s  suppress messages about unreadable files
  -r, -R  search directories recursively (symlinks below operands are not followed)
  -m NUM  stop after NUM selected lines      -a  treat binary files as text
Exit status: 0 if a line is selected, 1 if none, 2 on an error (unless -q
matched). Matching uses Go's RE2 engine with leftmost-longest semantics:
back-references are not supported. Context options (-A, -B, -C) are not
supported. Files with a NUL byte in the first 8 KiB print "Binary file NAME matches".
`

func runGrep(ctx context.Context, s stdio, args []string) error {
	o, err := parseOptions("grep", args, "EFGHLRachilnoqrsvwxy", "efm")
	if err != nil {
		return err
	}
	patterns := append([]string(nil), o.lists['e']...)
	for _, name := range o.lists['f'] {
		data, err := os.ReadFile(name)
		if err != nil {
			return &ExitError{Code: 2, Err: fmt.Errorf("grep: %w", err)}
		}
		text := strings.TrimSuffix(string(data), "\n")
		if text != "" || len(data) > 0 {
			patterns = append(patterns, strings.Split(text, "\n")...)
		}
	}
	operands := o.operands
	if !o.set['e'] && !o.set['f'] {
		if len(operands) == 0 {
			return usageErr("grep: missing pattern")
		}
		patterns, operands = strings.Split(operands[0], "\n"), operands[1:]
	}
	popts := search.PatternOptions{IgnoreCase: o.set['i'] || o.set['y'], Word: o.set['w'], Line: o.set['x']}
	switch {
	case o.set['F']:
		popts.Syntax = search.Fixed
	case o.set['E']:
		popts.Syntax = search.Extended
	}
	m, err := search.Compile(patterns, popts)
	if err != nil {
		return &ExitError{Code: 2, Err: fmt.Errorf("grep: %w", err)}
	}
	recursive := o.set['r'] || o.set['R']
	implicit := len(operands) == 0
	if implicit {
		operands = []string{"-"}
		if recursive {
			operands = []string{"."}
		}
	}
	gopts := search.GrepOptions{
		Invert: o.set['v'], Count: o.set['c'], ListMatches: o.set['l'], ListMissing: o.set['L'],
		LineNumbers: o.set['n'], OnlyMatching: o.set['o'], Quiet: o.set['q'], Binary: !o.set['a'],
		WithName: (len(operands) > 1 || recursive) && !o.set['h'] || o.set['H'],
	}
	if v, ok := o.values['m']; ok {
		if gopts.MaxCount, err = strconv.ParseInt(v, 10, 64); err != nil || gopts.MaxCount < 0 {
			return usageErr("grep: invalid max count %q", v)
		}
		if gopts.MaxCount == 0 {
			return &ExitError{Code: 1}
		}
	}
	out := &outputWriter{w: s.out}
	var errs []error
	matched := false
	errDone := errors.New("done")
	searchOne := func(name string, r io.Reader) error {
		opts := gopts
		opts.Name = name
		n, err := search.Grep(ctx, out, r, m, opts)
		if n > 0 {
			matched = true
		}
		if err != nil {
			if out.err != nil || ctx.Err() != nil {
				return err
			}
			errs = append(errs, fmt.Errorf("grep: %s: %w", name, err))
		}
		if matched && opts.Quiet {
			return errDone
		}
		return nil
	}
	searchFile := func(path, display string) error {
		f, err := os.Open(path)
		if err != nil {
			errs = append(errs, fmt.Errorf("grep: %w", err))
			return nil
		}
		defer f.Close()
		return searchOne(display, f)
	}
	var runErr error
	for _, op := range operands {
		if op == "-" {
			if runErr = searchOne("(standard input)", s.in); runErr != nil {
				break
			}
			continue
		}
		info, err := os.Stat(op)
		if err != nil {
			errs = append(errs, fmt.Errorf("grep: %w", err))
			continue
		}
		if !info.IsDir() {
			if runErr = searchFile(op, op); runErr != nil {
				break
			}
			continue
		}
		if !recursive {
			errs = append(errs, fmt.Errorf("grep: %s: %w", op, fileops.ErrIsDirectory))
			continue
		}
		runErr = filepath.WalkDir(op, func(p string, d fs.DirEntry, err error) error {
			if cerr := ctx.Err(); cerr != nil {
				return cerr
			}
			if err != nil {
				errs = append(errs, fmt.Errorf("grep: %w", err))
				return nil
			}
			if !d.Type().IsRegular() {
				return nil
			}
			display := p
			if implicit {
				display = strings.TrimPrefix(p, "."+string(filepath.Separator))
			}
			return searchFile(p, display)
		})
		if runErr != nil {
			break
		}
	}
	switch {
	case runErr == errDone:
		return nil
	case runErr != nil:
		return errors.Join(append(errs, runErr)...)
	case len(errs) > 0 && !(o.set['q'] && matched):
		if o.set['s'] {
			return &ExitError{Code: 2}
		}
		return &ExitError{Code: 2, Err: errors.Join(errs...)}
	case matched:
		return nil
	}
	return &ExitError{Code: 1}
}

const findHelp = `Usage: uniz find [paths...] [expression]
Walk each path (default ".") in name order without following symlinks and
print entries that match the expression.
  Tests:   -name PAT, -iname PAT, -path PAT, -ipath PAT (shell globs),
           -type [fdlpscb] (comma lists allowed), -size [+-]N[cbkMG],
           -mtime [+-]DAYS, -mmin [+-]MINUTES, -newer FILE, -empty, -true, -false
  Actions: -print, -print0, -prune, -quit
  Options: -maxdepth N, -mindepth N
  Logic:   ( EXPR ), ! EXPR, -not, EXPR [-a] EXPR, EXPR -o EXPR
Without an action, matching entries are printed. -exec and -delete are not
supported; pipe -print0 to "uniz xargs -0" instead.
`

func runFind(ctx context.Context, s stdio, args []string) error {
	i := 0
	for i < len(args) && !strings.HasPrefix(args[i], "-") && args[i] != "(" && args[i] != "!" && args[i] != ")" {
		i++
	}
	roots, exprArgs := args[:i], args[i:]
	if len(roots) == 0 {
		roots = []string{"."}
	}
	expr, err := search.ParseExpression(exprArgs, time.Now())
	if err != nil {
		return usageErr("find: %v", err)
	}
	out := bufio.NewWriter(&outputWriter{w: s.out})
	err = expr.Walk(ctx, roots, func(path string, term byte) error {
		if _, err := out.WriteString(path); err != nil {
			return err
		}
		return out.WriteByte(term)
	})
	if ferr := out.Flush(); ferr != nil {
		return errors.Join(err, ferr)
	}
	if err != nil {
		return fmt.Errorf("find: %w", err)
	}
	return nil
}

const chmodHelp = `Usage: uniz chmod [-R] MODE[,MODE]... [--] files...
  -R  change directories and their contents; symlinks inside are skipped
MODE is octal (644, 4755) or symbolic: [ugoa]*([-+=]([rwxXst]*|[ugo]))+,
for example u+x, go-w, a=rX, g=u. Without u/g/o/a the change applies to all
classes and the umask is not consulted. Symlink operands are followed.
`

func runChmod(ctx context.Context, _ stdio, args []string) error {
	var change fileops.ModeChange
	var files []string
	recursive, literal := false, false
	for _, a := range args {
		option := !literal && strings.HasPrefix(a, "-") && a != "-"
		switch {
		case option && a == "--":
			literal = true
		case option && a == "-R":
			recursive = true
		case change == nil: // the mode may itself start with '-', as in chmod -w file
			c, err := fileops.ParseMode(a)
			if err != nil && option {
				return usageErr("chmod: unknown option %q", a)
			}
			if err != nil {
				return usageErr("chmod: %v", err)
			}
			change = c
		case option:
			return usageErr("chmod: unknown option %q", a)
		default:
			files = append(files, a)
		}
	}
	if change == nil || len(files) == 0 {
		return usageErr("chmod: missing operand")
	}
	return forEach(ctx, "chmod", files, func(p string) error {
		return fileops.Chmod(ctx, p, change, fileops.ChmodOptions{Recursive: recursive})
	})
}

const mktempHelp = `Usage: uniz mktemp [-d] [-p DIR | -t] [TEMPLATE]
  -d      create a directory (mode 0700) instead of a file (mode 0600)
  -p DIR  create inside DIR
  -t      create inside $TMPDIR (or the system temporary directory)
TEMPLATE ends in at least 3 X characters (default tmp.XXXXXXXXXX, created in
the temporary directory). Without -p or -t, a given TEMPLATE is relative to
the current directory. Prints the created path. -u is not supported.
`

func runMktemp(_ context.Context, s stdio, args []string) error {
	o, err := parseOptions("mktemp", args, "dt", "p")
	if err != nil {
		return err
	}
	if len(o.operands) > 1 {
		return usageErr("mktemp: too many templates")
	}
	template, dir := "tmp.XXXXXXXXXX", os.TempDir()
	if len(o.operands) == 1 {
		template, dir = o.operands[0], ""
	}
	if o.set['t'] {
		dir = os.TempDir()
	}
	if v, ok := o.values['p']; ok {
		dir = v
	}
	if dir != "" && len(o.operands) == 1 && strings.ContainsAny(template, `/`+string(filepath.Separator)) {
		return usageErr("mktemp: template %q must not contain a directory with -p or -t", template)
	}
	path, err := fileops.MakeTemp(dir, template, o.set['d'])
	if err != nil {
		return fmt.Errorf("mktemp: %w", err)
	}
	_, err = fmt.Fprintln(s.out, path)
	return err
}

const truncateHelp = `Usage: uniz truncate [-c] -s SIZE [--] files...
  -c       do not create missing files
  -s SIZE  set the size; a prefix changes it relative to the current size:
           +N grow, -N shrink, <N at most, >N at least, /N round down,
           %N round up to a multiple of N
Suffixes: K M G T P E (powers of 1024, also KiB...) or KB MB GB... (powers
of 1000). Extending adds zero bytes. Reference files (-r) are not supported.
`

func runTruncate(ctx context.Context, _ stdio, args []string) error {
	o, err := parseOptions("truncate", args, "c", "s")
	if err != nil {
		return err
	}
	v, ok := o.values['s']
	if !ok {
		return usageErr("truncate: -s SIZE is required")
	}
	change, err := fileops.ParseSize(v)
	if err != nil {
		return usageErr("truncate: %v", err)
	}
	if len(o.operands) == 0 {
		return usageErr("truncate: missing file operand")
	}
	return forEach(ctx, "truncate", o.operands, func(p string) error {
		return fileops.Truncate(ctx, p, change, fileops.TruncateOptions{NoCreate: o.set['c']})
	})
}

const linkHelp = "Usage: uniz link FILE NEWNAME\nCreate exactly one hard link with link(2). Use ln for options.\n"
const unlinkHelp = "Usage: uniz unlink FILE\nRemove exactly one file or symlink with unlink(2). Directories are refused.\n"

func exactOperands(command string, args []string, n int) ([]string, error) {
	o, err := parseOptions(command, args, "", "")
	if err == nil && len(o.operands) != n {
		err = usageErr("%s: expected %d operand(s), got %d", command, n, len(o.operands))
	}
	return o.operands, err
}

func runLink(ctx context.Context, _ stdio, args []string) error {
	ops, err := exactOperands("link", args, 2)
	if err != nil {
		return err
	}
	if err := fileops.Link(ctx, ops[0], ops[1], fileops.LinkOptions{}); err != nil {
		return fmt.Errorf("link: %w", err)
	}
	return nil
}

func runUnlink(_ context.Context, _ stdio, args []string) error {
	ops, err := exactOperands("unlink", args, 1)
	if err != nil {
		return err
	}
	if err := fileops.Unlink(ops[0]); err != nil {
		return fmt.Errorf("unlink: %w", err)
	}
	return nil
}

const nprocHelp = "Usage: uniz nproc\nPrint the number of logical CPUs usable by this process.\n"

func runNproc(_ context.Context, s stdio, args []string) error {
	if err := noOperands("nproc", args); err != nil {
		return err
	}
	_, err := fmt.Fprintln(s.out, runtime.NumCPU())
	return err
}

const unameHelp = `Usage: uniz uname [-amnrsv]
  -s  kernel name (default)   -n  network host name   -r  kernel release
  -v  kernel version          -m  machine hardware    -a  all of the above
Fields print in the order s n r v m.
`

func runUname(_ context.Context, s stdio, args []string) error {
	o, err := parseOptions("uname", args, "amnrsv", "")
	if err != nil {
		return err
	}
	if len(o.operands) > 0 {
		return usageErr("uname: extra operand %q", o.operands[0])
	}
	info, err := sysinfo.Uname()
	if err != nil {
		return fmt.Errorf("uname: %w", err)
	}
	var fields []string
	for _, f := range []struct {
		flag  rune
		value string
	}{{'s', info.Sysname}, {'n', info.Nodename}, {'r', info.Release}, {'v', info.Version}, {'m', info.Machine}} {
		if o.set[f.flag] || o.set['a'] || f.flag == 's' && len(o.set) == 0 {
			fields = append(fields, f.value)
		}
	}
	_, err = fmt.Fprintln(s.out, strings.Join(fields, " "))
	return err
}

func lookupUser(command string, operands []string) (*user.User, error) {
	switch len(operands) {
	case 0:
		u, err := user.Current()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", command, err)
		}
		return u, nil
	case 1:
		u, err := user.Lookup(operands[0])
		if err != nil {
			if _, numErr := strconv.Atoi(operands[0]); numErr == nil {
				u, err = user.LookupId(operands[0])
			}
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", command, err)
		}
		return u, nil
	}
	return nil, usageErr("%s: extra operand %q", command, operands[1])
}

func groupName(gid string) string {
	if g, err := user.LookupGroupId(gid); err == nil {
		return g.Name
	}
	return gid
}

// userGroups returns the primary group followed by the other groups.
func userGroups(u *user.User) ([]string, error) {
	gids, err := u.GroupIds()
	out := []string{u.Gid}
	for _, g := range gids {
		if g != u.Gid {
			out = append(out, g)
		}
	}
	return out, err
}

const idHelp = `Usage: uniz id [-G | -g | -u] [-n] [user]
  -u  print only the user ID     -g  print only the primary group ID
  -G  print all group IDs        -n  print names instead of numbers
Without options prints "uid=N(name) gid=N(name) groups=N(name),...".
IDs come from the user database; effective and real IDs are not distinguished.
`

func runID(_ context.Context, s stdio, args []string) error {
	o, err := parseOptions("id", args, "Ggnur", "")
	if err != nil {
		return err
	}
	modes := 0
	for _, f := range "Ggu" {
		if o.set[f] {
			modes++
		}
	}
	if modes > 1 {
		return usageErr("id: -G, -g, and -u are mutually exclusive")
	}
	if o.set['n'] && modes == 0 {
		return usageErr("id: -n requires -G, -g, or -u")
	}
	u, err := lookupUser("id", o.operands)
	if err != nil {
		return err
	}
	groups, gerr := userGroups(u)
	if gerr != nil {
		gerr = fmt.Errorf("id: %w", gerr)
	}
	show := func(id, name string) string {
		if o.set['n'] {
			return name
		}
		return id
	}
	var line string
	switch {
	case o.set['u']:
		line = show(u.Uid, u.Username)
	case o.set['g']:
		line = show(u.Gid, groupName(u.Gid))
	case o.set['G']:
		parts := make([]string, len(groups))
		for i, g := range groups {
			parts[i] = show(g, groupName(g))
		}
		line = strings.Join(parts, " ")
	default:
		parts := make([]string, len(groups))
		for i, g := range groups {
			parts[i] = fmt.Sprintf("%s(%s)", g, groupName(g))
		}
		line = fmt.Sprintf("uid=%s(%s) gid=%s(%s) groups=%s", u.Uid, u.Username, u.Gid, groupName(u.Gid), strings.Join(parts, ","))
	}
	if _, err := fmt.Fprintln(s.out, line); err != nil {
		return err
	}
	return gerr
}

const groupsHelp = "Usage: uniz groups [user]\nPrint the names of the groups a user (default: the current user) belongs to.\n"

func runGroups(_ context.Context, s stdio, args []string) error {
	o, err := parseOptions("groups", args, "", "")
	if err != nil {
		return err
	}
	u, err := lookupUser("groups", o.operands)
	if err != nil {
		return err
	}
	groups, gerr := userGroups(u)
	names := make([]string, len(groups))
	for i, g := range groups {
		names[i] = groupName(g)
	}
	if _, err := fmt.Fprintln(s.out, strings.Join(names, " ")); err != nil {
		return err
	}
	if gerr != nil {
		return fmt.Errorf("groups: %w", gerr)
	}
	return nil
}

const envHelp = `Usage: uniz env [-i] [-u NAME]... [--] [NAME=VALUE]... [program [args...]]
  -i, -   start with an empty environment
  -u NAME remove NAME from the environment
Without a program, print the resulting environment. Otherwise run program
(found on the resulting PATH) with stdin, stdout, and stderr passed through.
Exit status is the program's, 127 if it is not found, 126 if it cannot run.
`

func runEnv(ctx context.Context, s stdio, args []string) error {
	env := os.Environ()
	i := 0
	for ; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			i++
			break
		}
		if a == "-" || a == "-i" {
			env = []string{}
			continue
		}
		if a == "-u" || strings.HasPrefix(a, "-u") {
			name := strings.TrimPrefix(a, "-u")
			if name == "" {
				if i+1 >= len(args) {
					return usageErr("env: option -u requires a value")
				}
				i++
				name = args[i]
			}
			if name == "" || strings.Contains(name, "=") {
				return usageErr("env: cannot unset %q", name)
			}
			kept := env[:0:0]
			for _, kv := range env {
				if k, _, _ := strings.Cut(kv, "="); k != name {
					kept = append(kept, kv)
				}
			}
			env = kept
			continue
		}
		if strings.HasPrefix(a, "-") {
			return usageErr("env: unknown option %q", a)
		}
		break
	}
	for ; i < len(args) && strings.Contains(args[i], "=") && !strings.HasPrefix(args[i], "="); i++ {
		k, _, _ := strings.Cut(args[i], "=")
		kept := env[:0:0]
		for _, kv := range env {
			if key, _, _ := strings.Cut(kv, "="); key != k {
				kept = append(kept, kv)
			}
		}
		env = append(kept, args[i])
	}
	if i == len(args) {
		out := &outputWriter{w: s.out}
		for _, kv := range env {
			if _, err := fmt.Fprintln(out, kv); err != nil {
				return err
			}
		}
		return nil
	}
	return process{argv: args[i:], env: env, stdin: s.in}.run(ctx, s, "env")
}

const xargsHelp = `Usage: uniz xargs [-0rt] [-n MAX] [-I REPLACE] [--] [program [initial-args...]]
  -0          input items are separated by NUL bytes; no quote processing
  -n MAX      use at most MAX items per command
  -I REPLACE  run once per input line, replacing REPLACE in initial-args
  -r          do not run the program if the input is empty
  -t          print each command to stderr before running it
Items are separated by blanks and newlines; single quotes, double quotes, and
backslashes escape them. The default program is uniz's built-in echo.
Commands are limited to about 128 KiB of arguments. Programs get the null
device as stdin. Exit status: 123 if any command exits 1-125, 124 if one
exits 255 (xargs stops), 125 if one is killed by a signal, 126/127 if the
program cannot run or is not found.
`

func runXargs(ctx context.Context, s stdio, args []string) error {
	nul, noEmpty, trace := false, false, false
	maxArgs, replace := 0, ""
	i := 0
	for ; i < len(args) && strings.HasPrefix(args[i], "-") && args[i] != "-"; i++ {
		a := args[i]
		if a == "--" {
			i++
			break
		}
		value := func(flag string) (string, error) {
			if v := strings.TrimPrefix(a, flag); v != "" {
				return v, nil
			}
			if i+1 >= len(args) {
				return "", usageErr("xargs: option %s requires a value", flag)
			}
			i++
			return args[i], nil
		}
		switch {
		case strings.HasPrefix(a, "-n"):
			v, err := value("-n")
			if err != nil {
				return err
			}
			if maxArgs, err = strconv.Atoi(v); err != nil || maxArgs < 1 {
				return usageErr("xargs: invalid number %q for -n", v)
			}
		case strings.HasPrefix(a, "-I"):
			v, err := value("-I")
			if err != nil {
				return err
			}
			replace = v
		default:
			for _, f := range a[1:] {
				switch f {
				case '0':
					nul = true
				case 'r':
					noEmpty = true
				case 't':
					trace = true
				default:
					return usageErr("xargs: unknown option %q", a)
				}
			}
		}
	}
	program := args[i:]
	if len(program) == 0 {
		program = []string{"echo"}
	}
	builtinEcho := i == len(args)
	items, err := xargsItems(s.in, nul, replace != "")
	if err != nil {
		return fmt.Errorf("xargs: %w", err)
	}
	var batches [][]string
	switch {
	case replace != "":
		for _, item := range items {
			argv := make([]string, len(program))
			for j, a := range program {
				argv[j] = strings.ReplaceAll(a, replace, item)
			}
			batches = append(batches, argv)
		}
	case len(items) == 0:
		if !noEmpty {
			batches = append(batches, program)
		}
	default:
		const limit = 128 * 1024
		base := 0
		for _, a := range program {
			base += len(a) + 1
		}
		var cur []string
		size := base
		for _, item := range items {
			if len(cur) > 0 && (maxArgs > 0 && len(cur) >= maxArgs || size+len(item)+1 > limit) {
				batches = append(batches, append(append([]string(nil), program...), cur...))
				cur, size = nil, base
			}
			cur = append(cur, item)
			size += len(item) + 1
		}
		batches = append(batches, append(append([]string(nil), program...), cur...))
	}
	status := 0
	for _, argv := range batches {
		if err := ctx.Err(); err != nil {
			return err
		}
		if trace {
			fmt.Fprintln(s.err, strings.Join(argv, " "))
		}
		var err error
		if builtinEcho {
			err = runEcho(ctx, s, argv[1:])
		} else {
			err = process{argv: argv}.run(ctx, s, "xargs")
		}
		var exit *ExitError
		switch {
		case err == nil:
		case errors.As(err, &exit) && exit.Err == nil:
			switch {
			case exit.Code == 255:
				return &ExitError{Code: 124, Err: fmt.Errorf("xargs: %s: exited with status 255; aborting", argv[0])}
			case exit.Code > 128:
				return &ExitError{Code: 125, Err: fmt.Errorf("xargs: %s: terminated by signal %d", argv[0], exit.Code-128)}
			default:
				status = 123
			}
		default:
			return err
		}
	}
	if status != 0 {
		return &ExitError{Code: status}
	}
	return nil
}

// xargsItems splits input into items: NUL-separated, one per line (-I), or
// blank-separated with quotes and backslash escapes.
func xargsItems(r io.Reader, nul, lines bool) ([]string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	in := string(data)
	switch {
	case nul:
		items := strings.Split(in, "\x00")
		if items[len(items)-1] == "" {
			items = items[:len(items)-1]
		}
		return items, nil
	case lines:
		var items []string
		for _, line := range strings.Split(in, "\n") {
			if line = strings.TrimLeft(line, " \t"); line != "" {
				items = append(items, line)
			}
		}
		return items, nil
	}
	var items []string
	var b strings.Builder
	inItem := false
	for i := 0; i < len(in); i++ {
		c := in[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n':
			if inItem {
				items, inItem = append(items, b.String()), false
				b.Reset()
			}
		case c == '\\':
			if i+1 < len(in) {
				i++
				b.WriteByte(in[i])
			}
			inItem = true
		case c == '\'' || c == '"':
			end := strings.IndexByte(in[i+1:], c)
			if end < 0 || strings.Contains(in[i+1:i+1+end], "\n") {
				return nil, fmt.Errorf("unmatched %s quote", map[byte]string{'\'': "single", '"': "double"}[c])
			}
			b.WriteString(in[i+1 : i+1+end])
			i += end + 1
			inItem = true
		default:
			b.WriteByte(c)
			inItem = true
		}
	}
	if inItem {
		items = append(items, b.String())
	}
	return items, nil
}

const timeoutHelp = `Usage: uniz timeout [-k KILL_AFTER] DURATION program [args...]
Run program and send it SIGTERM if it is still running after DURATION
(seconds, or with an s, m, h, or d suffix; 0 disables the limit).
  -k KILL_AFTER  also send SIGKILL if it is still running this long after SIGTERM
Exit status: 124 if the time limit was reached, otherwise the program's
status (126/127 if it cannot run or is not found). On Windows the program
is killed immediately. -s (other signals) is not supported.
`

func runTimeout(ctx context.Context, s stdio, args []string) error {
	var killAfter time.Duration
	i := 0
	for ; i < len(args) && strings.HasPrefix(args[i], "-") && len(args[i]) > 1; i++ {
		a := args[i]
		if a == "--" {
			i++
			break
		}
		if !strings.HasPrefix(a, "-k") {
			return usageErr("timeout: unknown option %q", a)
		}
		v := strings.TrimPrefix(a, "-k")
		if v == "" {
			if i+1 >= len(args) {
				return usageErr("timeout: option -k requires a value")
			}
			i++
			v = args[i]
		}
		d, err := parseInterval("timeout", v)
		if err != nil {
			return err
		}
		killAfter = d
	}
	if len(args)-i < 2 {
		return usageErr("timeout: expected a duration and a program")
	}
	limit, err := parseInterval("timeout", args[i])
	if err != nil {
		return err
	}
	runCtx := ctx
	if limit > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, limit)
		defer cancel()
	}
	p := process{argv: args[i+1:], stdin: s.in}
	stopKill := func() bool { return false }
	p.cancel = func(cmd *exec.Cmd) error {
		if ctx.Err() != nil { // the caller canceled: kill outright
			return cmd.Process.Kill()
		}
		if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
			return cmd.Process.Kill()
		}
		// A program that ignores SIGTERM is still killed if the caller
		// cancels later. Wait observes this function's result, so stopKill
		// is safely visible after run returns.
		stopKill = context.AfterFunc(ctx, func() { cmd.Process.Kill() })
		return nil
	}
	if killAfter > 0 {
		p.delay = func(cmd *exec.Cmd) { cmd.WaitDelay = killAfter }
	}
	err = p.run(runCtx, s, "timeout")
	stopKill()
	if ctx.Err() == nil && errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		return &ExitError{Code: 124}
	}
	return err
}
