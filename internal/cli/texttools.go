package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/pkar/uniz/internal/digest"
	"github.com/pkar/uniz/internal/format"
	"github.com/pkar/uniz/internal/testexpr"
	"github.com/pkar/uniz/internal/text"
)

func registerTextTools() {
	add := func(name, summary, usage string, literal bool, run func(context.Context, stdio, []string) error) {
		registry[name] = command{summary, usage, literal, run}
	}
	add("cut", "select bytes, characters, or fields of lines", cutHelp, false, runCut)
	add("tr", "translate, delete, or squeeze characters", trHelp, false, runTr)
	add("paste", "merge lines of files", pasteHelp, false, runPaste)
	add("nl", "number lines", nlHelp, false, runNl)
	add("tac", "print lines in reverse order", tacHelp, false, perInput("tac", text.ReverseLines))
	add("rev", "reverse the characters of each line", revHelp, false, perInput("rev", text.ReverseCharacters))
	add("fold", "wrap long lines", foldHelp, false, runFold)
	for _, alg := range []digest.Algorithm{digest.MD5, digest.SHA1, digest.SHA224, digest.SHA256, digest.SHA384, digest.SHA512} {
		name := string(alg) + "sum"
		add(name, "print or check "+strings.ToUpper(string(alg))+" checksums", strings.ReplaceAll(sumHelp, "NAME", name), false, digestCommand(name, alg))
	}
	add("cksum", "print POSIX CRC checksums and sizes", cksumHelp, false, runCksum)
	add("base64", "encode or decode base64", base64Help, false, runBase64)
	add("printf", "format and print arguments", printfHelp, true, runPrintf)
	add("test", "evaluate a conditional expression", testHelp, true, runTest(false))
	add("[", "evaluate a conditional expression (needs a closing ])", testHelp, true, runTest(true))
	add("date", "print or format the date and time", dateHelp, false, runDate)
}

func perInput(command string, fn func(context.Context, io.Writer, io.Reader) error) func(context.Context, stdio, []string) error {
	return func(ctx context.Context, s stdio, args []string) error {
		o, err := parseOptions(command, args, "", "")
		if err != nil {
			return err
		}
		out := &outputWriter{w: s.out}
		return eachInput(ctx, command, inputPaths(o.operands), s.in, out, func(_ string, r io.Reader) error {
			return fn(ctx, out, r)
		})
	}
}

const tacHelp = `Usage: uniz tac [--] [files...]
Print each input's lines last to first. Every output line ends with a newline.
Each input is held in memory. Separators (-s) are not supported.
`
const revHelp = `Usage: uniz rev [--] [files...]
Reverse the characters of every line. A missing final newline is preserved.
`

const cutHelp = `Usage: uniz cut -b LIST | -c LIST | -f LIST [-d DELIM] [-s] [--] [files...]
  -b LIST  select bytes
  -c LIST  select characters (UTF-8)
  -f LIST  select fields separated by DELIM (default tab)
  -d DELIM single-character field delimiter
  -s       with -f, skip lines that contain no delimiter
LIST is comma-separated: N, N-M, N-, or -M (1-based). Output keeps input
order. A missing final newline is preserved. --complement is not supported.
`

func runCut(ctx context.Context, s stdio, args []string) error {
	o, err := parseOptions("cut", args, "sn", "bcfd")
	if err != nil {
		return err
	}
	var opts text.CutOptions
	list, modes := "", 0
	for _, m := range []struct {
		flag rune
		unit text.CutUnit
	}{{'b', text.CutBytes}, {'c', text.CutChars}, {'f', text.CutFields}} {
		if v, ok := o.values[m.flag]; ok {
			list, opts.Unit = v, m.unit
			modes++
		}
	}
	if modes != 1 {
		return usageErr("cut: specify exactly one of -b, -c, or -f")
	}
	if opts.Unit != text.CutFields && (o.set['d'] || o.set['s']) {
		return usageErr("cut: -d and -s require -f")
	}
	if opts.Ranges, err = text.ParseRanges(list); err != nil {
		return usageErr("cut: %v", err)
	}
	opts.Delimiter, opts.OnlyDelimited = o.values['d'], o.set['s']
	if o.set['d'] && len([]rune(opts.Delimiter)) != 1 {
		return usageErr("cut: the delimiter must be a single character")
	}
	out := &outputWriter{w: s.out}
	return eachInput(ctx, "cut", inputPaths(o.operands), s.in, out, func(_ string, r io.Reader) error {
		return text.Cut(ctx, out, r, opts)
	})
}

