package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pkar/uniz/internal/expr"
	"github.com/pkar/uniz/internal/fileops"
	"github.com/pkar/uniz/internal/ls"
	"github.com/pkar/uniz/internal/pathutil"
	"github.com/pkar/uniz/internal/sysinfo"
)

func registerMoreSystem() {
	add := func(name, summary, usage string, literal bool, run func(context.Context, stdio, []string) error) {
		registry[name] = command{summary, usage, literal, run}
	}
	add("stat", "print file metadata", statHelp, false, runStat)
	add("du", "estimate disk usage", duHelp, false, runDu)
	add("df", "report filesystem space", dfHelp, false, runDf)
	add("install", "copy files and set their mode", installHelp, false, runInstall)
	add("shred", "overwrite files to hide their contents", shredHelp, false, runShred)
	add("sync", "flush cached writes to storage", syncHelp, false, runSync)
	add("mkfifo", "create named pipes", mkfifoHelp, false, runMkfifo)
	add("expr", "evaluate an expression", exprHelp, true, runExpr)
	add("tty", "print the terminal on stdin", ttyHelp, false, runTty)
	add("logname", "print the login name", lognameHelp, false, runLogname)
	add("pathchk", "check that file names are valid and portable", pathchkHelp, false, runPathchk)
	add("nice", "run a program with changed niceness", niceHelp, true, runNice)
	add("nohup", "run a program immune to terminal hangups", nohupHelp, true, runNohup)
	add("kill", "send a signal to processes", killHelp, true, runKill)
	add("time", "time a program", timeHelp, true, runTime)
}

const statHelp = `Usage: uniz stat [-L] [-c FORMAT] [--] files...
  -L         follow symbolic links
  -c FORMAT  print FORMAT and a newline for each file instead of the default
FORMAT directives: %n %N name; %s size; %b blocks of %B (512) bytes; %o I/O
block; %f raw mode (hex); %F type; %a octal mode; %A mode string; %u %U %g %G
owner and group; %h links; %i inode; %d device; %x %y %z %w access, modify,
change, birth time; %X %Y %Z %W the same as epoch seconds; %%. A width and
the - or 0 flag may precede the letter. Fields the platform lacks print "?".
`

func runStat(ctx context.Context, s stdio, args []string) error {
	o, err := parseOptions("stat", args, "L", "c")
	if err != nil {
		return err
	}
	if len(o.operands) == 0 {
		return usageErr("stat: missing operand")
	}
	format := fileops.DefaultStatFormat
	if v, ok := o.values['c']; ok {
		format = v + "\n"
	}
	out := &outputWriter{w: s.out}
	return forEach(ctx, "stat", o.operands, func(p string) error {
		si, err := fileops.Stat(p, o.set['L'])
		if err != nil {
			return err
		}
		_, err = io.WriteString(out, fileops.FormatStat(si, format))
		return err
	})
}

const duHelp = `Usage: uniz du [-abchks] [-d DEPTH] [--] [paths...]
Print the space used by each directory (default "."), children first.
  -a        also print files
  -s        print only a total for each operand (same as -d 0)
  -d DEPTH  print entries at most DEPTH levels below an operand
  -c        print a grand total
  -k        sizes in 1024-byte blocks, rounded up (the default)
  -h        sizes like 1.1K, 15M
  -b        apparent sizes in bytes rather than allocated space
Symbolic links are not followed and hard-linked files are counted once.
`

