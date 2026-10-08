package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/pkar/uniz/internal/digest"
	"github.com/pkar/uniz/internal/split"
	"github.com/pkar/uniz/internal/text"
)

func registerMoreText() {
	add := func(name, summary, usage string, run func(context.Context, stdio, []string) error) {
		registry[name] = command{summary, usage, false, run}
	}
	add("fmt", "fill and wrap paragraphs", fmtHelp, runFmt)
	add("expand", "convert tabs to spaces", expandHelp, runExpand)
	add("unexpand", "convert spaces to tabs", unexpandHelp, runUnexpand)
	add("comm", "compare two sorted files line by line", commHelp, runComm)
	add("join", "join lines of two files on a common field", joinHelp, runJoin)
	add("split", "split a file into pieces", splitHelp, runSplit)
	add("csplit", "split a file at context lines", csplitHelp, runCsplit)
	add("od", "dump bytes in octal, hex, decimal, or characters", odHelp, runOd)
	add("base32", "encode or decode base32", base32Help, runBase32)
	add("shuf", "shuffle lines", shufHelp, runShuf)
	add("tsort", "sort items topologically", tsortHelp, runTsort)
}

func positiveInt(command, what, v string) (int, error) {
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return 0, usageErr("%s: invalid %s %q", command, what, v)
	}
	return n, nil
}

const fmtHelp = `Usage: uniz fmt [-w WIDTH | -WIDTH] [-su] [--] [files...]
  -w WIDTH  maximum line width (default 75)
  -s        split long lines only; do not join short ones
  -u        uniform spacing (always on: words are joined by one space)
Paragraphs end at blank lines or where indentation changes after the second
line. The first line keeps its indentation; later lines use the second's.
`

func runFmt(ctx context.Context, s stdio, args []string) error {
	args = append([]string(nil), args...)
	for i, a := range args {
		if a == "--" {
			break
		}
		if len(a) > 1 && a[0] == '-' && strings.Trim(a[1:], "0123456789") == "" {
			args[i] = "-w" + a[1:]
		}
	}
	o, err := parseOptions("fmt", args, "su", "w")
	if err != nil {
		return err
	}
	opts := text.FmtOptions{SplitOnly: o.set['s']}
	if v, ok := o.values['w']; ok {
		if opts.Width, err = positiveInt("fmt", "width", v); err != nil {
			return err
		}
	}
	out := &outputWriter{w: s.out}
	return eachInput(ctx, "fmt", inputPaths(o.operands), s.in, out, func(_ string, r io.Reader) error {
		return text.Fmt(ctx, out, r, opts)
	})
}

const expandHelp = `Usage: uniz expand [-i] [-t LIST] [--] [files...]
  -i       convert only leading tabs
  -t LIST  tab stops: N (every N columns) or an ascending list such as 4,8,12
           (after the last stop, tabs become single spaces); default 8
`

func tabStops(command string, o options) (text.TabStops, error) {
	v, ok := o.values['t']
	if !ok {
		return nil, nil
	}
	t, err := text.ParseTabStops(v)
	if err != nil {
		return nil, usageErr("%s: %v", command, err)
	}
	return t, nil
}

func runExpand(ctx context.Context, s stdio, args []string) error {
	o, err := parseOptions("expand", args, "i", "t")
	if err != nil {
		return err
	}
	opts := text.ExpandOptions{Initial: o.set['i']}
	if opts.Tabs, err = tabStops("expand", o); err != nil {
		return err
	}
	out := &outputWriter{w: s.out}
	return eachInput(ctx, "expand", inputPaths(o.operands), s.in, out, func(_ string, r io.Reader) error {
		return text.Expand(ctx, out, r, opts)
	})
}

const unexpandHelp = `Usage: uniz unexpand [-a] [-t LIST] [--] [files...]
  -a       convert blanks anywhere, not only leading blanks
  -t LIST  tab stops as for expand (implies -a); default 8
A single space before a tab stop is left alone.
`

func runUnexpand(ctx context.Context, s stdio, args []string) error {
	o, err := parseOptions("unexpand", args, "a", "t")
	if err != nil {
		return err
	}
	opts := text.UnexpandOptions{All: o.set['a'] || o.set['t']}
	if opts.Tabs, err = tabStops("unexpand", o); err != nil {
		return err
	}
	out := &outputWriter{w: s.out}
	return eachInput(ctx, "unexpand", inputPaths(o.operands), s.in, out, func(_ string, r io.Reader) error {
		return text.Unexpand(ctx, out, r, opts)
	})
}