const trHelp = `Usage: uniz tr [-cCds] [--] SET1 [SET2]
  -c, -C  use the complement of SET1
  -d      delete characters in SET1
  -s      squeeze repeats of characters in the last set
Translate stdin to stdout. Sets support escapes (\n \t \\ \NNN), ranges
(a-z), and ASCII classes ([:alpha:] [:digit:] [:lower:] [:upper:] [:space:]
[:blank:] [:alnum:] [:punct:] [:print:] [:graph:] [:cntrl:] [:xdigit:]).
Works on Unicode characters. [c*n] repeats and [=c=] are not supported.
`

func runTr(ctx context.Context, s stdio, args []string) error {
	o, err := parseOptions("tr", args, "cCds", "")
	if err != nil {
		return err
	}
	opts := text.TranslateOptions{Delete: o.set['d'], Squeeze: o.set['s'], Complement: o.set['c'] || o.set['C']}
	n := len(o.operands)
	switch {
	case n == 0:
		return usageErr("tr: missing operand")
	case opts.Delete && !opts.Squeeze && n != 1:
		return usageErr("tr: -d takes exactly one set")
	case opts.Delete && opts.Squeeze && n != 2:
		return usageErr("tr: -d -s takes exactly two sets")
	case !opts.Delete && !opts.Squeeze && n != 2:
		return usageErr("tr: two sets are required to translate")
	case n > 2:
		return usageErr("tr: extra operand %q", o.operands[2])
	}
	set2 := ""
	if n == 2 {
		set2 = o.operands[1]
	}
	if _, err := text.ExpandSet(o.operands[0]); err != nil {
		return usageErr("tr: %v", err)
	}
	if _, err := text.ExpandSet(set2); err != nil {
		return usageErr("tr: %v", err)
	}
	if err := text.Translate(ctx, s.out, s.in, o.operands[0], set2, opts); err != nil {
		return fmt.Errorf("tr: %w", err)
	}
	return nil
}

const pasteHelp = `Usage: uniz paste [-s] [-d LIST] [--] [files...]
  -d LIST  use the characters of LIST cyclically as delimiters (default tab);
           \n, \t, \\, and \0 (no delimiter) are recognized
  -s       paste each file's lines onto one line instead of side by side
'-' reads stdin; repeating it reads successive lines (paste - -).
`