func runDu(ctx context.Context, s stdio, args []string) error {
	o, err := parseOptions("du", args, "abchks", "d")
	if err != nil {
		return err
	}
	opts := fileops.DuOptions{All: o.set['a'], MaxDepth: -1, Apparent: o.set['b']}
	if v, ok := o.values['d']; ok {
		if opts.MaxDepth, err = strconv.Atoi(v); err != nil || opts.MaxDepth < 0 {
			return usageErr("du: invalid depth %q", v)
		}
	}
	if o.set['s'] {
		if o.set['a'] || o.set['d'] {
			return usageErr("du: -s cannot be combined with -a or -d")
		}
		opts.MaxDepth = 0
	}
	size := func(n int64) string {
		switch {
		case o.set['h']:
			return ls.HumanSize(n)
		case o.set['b'] && !o.set['k']:
			return strconv.FormatInt(n, 10)
		}
		return strconv.FormatInt((n+1023)/1024, 10)
	}
	out := &outputWriter{w: s.out}
	var grand int64
	operands := o.operands
	if len(operands) == 0 {
		operands = []string{"."}
	}
	err = forEach(ctx, "du", operands, func(p string) error {
		n, err := fileops.DiskUsage(ctx, p, opts, func(e fileops.DuEntry) error {
			_, err := fmt.Fprintf(out, "%s\t%s\n", size(e.Bytes), e.Path)
			return err
		})
		grand += n
		return err
	})
	if o.set['c'] && out.err == nil {
		fmt.Fprintf(out, "%s\ttotal\n", size(grand))
	}
	return errors.Join(err, out.err)
}

const dfHelp = `Usage: uniz df [-hkPT] [--] [paths...]
Report size, used, and available space of the filesystems holding paths, or
of every mounted filesystem with a non-zero size.
  -k  sizes in 1024-byte blocks (the default)
  -h  sizes like 1.1G
  -P  POSIX output format
  -T  include the filesystem type
Not supported on Windows.
`

func runDf(ctx context.Context, s stdio, args []string) error {
	o, err := parseOptions("df", args, "hkPT", "")
	if err != nil {
		return err
	}
	var rows []sysinfo.FSUsage
	if len(o.operands) == 0 {
		if rows, err = sysinfo.Mounts(); err != nil {
			return fmt.Errorf("df: %w", err)
		}
	} else {
		err = forEach(ctx, "df", o.operands, func(p string) error {
			u, err := sysinfo.DiskFree(p)
			if err != nil {
				return fmt.Errorf("%s: %w", p, err)
			}
			rows = append(rows, u)
			return nil
		})
	}
	human := o.set['h'] && !o.set['P']
	size := func(n uint64) string {
		if human {
			return ls.HumanSize(int64(min(n, 1<<62)))
		}
		return strconv.FormatUint((n+1023)/1024, 10)
	}
	header := []string{"Filesystem", "1K-blocks", "Used", "Available", "Use%", "Mounted on"}
	switch {
	case human:
		header = []string{"Filesystem", "Size", "Used", "Avail", "Use%", "Mounted on"}
	case o.set['P']:
		header = []string{"Filesystem", "1024-blocks", "Used", "Available", "Capacity", "Mounted on"}
	}
	if o.set['T'] {
		header = append(header[:1], append([]string{"Type"}, header[1:]...)...)
	}
	table := [][]string{header}
	for _, r := range rows {
		pct := "-"
		if used := r.Used(); used+r.Avail > 0 {
			pct = strconv.FormatUint((used*100+used+r.Avail-1)/(used+r.Avail), 10) + "%"
		}
		cells := []string{r.Filesystem, size(r.Total), size(r.Used()), size(r.Avail), pct, r.MountPoint}
		if o.set['T'] {
			cells = append(cells[:1], append([]string{r.Type}, cells[1:]...)...)
		}
		table = append(table, cells)
	}
	// Left-align text columns (name, type, mount point); right-align numbers.
	widths := make([]int, len(header))
	for _, row := range table {
		for i, c := range row {
			widths[i] = max(widths[i], len(c))
		}
	}
	left := func(i int) bool { return i == 0 || i == len(header)-1 || o.set['T'] && i == 1 }
	out := &outputWriter{w: s.out}
	for _, row := range table {
		var b strings.Builder
		for i, c := range row {
			if i > 0 {
				b.WriteByte(' ')
			}
			pad := strings.Repeat(" ", widths[i]-len(c))
			switch {
			case i == len(row)-1:
				b.WriteString(c)
			case left(i):
				b.WriteString(c + pad)
			default:
				b.WriteString(pad + c)
			}
		}
		fmt.Fprintln(out, b.String())
	}
	return errors.Join(err, out.err)
}