// openTwo opens two operands, at most one of which may be "-" (stdin).
func openTwo(command string, operands []string, stdin io.Reader) (a, b io.Reader, closeAll func(), err error) {
	if len(operands) != 2 {
		return nil, nil, nil, usageErr("%s: expected two files", command)
	}
	if operands[0] == "-" && operands[1] == "-" {
		return nil, nil, nil, usageErr("%s: only one input may be stdin", command)
	}
	var files []*os.File
	closeAll = func() {
		for _, f := range files {
			f.Close()
		}
	}
	open := func(p string) (io.Reader, error) {
		if p == "-" {
			return stdin, nil
		}
		f, err := os.Open(p)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", command, err)
		}
		files = append(files, f)
		return f, nil
	}
	if a, err = open(operands[0]); err == nil {
		b, err = open(operands[1])
	}
	if err != nil {
		closeAll()
		return nil, nil, nil, err
	}
	return a, b, closeAll, nil
}

const commHelp = `Usage: uniz comm [-123] [--] FILE1 FILE2
Compare two sorted files ("-" is stdin). Column 1 holds lines only in FILE1,
column 2 lines only in FILE2, column 3 lines in both, separated by tabs.
  -1, -2, -3  suppress that column
Lines are compared bytewise; inputs are not checked for order.
`

func runComm(ctx context.Context, s stdio, args []string) error {
	o, err := parseOptions("comm", args, "123", "")
	if err != nil {
		return err
	}
	a, b, closeAll, err := openTwo("comm", o.operands, s.in)
	if err != nil {
		return err
	}
	defer closeAll()
	if err := text.Comm(ctx, s.out, a, b, text.CommOptions{Suppress1: o.set['1'], Suppress2: o.set['2'], Suppress3: o.set['3']}); err != nil {
		return fmt.Errorf("comm: %w", err)
	}
	return nil
}

const joinHelp = `Usage: uniz join [-i] [-a 1|2] [-v 1|2] [-e EMPTY] [-o LIST] [-t CHAR]
                 [-1 FIELD] [-2 FIELD] [-j FIELD] [--] FILE1 FILE2
Join lines of two files sorted on their join fields ("-" is stdin). Output:
the join field, the other fields of FILE1, then the other fields of FILE2.
  -a N      also print unpairable lines from file N (repeatable)
  -v N      print only unpairable lines from file N (repeatable)
  -e EMPTY  replace missing fields named by -o
  -o LIST   output fields such as 0,1.2,2.1 (0 is the join field)
  -t CHAR   field separator (default: runs of blanks; output uses a space)
  -1, -2    join field of file 1 or 2 (default 1); -j sets both
  -i        ignore case when comparing join fields
Both files are held in memory. Order is checked bytewise but not enforced.
`

func runJoin(ctx context.Context, s stdio, args []string) error {
	o, err := parseOptions("join", args, "i", "avteo12j")
	if err != nil {
		return err
	}
	opts := text.JoinOptions{Empty: o.values['e'], IgnoreCase: o.set['i'], Separator: o.values['t']}
	if o.set['t'] && len([]rune(opts.Separator)) != 1 {
		return usageErr("join: the separator must be a single character")
	}
	for _, f := range []struct {
		flag rune
		dst  []*int
	}{{'j', []*int{&opts.Field1, &opts.Field2}}, {'1', []*int{&opts.Field1}}, {'2', []*int{&opts.Field2}}} {
		if v, ok := o.values[f.flag]; ok {
			n, err := positiveInt("join", "field number", v)
			if err != nil {
				return err
			}
			for _, d := range f.dst {
				*d = n
			}
		}
	}
	for _, flag := range []rune{'a', 'v'} {
		for _, v := range o.lists[flag] {
			switch v {
			case "1":
				opts.Unpaired1 = true
			case "2":
				opts.Unpaired2 = true
			default:
				return usageErr("join: invalid file number %q", v)
			}
		}
	}
	opts.OnlyUnpaired = o.set['v']
	if v, ok := o.values['o']; ok {
		if opts.Output, err = text.ParseJoinFields(v); err != nil {
			return usageErr("join: %v", err)
		}
	}
	a, b, closeAll, err := openTwo("join", o.operands, s.in)
	if err != nil {
		return err
	}
	defer closeAll()
	if err := text.Join(ctx, s.out, a, b, opts); err != nil {
		return fmt.Errorf("join: %w", err)
	}
	return nil
}

