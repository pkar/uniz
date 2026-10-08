package text

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"strconv"
	"strings"
	"unicode/utf8"
)

// TabStops holds tab stop columns. A single value is a repeating interval;
// several values are explicit, ascending stop columns.
type TabStops []int

// ParseTabStops parses "N" or a comma- or blank-separated ascending list.
func ParseTabStops(s string) (TabStops, error) {
	var stops TabStops
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' }) {
		n, err := strconv.Atoi(f)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("invalid tab stop %q", f)
		}
		if len(stops) > 0 && n <= stops[len(stops)-1] {
			return nil, errors.New("tab stops must be ascending")
		}
		stops = append(stops, n)
	}
	if len(stops) == 0 {
		return nil, fmt.Errorf("invalid tab stops %q", s)
	}
	return stops, nil
}

func (t TabStops) orDefault() TabStops {
	if len(t) == 0 {
		return TabStops{8}
	}
	return t
}

// next returns the column a tab at col advances to. Past the last explicit
// stop a tab advances one column.
func (t TabStops) next(col int) int {
	if len(t) == 1 {
		return col + t[0] - col%t[0]
	}
	for _, s := range t {
		if s > col {
			return s
		}
	}
	return col + 1
}

func (t TabStops) isStop(col int) bool {
	if len(t) == 1 {
		return col%t[0] == 0
	}
	for _, s := range t {
		if s == col {
			return true
		}
	}
	return false
}

// ExpandOptions controls Expand.
type ExpandOptions struct {
	Tabs    TabStops // Default every 8 columns.
	Initial bool     // Convert only tabs before the first non-blank.
}

// Expand converts tabs to spaces. Columns count characters (runes).
func Expand(ctx context.Context, dst io.Writer, src io.Reader, opts ExpandOptions) error {
	tabs := opts.Tabs.orDefault()
	return mapLines(ctx, dst, src, func(line string) (string, bool) {
		var b strings.Builder
		col, leading := 0, true
		for i, r := range line {
			switch {
			case r == '\t' && leading:
				next := tabs.next(col)
				b.WriteString(strings.Repeat(" ", next-col))
				col = next
				continue
			case r == '\b':
				col = max(col-1, 0)
			default:
				col++
			}
			if r != ' ' && r != '\t' && opts.Initial {
				leading = false
				b.WriteString(line[i:])
				return b.String(), true
			}
			b.WriteRune(r)
		}
		return b.String(), true
	})
}

// UnexpandOptions controls Unexpand.
type UnexpandOptions struct {
	Tabs TabStops // Default every 8 columns.
	All  bool     // Convert blanks anywhere, not just leading blanks.
}

// Unexpand converts runs of blanks that reach a tab stop into tabs. A single
// space before a stop stays a space.
func Unexpand(ctx context.Context, dst io.Writer, src io.Reader, opts UnexpandOptions) error {
	tabs := opts.Tabs.orDefault()
	return mapLines(ctx, dst, src, func(line string) (string, bool) {
		var b strings.Builder
		col, start, pendingTab := 0, 0, false
		flush := func() {
			b.WriteString(strings.Repeat(" ", col-start))
			start = col
		}
		for i, r := range line {
			if r == ' ' || r == '\t' {
				if r == '\t' {
					col, pendingTab = tabs.next(col), true
				} else {
					col++
				}
				if tabs.isStop(col) || r == '\t' {
					if pendingTab || col-start >= 2 {
						b.WriteByte('\t')
					} else {
						b.WriteString(strings.Repeat(" ", col-start))
					}
					start, pendingTab = col, false
				}
				continue
			}
			flush()
			if !opts.All {
				b.WriteString(line[i:])
				return b.String(), true
			}
			b.WriteRune(r)
			col++
			start = col
		}
		flush()
		return b.String(), true
	})
}

// FmtOptions controls Fmt.
type FmtOptions struct {
	Width     int  // Maximum line width in characters; default 75.
	SplitOnly bool // Wrap long lines but never join short ones.
}