const installHelp = `Usage: uniz install [-Dp] [-m MODE] [--] SOURCE DEST
       uniz install [-p] [-m MODE] [--] SOURCE... DIRECTORY
       uniz install -d [-m MODE] [--] DIRECTORY...
Copy files and set their permissions (default 0755).
  -m MODE  octal or symbolic mode (symbolic modes start from no permissions)
  -D       create missing parent directories of DEST
  -d       create directories (and their parents) instead of copying
  -p       keep the source's modification time
Each file is written to a temporary file and renamed into place, so a
symbolic link at the destination is replaced, not followed. Changing the
owner or group (-o, -g) and stripping (-s) are not supported.
`

func runInstall(ctx context.Context, s stdio, args []string) error {
	o, err := parseOptions("install", args, "cdDp", "m")
	if err != nil {
		return err
	}
	mode := fs.FileMode(0o755)
	if v, ok := o.values['m']; ok {
		change, err := fileops.ParseMode(v)
		if err != nil {
			return usageErr("install: %v", err)
		}
		mode = change(0, o.set['d'])
	}
	if o.set['d'] {
		if len(o.operands) == 0 {
			return usageErr("install: missing directory operand")
		}
		return forEach(ctx, "install", o.operands, func(dir string) error {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
			return os.Chmod(dir, mode)
		})
	}
	if len(o.operands) < 2 {
		return usageErr("install: expected a source and a destination")
	}
	opts := fileops.InstallOptions{Mode: mode, MakeParents: o.set['D'], PreserveTimes: o.set['p']}
	srcs, dst := o.operands[:len(o.operands)-1], o.operands[len(o.operands)-1]
	info, statErr := os.Stat(dst)
	isDir := statErr == nil && info.IsDir()
	if len(srcs) > 1 && !isDir {
		return usageErr("install: target %q is not a directory", dst)
	}
	return forEach(ctx, "install", srcs, func(src string) error {
		target := dst
		if isDir {
			target = filepath.Join(dst, filepath.Base(src))
		}
		return fileops.Install(ctx, src, target, opts)
	})
}

const shredHelp = `Usage: uniz shred [-uz] [-n PASSES] [--] files...
Overwrite regular files in place with random data, syncing after each pass.
  -n PASSES  random passes (default 3)
  -z         finish with a pass of zeros
  -u         truncate and remove each file afterwards
Symbolic links and non-regular files are refused. Journaling and
copy-on-write filesystems (APFS, btrfs, ZFS) and SSDs may keep old copies
that shred cannot reach.
`

func runShred(ctx context.Context, _ stdio, args []string) error {
	o, err := parseOptions("shred", args, "uz", "n")
	if err != nil {
		return err
	}
	if len(o.operands) == 0 {
		return usageErr("shred: missing file operand")
	}
	opts := fileops.ShredOptions{Zero: o.set['z'], Remove: o.set['u']}
	if v, ok := o.values['n']; ok {
		if opts.Passes, err = positiveInt("shred", "pass count", v); err != nil {
			return err
		}
	}
	return forEach(ctx, "shred", o.operands, func(p string) error { return fileops.Shred(ctx, p, opts) })
}

const syncHelp = `Usage: uniz sync [--] [files...]
With no files, ask the kernel to flush all cached writes (not on Windows).
With files, flush only those files.
`

func runSync(ctx context.Context, _ stdio, args []string) error {
	o, err := parseOptions("sync", args, "", "")
	if err != nil {
		return err
	}
	if len(o.operands) == 0 {
		if err := fileops.SyncAll(); err != nil {
			return fmt.Errorf("sync: %w", err)
		}
		return nil
	}
	return forEach(ctx, "sync", o.operands, fileops.SyncFile)
}