// parseByteCount parses N with an optional K, M, G, T, P, or E suffix
// (powers of 1024) or KB, MB, ... (powers of 1000).
func parseByteCount(command, v string) (int64, error) {
	num, mult := v, int64(1)
	for i, u := range "KMGTPE" {
		p1024, p1000 := int64(1)<<(10*(i+1)), int64(1)
		for range i + 1 {
			p1000 *= 1000
		}
		switch {
		case strings.HasSuffix(v, string(u)+"B"):
			num, mult = v[:len(v)-2], p1000
		case strings.HasSuffix(v, string(u)), u == 'K' && strings.HasSuffix(v, "k"):
			num, mult = v[:len(v)-1], p1024
		default:
			continue
		}
		break
	}
	n, err := strconv.ParseInt(num, 10, 64)
	if err != nil || n <= 0 || n > (1<<62)/mult {
		return 0, usageErr("%s: invalid size %q", command, v)
	}
	return n * mult, nil
}

const splitHelp = `Usage: uniz split [-l LINES | -b SIZE | -n CHUNKS] [-a LEN] [-d] [--] [file [prefix]]
  -l LINES   put LINES lines in each file (default 1000)
  -b SIZE    put SIZE bytes in each file (K, M, G suffixes = 1024^n; KB, MB = 1000^n)
  -n CHUNKS  split into CHUNKS files of near-equal size (stdin is buffered)
  -a LEN     suffix length (default 2)
  -d         numeric suffixes (00, 01, ...) instead of aa, ab, ...
Output files are PREFIX (default x) plus the suffix. Existing regular files
are overwritten; symbolic links are refused.
`

func runSplit(ctx context.Context, s stdio, args []string) error {
	o, err := parseOptions("split", args, "d", "lbna")
	if err != nil {
		return err
	}
	if len(o.operands) > 2 {
		return usageErr("split: extra operand %q", o.operands[2])
	}
	modes := 0
	for _, f := range "lbn" {
		if o.set[f] {
			modes++
		}
	}
	if modes > 1 {
		return usageErr("split: use only one of -l, -b, or -n")
	}
	opts := split.Options{Numeric: o.set['d']}
	if v, ok := o.values['l']; ok {
		n, err := positiveInt("split", "line count", v)
		if err != nil {
			return err
		}
		opts.Lines = int64(n)
	}
	if v, ok := o.values['b']; ok {
		if opts.Bytes, err = parseByteCount("split", v); err != nil {
			return err
		}
	}
	if v, ok := o.values['n']; ok {
		if opts.Chunks, err = positiveInt("split", "chunk count", v); err != nil {
			return err
		}
	}
	if v, ok := o.values['a']; ok {
		if opts.SuffixLength, err = positiveInt("split", "suffix length", v); err != nil {
			return err
		}
	}
	var src io.Reader = s.in
	if len(o.operands) > 0 && o.operands[0] != "-" {
		f, err := os.Open(o.operands[0])
		if err != nil {
			return fmt.Errorf("split: %w", err)
		}
		defer f.Close()
		src = f
		if info, err := f.Stat(); err == nil && info.Mode().IsRegular() {
			opts.Size = info.Size()
		}
	}
	if len(o.operands) == 2 {
		opts.Prefix = o.operands[1]
	}
	if opts.Chunks > 0 && (src == s.in || opts.Size == 0) {
		data, err := io.ReadAll(src)
		if err != nil {
			return fmt.Errorf("split: %w", err)
		}
		src, opts.Size = strings.NewReader(string(data)), int64(len(data))
	}
	if _, err := split.Split(ctx, src, opts); err != nil {
		return fmt.Errorf("split: %w", err)
	}
	return nil
}

const csplitHelp = `Usage: uniz csplit [-ksz] [-f PREFIX] [-n DIGITS] [--] FILE PATTERN...
Split FILE ("-" is stdin) into xx00, xx01, ... and print each piece's size.
Patterns:
  N           split before line N
  /RE/[OFF]   split before the next line matching basic regexp RE (+/- OFF lines)
  %RE%[OFF]   skip to that line without writing a piece
  {N}  {*}    repeat the previous pattern N times, or while it matches
  -f PREFIX  file name prefix (default xx)
  -n DIGITS  suffix digits (default 2)
  -k         keep files written before an error
  -s         do not print sizes
  -z         do not write empty pieces
`