// Fmt fills paragraphs to Width. Paragraphs are separated by blank lines or
// by a change in indentation after the second line; the first line keeps its
// own indentation and later lines use the second line's. Words are joined by
// one space. Words longer than Width get a line of their own.
func Fmt(ctx context.Context, dst io.Writer, src io.Reader, opts FmtOptions) error {
	width := opts.Width
	if width <= 0 {
		width = 75
	}
	lines, err := ReadLines(ctx, src)
	if err != nil {
		return err
	}
	bw := bufio.NewWriter(writer{ctx, dst})
	indentOf := func(s string) string { return s[:len(s)-len(strings.TrimLeft(s, " \t"))] }
	fill := func(first, rest string, words []string) {
		prefix, line := first, ""
		for _, w := range words {
			if line != "" && utf8.RuneCountInString(prefix+line+" "+w) > width {
				bw.WriteString(prefix + line + "\n")
				prefix, line = rest, ""
			}
			if line == "" {
				line = w
			} else {
				line += " " + w
			}
		}
		if line != "" || len(words) == 0 {
			bw.WriteString(prefix + line + "\n")
		}
	}
	for i := 0; i < len(lines); {
		if strings.TrimSpace(lines[i]) == "" {
			bw.WriteString("\n")
			i++
			continue
		}
		first := indentOf(lines[i])
		if opts.SplitOnly {
			fill(first, first, strings.Fields(lines[i]))
			i++
			continue
		}
		words := strings.Fields(lines[i])
		rest := first
		j := i + 1
		if j < len(lines) && strings.TrimSpace(lines[j]) != "" {
			rest = indentOf(lines[j])
			for ; j < len(lines) && strings.TrimSpace(lines[j]) != "" && indentOf(lines[j]) == rest; j++ {
				words = append(words, strings.Fields(lines[j])...)
			}
		}
		fill(first, rest, words)
		i = j
	}
	return bw.Flush()
}

// CommOptions controls Comm.
type CommOptions struct {
	Suppress1, Suppress2, Suppress3 bool   // Hide lines only in A, only in B, in both.
	Delimiter                       string // Column separator; default tab.
}

func lineReader(ctx context.Context, r io.Reader) func() (string, bool, error) {
	br := bufio.NewReaderSize(reader{ctx, r}, chunkSize)
	return func() (string, bool, error) {
		line, err := br.ReadString('\n')
		if err == io.EOF {
			if line == "" {
				return "", false, nil
			}
			err = nil
		}
		return strings.TrimSuffix(line, "\n"), err == nil, err
	}
}

// Comm compares two sorted inputs line by line and writes three columns:
// lines only in a, lines only in b, and lines in both. Comparison is bytewise.
func Comm(ctx context.Context, dst io.Writer, a, b io.Reader, opts CommOptions) error {
	delim := opts.Delimiter
	if delim == "" {
		delim = "\t"
	}
	prefix2, prefix3 := "", ""
	if !opts.Suppress1 {
		prefix2 += delim
		prefix3 += delim
	}
	if !opts.Suppress2 {
		prefix3 += delim
	}
	nextA, nextB := lineReader(ctx, a), lineReader(ctx, b)
	bw := bufio.NewWriter(writer{ctx, dst})
	la, okA, err := nextA()
	if err != nil {
		return err
	}
	lb, okB, err := nextB()
	if err != nil {
		return err
	}
	for okA || okB {
		var out string
		show := false
		switch {
		case okA && (!okB || la < lb):
			out, show = la, !opts.Suppress1
			la, okA, err = nextA()
		case okB && (!okA || lb < la):
			out, show = prefix2+lb, !opts.Suppress2
			lb, okB, err = nextB()
		default:
			out, show = prefix3+la, !opts.Suppress3
			if la, okA, err = nextA(); err == nil {
				lb, okB, err = nextB()
			}
		}
		if err != nil {
			return errors.Join(err, bw.Flush())
		}
		if show {
			if _, err := bw.WriteString(out + "\n"); err != nil {
				return err
			}
		}
	}
	return bw.Flush()
}

// JoinField selects an output field: File 0 is the join field, otherwise
// Field (1-based) of file 1 or 2.
type JoinField struct{ File, Field int }