const mkfifoHelp = `Usage: uniz mkfifo [-m MODE] [--] names...
Create named pipes (FIFOs). Not supported on Windows.
  -m MODE  octal or symbolic mode, applied exactly (default 0666 less umask)
`

func runMkfifo(ctx context.Context, _ stdio, args []string) error {
	o, err := parseOptions("mkfifo", args, "", "m")
	if err != nil {
		return err
	}
	if len(o.operands) == 0 {
		return usageErr("mkfifo: missing operand")
	}
	mode := fs.FileMode(0o666)
	if v, ok := o.values['m']; ok {
		change, err := fileops.ParseMode(v)
		if err != nil {
			return usageErr("mkfifo: %v", err)
		}
		mode = change(0o666, false)
	}
	return forEach(ctx, "mkfifo", o.operands, func(p string) error {
		return fileops.Mkfifo(p, mode, o.set['m'])
	})
}

const exprHelp = `Usage: uniz expr EXPRESSION
Evaluate EXPRESSION, given as separate arguments, and print the result.
Operators, lowest precedence first:
  A | B      A if it is neither empty nor 0, otherwise B (or 0)
  A & B      A if neither is empty or 0, otherwise 0
  A < B  A <= B  A = B  A != B  A >= B  A > B   numeric when both are integers
  A + B  A - B   A * B  A / B  A % B           64-bit integer arithmetic
  S : RE     anchored basic regexp match: the \(group\) text or the length
  match S RE, substr S POS LEN, index S CHARS, length S, + TOKEN, ( A )
Exit status: 0 if the result is neither empty nor 0, 1 if it is, 2 on error.
Remember to quote shell characters such as * ( ) < > | &.
`

func runExpr(_ context.Context, s stdio, args []string) error {
	v, err := expr.Evaluate(args)
	if err != nil {
		return &ExitError{Code: 2, Err: fmt.Errorf("expr: %w", err)}
	}
	if _, err := fmt.Fprintln(s.out, v); err != nil {
		return &ExitError{Code: 3, Err: fmt.Errorf("expr: %w", err)}
	}
	if expr.IsNull(v) {
		return &ExitError{Code: 1}
	}
	return nil
}

const ttyHelp = `Usage: uniz tty [-s]
Print the device name of the terminal on stdin, or "not a tty" (status 1).
  -s  print nothing; only set the exit status
`

func runTty(_ context.Context, s stdio, args []string) error {
	o, err := parseOptions("tty", args, "s", "")
	if err != nil {
		return err
	}
	if len(o.operands) > 0 {
		return usageErr("tty: extra operand %q", o.operands[0])
	}
	name := ""
	if f, ok := s.in.(*os.File); ok {
		name, err = sysinfo.TerminalName(f)
	} else {
		err = sysinfo.ErrNotTerminal
	}
	if errors.Is(err, sysinfo.ErrNotTerminal) {
		if !o.set['s'] {
			fmt.Fprintln(s.out, "not a tty")
		}
		return &ExitError{Code: 1}
	}
	if err != nil {
		return &ExitError{Code: 2, Err: fmt.Errorf("tty: %w", err)}
	}
	if !o.set['s'] {
		if _, err := fmt.Fprintln(s.out, name); err != nil {
			return err
		}
	}
	return nil
}

const lognameHelp = `Usage: uniz logname
Print the login name from the LOGNAME environment variable, which login and
sshd set. Unlike whoami, this is the user who logged in, even after su.
`

func runLogname(_ context.Context, s stdio, args []string) error {
	o, err := parseOptions("logname", args, "", "")
	if err != nil {
		return err
	}
	if len(o.operands) > 0 {
		return usageErr("logname: extra operand %q", o.operands[0])
	}
	name, err := sysinfo.LoginName()
	if err != nil {
		return fmt.Errorf("logname: %w", err)
	}
	_, err = fmt.Fprintln(s.out, name)
	return err
}