func runCsplit(ctx context.Context, s stdio, args []string) error {
	o, err := parseOptions("csplit", args, "ksz", "fn")
	if err != nil {
		return err
	}
	if len(o.operands) < 2 {
		return usageErr("csplit: expected a file and at least one pattern")
	}
	opts := split.CsplitOptions{Prefix: o.values['f'], KeepFiles: o.set['k'], ElideEmpty: o.set['z']}
	if v, ok := o.values['n']; ok {
		if opts.Digits, err = positiveInt("csplit", "digit count", v); err != nil {
			return err
		}
	}
	var src io.Reader = s.in
	if o.operands[0] != "-" {
		f, err := os.Open(o.operands[0])
		if err != nil {
			return fmt.Errorf("csplit: %w", err)
		}
		defer f.Close()
		src = f
	}
	_, sizes, err := split.Csplit(ctx, src, o.operands[1:], opts)
	if !o.set['s'] {
		out := &outputWriter{w: s.out}
		for _, n := range sizes {
			fmt.Fprintln(out, n)
		}
		err = errors.Join(err, out.err)
	}
	if err != nil {
		return fmt.Errorf("csplit: %w", err)
	}
	return nil
}

const odHelp = `Usage: uniz od [-bcdovx] [-A RADIX] [-t TYPE]... [-j SKIP] [-N COUNT] [-w WIDTH] [--] [files...]
Dump the concatenated inputs. Each line starts with the offset.
  -A RADIX  offset radix: o (default), d, x, or n (none)
  -t TYPE   a (named chars), c (chars), d/o/u/x[1|2|4|8] integers, f[4|8] floats
  -j SKIP   skip SKIP bytes first (K, M, G suffixes allowed)
  -N COUNT  dump at most COUNT bytes
  -w WIDTH  bytes per line (default 16)
  -v        print repeated lines instead of "*"
  -b -c -d -o -x  same as -t o1, -t c, -t u2, -t o2, -t x2
Integers use the machine's byte order.
`

func runOd(ctx context.Context, s stdio, args []string) error {
	o, err := parseOptions("od", args, "bcdovx", "AtjNw")
	if err != nil {
		return err
	}
	opts := text.OdOptions{All: o.set['v'], Limit: -1}
	// Keep -t and the short forms in command-line order.
	short := map[rune]string{'b': "o1", 'c': "c", 'd': "u2", 'o': "o2", 'x': "x2"}
	seenT := 0
	for _, a := range args {
		if a == "--" {
			break
		}
		if !strings.HasPrefix(a, "-") || len(a) < 2 {
			continue
		}
		for _, f := range a[1:] {
			var spec string
			if f == 't' {
				if seenT < len(o.lists['t']) {
					spec = o.lists['t'][seenT]
					seenT++
				}
			} else if sp, ok := short[f]; ok {
				spec = sp
			} else if strings.ContainsRune("AjNw", f) {
				break // the rest of this argument is a value
			}
			if spec != "" {
				ts, err := text.ParseOdTypes(spec)
				if err != nil {
					return usageErr("od: %v", err)
				}
				opts.Types = append(opts.Types, ts...)
			}
			if f == 't' {
				break
			}
		}
	}
	if v, ok := o.values['A']; ok {
		if len(v) != 1 || !strings.Contains("odxn", v) {
			return usageErr("od: invalid address radix %q", v)
		}
		opts.Radix = v[0]
	}
	if v, ok := o.values['j']; ok && v != "0" {
		if opts.Skip, err = parseByteCount("od", v); err != nil {
			return err
		}
	}
	if v, ok := o.values['N']; ok {
		if v == "0" {
			opts.Limit = 0
		} else if opts.Limit, err = parseByteCount("od", v); err != nil {
			return err
		}
	}
	if v, ok := o.values['w']; ok {
		if opts.Width, err = positiveInt("od", "width", v); err != nil {
			return err
		}
	}
	var readers []io.Reader
	for _, p := range inputPaths(o.operands) {
		if p == "-" {
			readers = append(readers, s.in)
			continue
		}
		f, err := os.Open(p)
		if err != nil {
			return fmt.Errorf("od: %w", err)
		}
		defer f.Close()
		readers = append(readers, f)
	}
	if err := text.Od(ctx, s.out, io.MultiReader(readers...), opts); err != nil {
		return fmt.Errorf("od: %w", err)
	}
	return nil
}