// ParseJoinFields parses a join -o list such as "0,1.2,2.3".
func ParseJoinFields(list string) ([]JoinField, error) {
	var out []JoinField
	for _, f := range strings.FieldsFunc(list, func(r rune) bool { return r == ',' || r == ' ' }) {
		if f == "0" {
			out = append(out, JoinField{})
			continue
		}
		file, field, ok := strings.Cut(f, ".")
		n, err := strconv.Atoi(field)
		if !ok || (file != "1" && file != "2") || err != nil || n < 1 {
			return nil, fmt.Errorf("invalid field specifier %q", f)
		}
		out = append(out, JoinField{File: int(file[0] - '0'), Field: n})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("invalid field list %q", list)
	}
	return out, nil
}

// JoinOptions controls Join.
type JoinOptions struct {
	Field1, Field2 int    // 1-based join fields; default 1.
	Separator      string // Field separator; "" splits on runs of blanks and outputs a space.
	Unpaired1      bool   // Also print unpairable lines from file 1 (-a 1).
	Unpaired2      bool   // Also print unpairable lines from file 2 (-a 2).
	OnlyUnpaired   bool   // Print only the selected unpairable lines (-v).
	Output         []JoinField
	Empty          string // Replacement for missing fields named in Output.
	IgnoreCase     bool
}

// Join writes lines of two inputs, sorted on their join fields, that share a
// join field value. Inputs are held in memory. Groups of equal keys produce
// every pairing.
func Join(ctx context.Context, dst io.Writer, a, b io.Reader, opts JoinOptions) error {
	f1, f2 := max(opts.Field1, 1), max(opts.Field2, 1)
	linesA, err := ReadLines(ctx, a)
	if err != nil {
		return err
	}
	linesB, err := ReadLines(ctx, b)
	if err != nil {
		return err
	}
	split := func(s string) []string {
		if opts.Separator == "" {
			return strings.Fields(s)
		}
		return strings.Split(s, opts.Separator)
	}
	outSep := opts.Separator
	if outSep == "" {
		outSep = " "
	}
	fieldsA, fieldsB := make([][]string, len(linesA)), make([][]string, len(linesB))
	for i, l := range linesA {
		fieldsA[i] = split(l)
	}
	for i, l := range linesB {
		fieldsB[i] = split(l)
	}
	key := func(fields []string, n int) string {
		if n <= len(fields) {
			if opts.IgnoreCase {
				return strings.ToLower(fields[n-1])
			}
			return fields[n-1]
		}
		return ""
	}
	raw := func(fields []string, n int) string {
		if n <= len(fields) {
			return fields[n-1]
		}
		return ""
	}
	bw := bufio.NewWriter(writer{ctx, dst})
	emit := func(x, y []string) {
		var parts []string
		if opts.Output != nil {
			for _, o := range opts.Output {
				var v string
				var ok bool
				switch {
				case o.File == 0 && x != nil:
					v, ok = raw(x, f1), f1 <= len(x)
				case o.File == 0:
					v, ok = raw(y, f2), f2 <= len(y)
				case o.File == 1 && x != nil:
					v, ok = raw(x, o.Field), o.Field <= len(x)
				case o.File == 2 && y != nil:
					v, ok = raw(y, o.Field), o.Field <= len(y)
				}
				if !ok {
					v = opts.Empty
				}
				parts = append(parts, v)
			}
		} else {
			if x != nil {
				parts = append(parts, raw(x, f1))
			} else {
				parts = append(parts, raw(y, f2))
			}
			for i, f := range x {
				if i != f1-1 {
					parts = append(parts, f)
				}
			}
			for i, f := range y {
				if i != f2-1 {
					parts = append(parts, f)
				}
			}
		}
		bw.WriteString(strings.Join(parts, outSep) + "\n")
	}
	i, j := 0, 0
	for i < len(fieldsA) || j < len(fieldsB) {
		if err := ctx.Err(); err != nil {
			return err
		}
		var c int
		switch {
		case i >= len(fieldsA):
			c = 1
		case j >= len(fieldsB):
			c = -1
		default:
			c = strings.Compare(key(fieldsA[i], f1), key(fieldsB[j], f2))
		}
		if c < 0 {
			if opts.Unpaired1 {
				emit(fieldsA[i], nil)
			}
			i++
			continue
		}
		if c > 0 {
			if opts.Unpaired2 {
				emit(nil, fieldsB[j])
			}
			j++
			continue
		}
		k := key(fieldsA[i], f1)
		ei, ej := i, j
		for ei < len(fieldsA) && key(fieldsA[ei], f1) == k {
			ei++
		}
		for ej < len(fieldsB) && key(fieldsB[ej], f2) == k {
			ej++
		}
		if !opts.OnlyUnpaired {
			for _, x := range fieldsA[i:ei] {
				for _, y := range fieldsB[j:ej] {
					emit(x, y)
				}
			}
		}
		i, j = ei, ej
	}
	return bw.Flush()
}