const pathchkHelp = `Usage: uniz pathchk [-pP] [--] names...
Check that names are valid: by default, at most 4095 bytes with components
of at most 255, and no existing non-directory used as a directory.
  -p  check POSIX portability instead: 255 bytes, 14-byte components, and
      only the characters A-Z a-z 0-9 . _ - /
  -P  also reject components that start with '-'
`

func runPathchk(ctx context.Context, _ stdio, args []string) error {
	o, err := parseOptions("pathchk", args, "pP", "")
	if err != nil {
		return err
	}
	if len(o.operands) == 0 {
		return usageErr("pathchk: missing operand")
	}
	opts := pathutil.CheckOptions{Portable: o.set['p'], Extra: o.set['P']}
	return forEach(ctx, "pathchk", o.operands, func(p string) error { return pathutil.CheckPath(p, opts) })
}

const niceHelp = `Usage: uniz nice [-n ADJUSTMENT | -ADJUSTMENT] [program [args...]]
Run program with its niceness raised by ADJUSTMENT (default 10; the result
is kept within -20..19). With no program, print the current niceness.
The change is applied just after the program starts. Lowering niceness needs
privilege; if refused, the program runs at the current niceness, as with
GNU nice, but without a warning. Not supported on Windows.
`

func runNice(ctx context.Context, s stdio, args []string) error {
	adj, i, explicit := 10, 0, false
	parseAdj := func(v string) error {
		n, err := strconv.Atoi(v)
		if err != nil {
			return usageErr("nice: invalid adjustment %q", v)
		}
		adj, explicit = n, true
		return nil
	}
	if i < len(args) {
		a := args[i]
		switch {
		case a == "--":
			i++
		case a == "-n":
			if i+1 >= len(args) {
				return usageErr("nice: option -n requires a value")
			}
			if err := parseAdj(args[i+1]); err != nil {
				return err
			}
			i += 2
		case strings.HasPrefix(a, "-n"):
			if err := parseAdj(a[2:]); err != nil {
				return err
			}
			i++
		case len(a) > 1 && a[0] == '-':
			if err := parseAdj(a[1:]); err != nil {
				return usageErr("nice: unknown option %q", a)
			}
			i++
		}
		if i < len(args) && args[i] == "--" {
			i++
		}
	}
	cur, err := sysinfo.Niceness()
	if i == len(args) {
		if explicit {
			return usageErr("nice: an adjustment needs a program to run")
		}
		if err != nil {
			return fmt.Errorf("nice: %w", err)
		}
		_, err = fmt.Fprintln(s.out, cur)
		return err
	}
	p := process{argv: args[i:], stdin: s.in}
	if err == nil {
		target := min(max(cur+adj, -20), 19)
		p.started = func(cmd *exec.Cmd) { sysinfo.SetNiceness(cmd.Process.Pid, target) }
	}
	return p.run(ctx, s, "nice")
}

const nohupHelp = `Usage: uniz nohup program [args...]
Run program in a new session, detached from the controlling terminal, so a
hangup does not reach it. If stdout is a terminal, output is appended to
nohup.out (or $HOME/nohup.out); stderr follows it when it is a terminal too.
A terminal on stdin is replaced by the null device.
Exit status: 125 if nohup fails, 126/127 if program cannot run or is not
found, otherwise the program's. Unlike GNU nohup, SIGHUP is not ignored; the
program is moved out of the terminal's session instead.
`

func isTerminal(x any) bool {
	f, ok := x.(*os.File)
	if !ok {
		return false
	}
	_, err := sysinfo.TerminalName(f)
	return err == nil
}