const base32Help = `Usage: uniz base32 [-d] [-w COLS] [--] [file]
  -d       decode instead of encode
  -w COLS  wrap encoded lines at COLS characters (default 76; 0 disables)
Decoding ignores line breaks and rejects other non-alphabet characters.
`

func runBase32(ctx context.Context, s stdio, args []string) error {
	o, err := parseOptions("base32", args, "d", "w")
	if err != nil {
		return err
	}
	if len(o.operands) > 1 {
		return usageErr("base32: extra operand %q", o.operands[1])
	}
	wrap := 76
	if v, ok := o.values['w']; ok {
		if wrap, err = strconv.Atoi(v); err != nil || wrap < 0 {
			return usageErr("base32: invalid wrap size %q", v)
		}
	}
	out := &outputWriter{w: s.out}
	return eachInput(ctx, "base32", inputPaths(o.operands), s.in, out, func(_ string, r io.Reader) error {
		if o.set['d'] {
			return digest.Base32Decode(ctx, out, r)
		}
		return digest.Base32Encode(ctx, out, r, wrap)
	})
}

const shufHelp = `Usage: uniz shuf [-r] [-n COUNT] [--] [file]
       uniz shuf -e [-r] [-n COUNT] [--] [items...]
       uniz shuf -i LO-HI [-r] [-n COUNT]
Write input lines, items, or the integers LO..HI in random order.
  -n COUNT  output at most COUNT lines
  -r        pick with replacement; without -n this repeats until stopped
`

var rangeRE = regexp.MustCompile(`^(\d+)-(\d+)$`)

func runShuf(ctx context.Context, s stdio, args []string) error {
	o, err := parseOptions("shuf", args, "er", "in")
	if err != nil {
		return err
	}
	opts := text.ShuffleOptions{Count: -1, Repeat: o.set['r']}
	if v, ok := o.values['n']; ok {
		if opts.Count, err = strconv.Atoi(v); err != nil || opts.Count < 0 {
			return usageErr("shuf: invalid line count %q", v)
		}
	}
	var items []string
	switch {
	case o.set['e'] && o.set['i']:
		return usageErr("shuf: -e and -i cannot be combined")
	case o.set['e']:
		items = o.operands
	case o.set['i']:
		m := rangeRE.FindStringSubmatch(o.values['i'])
		if m == nil || len(o.operands) > 0 {
			return usageErr("shuf: invalid input range %q", o.values['i'])
		}
		lo, err1 := strconv.ParseInt(m[1], 10, 64)
		hi, err2 := strconv.ParseInt(m[2], 10, 64)
		if err1 != nil || err2 != nil || hi < lo-1 || hi-lo >= 1<<24 {
			return usageErr("shuf: invalid input range %q", o.values['i'])
		}
		for n := lo; n <= hi; n++ {
			items = append(items, strconv.FormatInt(n, 10))
		}
	default:
		if len(o.operands) > 1 {
			return usageErr("shuf: extra operand %q", o.operands[1])
		}
		out := &outputWriter{w: io.Discard}
		err := eachInput(ctx, "shuf", inputPaths(o.operands), s.in, out, func(_ string, r io.Reader) error {
			var err error
			items, err = text.ReadLines(ctx, r)
			return err
		})
		if err != nil {
			return err
		}
	}
	if err := text.Shuffle(ctx, &outputWriter{w: s.out}, items, opts); err != nil {
		return fmt.Errorf("shuf: %w", err)
	}
	return nil
}

const tsortHelp = `Usage: uniz tsort [--] [file]
Read pairs "A B" (A comes before B) separated by blanks and print a total
order, one item per line. On a cycle every item is still printed, a loop is
reported, and the exit status is 1.
`

func runTsort(ctx context.Context, s stdio, args []string) error {
	o, err := parseOptions("tsort", args, "", "")
	if err != nil {
		return err
	}
	if len(o.operands) > 1 {
		return usageErr("tsort: extra operand %q", o.operands[1])
	}
	out := &outputWriter{w: s.out}
	return eachInput(ctx, "tsort", inputPaths(o.operands), s.in, out, func(_ string, r io.Reader) error {
		return text.Tsort(ctx, out, r)
	})
}