// ShuffleOptions controls Shuffle.
type ShuffleOptions struct {
	Count  int        // Output at most Count items; negative means all.
	Repeat bool       // Pick with replacement; without a Count it runs until ctx ends or a write fails.
	Rand   *rand.Rand // Random source; nil uses a randomly seeded generator.
}

// Shuffle writes items in random order, one per line.
func Shuffle(ctx context.Context, dst io.Writer, items []string, opts ShuffleOptions) error {
	rng := opts.Rand
	if rng == nil {
		rng = rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))
	}
	bw := bufio.NewWriter(writer{ctx, dst})
	if opts.Repeat {
		if len(items) == 0 {
			if opts.Count > 0 {
				return errors.New("no lines to repeat")
			}
			return nil
		}
		for n := 0; opts.Count < 0 || n < opts.Count; n++ {
			if _, err := bw.WriteString(items[rng.IntN(len(items))] + "\n"); err != nil {
				return err
			}
		}
		return bw.Flush()
	}
	items = append([]string(nil), items...)
	count := len(items)
	if opts.Count >= 0 {
		count = min(count, opts.Count)
	}
	for i := 0; i < count; i++ {
		j := i + rng.IntN(len(items)-i)
		items[i], items[j] = items[j], items[i]
		if _, err := bw.WriteString(items[i] + "\n"); err != nil {
			return err
		}
	}
	return bw.Flush()
}

// ErrLoop reports that Tsort input contains a cycle.
var ErrLoop = errors.New("input contains a loop")

// Tsort reads pairs of whitespace-separated items, "a b" meaning a comes
// before b, and writes a total order, one item per line. Ready items are
// written in order of first appearance. On a cycle every item is still
// written and the error wraps ErrLoop and names the items in the loop.
func Tsort(ctx context.Context, dst io.Writer, src io.Reader) error {
	data, err := io.ReadAll(reader{ctx, src})
	if err != nil {
		return err
	}
	tokens := strings.Fields(string(data))
	if len(tokens)%2 != 0 {
		return errors.New("input contains an odd number of tokens")
	}
	index := map[string]int{}
	var names []string
	id := func(s string) int {
		if n, ok := index[s]; ok {
			return n
		}
		index[s] = len(names)
		names = append(names, s)
		return len(names) - 1
	}
	var succ [][]int
	var indeg []int
	seen := map[[2]int]bool{}
	for i := 0; i < len(tokens); i += 2 {
		a, b := id(tokens[i]), id(tokens[i+1])
		for len(succ) < len(names) {
			succ, indeg = append(succ, nil), append(indeg, 0)
		}
		if a != b && !seen[[2]int{a, b}] {
			seen[[2]int{a, b}] = true
			succ[a] = append(succ[a], b)
			indeg[b]++
		}
	}
	done := make([]bool, len(names))
	var queue []int
	for n := range names {
		if indeg[n] == 0 {
			queue = append(queue, n)
		}
	}
	bw := bufio.NewWriter(writer{ctx, dst})
	var loops []string
	for written := 0; written < len(names); {
		if len(queue) == 0 {
			// Break a cycle at the first remaining item.
			for n := range names {
				if !done[n] {
					loops = append(loops, names[n])
					queue = append(queue, n)
					indeg[n] = 0
					break
				}
			}
		}
		n := queue[0]
		queue = queue[1:]
		if done[n] {
			continue
		}
		done[n] = true
		written++
		if _, err := bw.WriteString(names[n] + "\n"); err != nil {
			return err
		}
		for _, m := range succ[n] {
			if indeg[m]--; indeg[m] == 0 && !done[m] {
				queue = append(queue, m)
			}
		}
	}
	if err := bw.Flush(); err != nil {
		return err
	}
	if loops != nil {
		return fmt.Errorf("%w involving %s", ErrLoop, strings.Join(loops, ", "))
	}
	return nil
}