func runNohup(ctx context.Context, s stdio, args []string) error {
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	if len(args) == 0 {
		return usageErr("nohup: missing program")
	}
	p := process{argv: args, stdin: s.in, delay: detach}
	if isTerminal(s.in) {
		p.stdin = nil
	}
	child := s
	if isTerminal(s.out) {
		f, err := os.OpenFile("nohup.out", os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
		if err != nil {
			home, herr := os.UserHomeDir()
			if herr != nil {
				return &ExitError{Code: 125, Err: fmt.Errorf("nohup: %w", err)}
			}
			if f, err = os.OpenFile(filepath.Join(home, "nohup.out"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600); err != nil {
				return &ExitError{Code: 125, Err: fmt.Errorf("nohup: %w", err)}
			}
		}
		defer f.Close()
		child.out = f
		if isTerminal(s.err) {
			child.err = f
		}
	}
	return p.run(ctx, child, "nohup")
}

const killHelp = `Usage: uniz kill [-s SIGNAL | -SIGNAL] [--] pid...
       uniz kill -l [SIGNAL | STATUS]...
Send SIGNAL (default TERM) to processes. SIGNAL is a name such as KILL or
SIGKILL, or a number. A pid of 0 means the process group; use -- before a
negative pid to signal a process group. Signal 0 only checks that the
process exists. -l lists signal names, or converts names, numbers, and exit
statuses above 128 to the other form. On Windows only KILL and TERM (both
kill outright) and 0 are supported.
`

func runKill(ctx context.Context, s stdio, args []string) error {
	if len(args) > 0 && (args[0] == "-l" || args[0] == "-L") {
		out := &outputWriter{w: s.out}
		if len(args) == 1 {
			for _, n := range sysinfo.SignalNames() {
				fmt.Fprintln(out, n)
			}
			return out.err
		}
		err := forEach(ctx, "kill", args[1:], func(a string) error {
			if n, err := strconv.Atoi(a); err == nil {
				if n > 128 {
					n -= 128
				}
				name, ok := sysinfo.SignalName(n)
				if !ok {
					return fmt.Errorf("unknown signal %s", a)
				}
				_, err := fmt.Fprintln(out, name)
				return err
			}
			n, err := sysinfo.SignalNumber(a)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(out, n)
			return err
		})
		return err
	}
	sig, i := 15, 0
	if i < len(args) && strings.HasPrefix(args[i], "-") && len(args[i]) > 1 {
		a := args[i]
		var spec string
		switch {
		case a == "--":
		case a == "-s" || a == "-n":
			if i+1 >= len(args) {
				return usageErr("kill: option %s requires a value", a)
			}
			i++
			spec = args[i]
		default:
			spec = a[1:]
		}
		if a != "--" {
			n, err := sysinfo.SignalNumber(spec)
			if err != nil {
				return usageErr("kill: %v", err)
			}
			sig = n
		}
		i++
		if a != "--" && i < len(args) && args[i] == "--" {
			i++
		}
	}
	if i == len(args) {
		return usageErr("kill: missing process id")
	}
	return forEach(ctx, "kill", args[i:], func(a string) error {
		pid, err := strconv.Atoi(a)
		if err != nil {
			return fmt.Errorf("invalid process id %q", a)
		}
		if err := sysinfo.Kill(pid, sig); err != nil {
			return fmt.Errorf("(%d): %w", pid, err)
		}
		return nil
	})
}

const timeHelp = `Usage: uniz time [-p] program [args...]
Run program, then write its elapsed real time and user and system CPU time
in seconds to standard error:
  real 0.52
  user 0.31
  sys 0.04
-p selects this POSIX format, which is also the default. The exit status is
the program's (126/127 if it cannot run or is not found).
`

func runTime(ctx context.Context, s stdio, args []string) error {
	if len(args) > 0 && args[0] == "-p" {
		args = args[1:]
	}
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	if len(args) == 0 {
		return usageErr("time: missing program")
	}
	var state *os.ProcessState
	start := time.Now()
	p := process{argv: args, stdin: s.in, done: func(cmd *exec.Cmd) { state = cmd.ProcessState }}
	err := p.run(ctx, s, "time")
	if state != nil {
		fmt.Fprintf(s.err, "real %.2f\nuser %.2f\nsys %.2f\n", time.Since(start).Seconds(), state.UserTime().Seconds(), state.SystemTime().Seconds())
	}
	return err
}