func runPaste(ctx context.Context, s stdio, args []string) error {
	o, err := parseOptions("paste", args, "s", "d")
	if err != nil {
		return err
	}
	opts := text.PasteOptions{Serial: o.set['s']}
	if list, ok := o.values['d']; ok {
		if list == "" {
			opts.Delimiters = []string{""}
		}
		for rest := list; rest != ""; {
			_, size := utf8.DecodeRuneInString(rest)
			d := rest[:size]
			if d == `\` && len(rest) > 1 {
				_, n := utf8.DecodeRuneInString(rest[1:])
				d, size = rest[1:1+n], 1+n
				if e, ok := map[string]string{"n": "\n", "t": "\t", "0": ""}[d]; ok {
					d = e
				}
			}
			opts.Delimiters = append(opts.Delimiters, d)
			rest = rest[size:]
		}
	}
	var srcs []io.Reader
	var files []*os.File
	defer func() {
		for _, f := range files {
			f.Close()
		}
	}()
	for _, path := range inputPaths(o.operands) {
		if path == "-" {
			srcs = append(srcs, s.in)
			continue
		}
		f, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("paste: %w", err)
		}
		files = append(files, f)
		srcs = append(srcs, f)
	}
	return text.Paste(ctx, s.out, srcs, opts)
}

const nlHelp = `Usage: uniz nl [-b STYLE] [-w WIDTH] [-s SEP] [-v START] [-i INCR] [--] [files...]
  -b a|t|n  number all lines, non-empty lines (default), or none
  -w WIDTH  number width (default 6)
  -s SEP    text after each number (default tab)
  -v START  first line number (default 1)
  -i INCR   increment (default 1)
Numbering continues across files. Logical pages (\:) are not supported.
`

func runNl(ctx context.Context, s stdio, args []string) error {
	o, err := parseOptions("nl", args, "", "bwsvi")
	if err != nil {
		return err
	}
	opts := text.DefaultNumberOptions()
	if v, ok := o.values['b']; ok {
		styles := map[string]text.NumberStyle{"a": text.NumberAll, "t": text.NumberNonEmpty, "n": text.NumberNone}
		style, ok := styles[v]
		if !ok {
			return usageErr("nl: invalid body numbering style %q", v)
		}
		opts.Style = style
	}
	if v, ok := o.values['s']; ok {
		opts.Separator = v
	}
	for flag, dst := range map[rune]*int64{'v': &opts.Start, 'i': &opts.Increment} {
		if v, ok := o.values[flag]; ok {
			if *dst, err = strconv.ParseInt(v, 10, 64); err != nil {
				return usageErr("nl: invalid number %q", v)
			}
		}
	}
	if v, ok := o.values['w']; ok {
		if opts.Width, err = strconv.Atoi(v); err != nil || opts.Width < 1 {
			return usageErr("nl: invalid width %q", v)
		}
	}
	out := &outputWriter{w: s.out}
	return eachInput(ctx, "nl", inputPaths(o.operands), s.in, out, func(_ string, r io.Reader) error {
		next, err := text.NumberLines(ctx, out, r, opts)
		opts.Start = next
		return err
	})
}

const foldHelp = `Usage: uniz fold [-bs] [-w WIDTH] [--] [files...]
  -w WIDTH  maximum line width (default 80)
  -b        count bytes instead of characters
  -s        break after the last blank within the width when possible
Every character, including tab, counts as one column.
`

func runFold(ctx context.Context, s stdio, args []string) error {
	for i, a := range args { // fold -NUM is shorthand for -w NUM.
		if a == "--" {
			break
		}
		if len(a) > 1 && a[0] == '-' && strings.Trim(a[1:], "0123456789") == "" {
			args[i] = "-w" + a[1:]
		}
	}
	o, err := parseOptions("fold", args, "bs", "w")
	if err != nil {
		return err
	}
	opts := text.FoldOptions{Width: 80, Bytes: o.set['b'], Spaces: o.set['s']}
	if v, ok := o.values['w']; ok {
		if opts.Width, err = strconv.Atoi(v); err != nil || opts.Width < 1 {
			return usageErr("fold: invalid width %q", v)
		}
	}
	out := &outputWriter{w: s.out}
	return eachInput(ctx, "fold", inputPaths(o.operands), s.in, out, func(_ string, r io.Reader) error {
		return text.Fold(ctx, out, r, opts)
	})
}

const sumHelp = `Usage: uniz NAME [-c] [--] [files...]
  -c  read "<hex>  <name>" lines from the files and verify each named file
Print "<hex>  <name>" for each input ('-' or no files reads stdin).
With -c, prints "name: OK" or "name: FAILED" and fails if any check fails.
Names are printed verbatim, without GNU's backslash escaping.
`

func digestCommand(command string, alg digest.Algorithm) func(context.Context, stdio, []string) error {
	return func(ctx context.Context, s stdio, args []string) error {
		o, err := parseOptions(command, args, "c", "")
		if err != nil {
			return err
		}
		out := &outputWriter{w: s.out}
		if !o.set['c'] {
			return eachInput(ctx, command, inputPaths(o.operands), s.in, out, func(path string, r io.Reader) error {
				sum, err := digest.Sum(ctx, alg, r)
				if err != nil {
					return err
				}
				_, err = fmt.Fprintf(out, "%s  %s\n", sum, path)
				return err
			})
		}
		h, _ := digest.New(alg)
		var failed, unreadable int
		err = eachInput(ctx, command, inputPaths(o.operands), s.in, out, func(path string, r io.Reader) error {
			sc := bufio.NewScanner(r)
			sc.Buffer(nil, 1<<20)
			valid := 0
			for sc.Scan() {
				if err := ctx.Err(); err != nil {
					return err
				}
				want, name, ok := digest.ParseLine(sc.Text())
				if !ok || len(want) != 2*h.Size() {
					continue
				}
				valid++
				status := "OK"
				f, err := os.Open(name)
				if err == nil {
					var got string
					got, err = digest.Sum(ctx, alg, f)
					f.Close()
					if err == nil && got != want {
						status = "FAILED"
						failed++
					}
				}
				if err != nil {
					status = "FAILED open or read"
					unreadable++
				}
				if _, err := fmt.Fprintf(out, "%s: %s\n", name, status); err != nil {
					return err
				}
			}
			if err := sc.Err(); err != nil {
				return err
			}
			if valid == 0 {
				return errors.New("no properly formatted checksum lines found")
			}
			return nil
		})
		var errs []error
		if err != nil {
			errs = append(errs, err)
		}
		if failed > 0 {
			errs = append(errs, fmt.Errorf("%s: %d computed checksum(s) did NOT match", command, failed))
		}
		if unreadable > 0 {
			errs = append(errs, fmt.Errorf("%s: %d listed file(s) could not be read", command, unreadable))
		}
		return errors.Join(errs...)
	}
}

const cksumHelp = `Usage: uniz cksum [--] [files...]
Print the POSIX CRC checksum, byte count, and name of each input.
Stdin read implicitly is printed without a name. -a algorithms are not supported.
`

func runCksum(ctx context.Context, s stdio, args []string) error {
	o, err := parseOptions("cksum", args, "", "")
	if err != nil {
		return err
	}
	out := &outputWriter{w: s.out}
	return eachInput(ctx, "cksum", inputPaths(o.operands), s.in, out, func(path string, r io.Reader) error {
		sum, size, err := digest.CRC(ctx, r)
		if err != nil {
			return err
		}
		if len(o.operands) == 0 {
			_, err = fmt.Fprintf(out, "%d %d\n", sum, size)
		} else {
			_, err = fmt.Fprintf(out, "%d %d %s\n", sum, size, path)
		}
		return err
	})
}

const base64Help = `Usage: uniz base64 [-d] [-w COLS] [--] [file]
  -d       decode instead of encode
  -w COLS  wrap encoded lines at COLS characters (default 76; 0 disables)
Decoding ignores line breaks and rejects other non-alphabet characters.
`

func runBase64(ctx context.Context, s stdio, args []string) error {
	o, err := parseOptions("base64", args, "d", "w")
	if err != nil {
		return err
	}
	if len(o.operands) > 1 {
		return usageErr("base64: extra operand %q", o.operands[1])
	}
	wrap := 76
	if v, ok := o.values['w']; ok {
		if wrap, err = strconv.Atoi(v); err != nil || wrap < 0 {
			return usageErr("base64: invalid wrap size %q", v)
		}
	}
	out := &outputWriter{w: s.out}
	return eachInput(ctx, "base64", inputPaths(o.operands), s.in, out, func(_ string, r io.Reader) error {
		if o.set['d'] {
			return digest.Decode(ctx, out, r)
		}
		return digest.Encode(ctx, out, r, wrap)
	})
}

const printfHelp = `Usage: uniz printf FORMAT [arguments...]
Format arguments like C printf. Conversions: %d %i %u %o %x %X %f %F %e %E
%g %G %c %s %b %%, with flags (-+ #0), width, and precision (including *).
FORMAT escapes: \\ \" \a \b \f \n \r \t \v \NNN \xHH \c. %b expands echo -e
escapes in its argument. FORMAT is reused while arguments remain. Numeric
arguments may be decimal, 0x hex, 0 octal, or 'c (a character's code).
Invalid numbers print as 0 and make the command fail after printing.
`

func runPrintf(_ context.Context, s stdio, args []string) error {
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	if len(args) == 0 {
		return usageErr("printf: missing format")
	}
	out, ferr := format.Printf(args[0], args[1:])
	if _, err := io.WriteString(s.out, out); err != nil {
		return err
	}
	if ferr != nil {
		return fmt.Errorf("printf: %w", ferr)
	}
	return nil
}

const testHelp = `Usage: uniz test EXPRESSION
       uniz [ EXPRESSION ]
Exit 0 if EXPRESSION is true, 1 if false, 2 on a syntax error.
Strings:  -n S, -z S, S, S1 = S2, S1 == S2, S1 != S2, S1 < S2, S1 > S2
Integers: N1 -eq|-ne|-lt|-le|-gt|-ge N2
Files:    -e -f -d -h -L -s -r -w -x -b -c -p -S -g -u -k FILE;
          F1 -nt F2, F1 -ot F2, F1 -ef F2
Logic:    ! EXPR, EXPR -a EXPR, EXPR -o EXPR, ( EXPR )
-t (terminal test) is not supported.
`

func runTest(bracket bool) func(context.Context, stdio, []string) error {
	return func(_ context.Context, _ stdio, args []string) error {
		name := "test"
		if bracket {
			name = "["
			if len(args) == 0 || args[len(args)-1] != "]" {
				return usageErr("[: missing ']'")
			}
			args = args[:len(args)-1]
		}
		ok, err := testexpr.Evaluate(args)
		if err != nil {
			return usageErr("%s: %v", name, err)
		}
		if !ok {
			return &ExitError{Code: 1}
		}
		return nil
	}
}

const dateHelp = `Usage: uniz date [-uIR] [-d DATE | -r FILE] [+FORMAT]
  -u       use UTC
  -d DATE  show DATE instead of now: "@SECONDS", RFC 3339, "YYYY-MM-DD[ HH:MM[:SS]]",
           RFC 1123, or date's default output format
  -r FILE  show FILE's modification time
  -I       ISO 8601 date (%Y-%m-%d)
  -R       RFC 5322 format (%a, %d %b %Y %H:%M:%S %z)
FORMAT uses strftime conversions (see the library's Strftime documentation),
default "%a %b %e %H:%M:%S %Z %Y". Setting the clock is not supported.
`

func runDate(_ context.Context, s stdio, args []string) error {
	o, err := parseOptions("date", args, "uIR", "dr")
	if err != nil {
		return err
	}
	if o.set['d'] && o.set['r'] {
		return usageErr("date: -d and -r cannot be combined")
	}
	loc := time.Local
	if o.set['u'] {
		loc = time.UTC
	}
	layout := format.DefaultDateFormat
	switch {
	case o.set['I']:
		layout = "%Y-%m-%d"
	case o.set['R']:
		layout = "%a, %d %b %Y %H:%M:%S %z"
	}
	for _, op := range o.operands {
		if !strings.HasPrefix(op, "+") {
			return usageErr("date: invalid operand %q (setting the date is not supported)", op)
		}
		if len(o.operands) > 1 {
			return usageErr("date: extra operand")
		}
		layout = op[1:]
	}
	t := time.Now().In(loc)
	if v, ok := o.values['d']; ok {
		if t, err = format.ParseDate(v, time.Now(), loc); err != nil {
			return usageErr("date: %v", err)
		}
	}
	if v, ok := o.values['r']; ok {
		info, err := os.Stat(v)
		if err != nil {
			return fmt.Errorf("date: %w", err)
		}
		t = info.ModTime().In(loc)
	}
	_, err = fmt.Fprintln(s.out, format.Strftime(t, layout))
	return err
}
