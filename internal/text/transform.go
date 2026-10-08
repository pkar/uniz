package text

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// mapLines applies fn to each line without its newline. When keep is true the
// result is written, followed by a newline only if the input line had one.
func mapLines(ctx context.Context, dst io.Writer, src io.Reader, fn func(line string) (string, bool)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	br := bufio.NewReaderSize(reader{ctx, src}, chunkSize)
	bw := bufio.NewWriterSize(writer{ctx, dst}, chunkSize)
	for {
		line, err := br.ReadString('\n')
		if len(line) > 0 {
			body, newline := strings.CutSuffix(line, "\n")
			if out, keep := fn(body); keep {
				if newline {
					out += "\n"
				}
				if _, werr := bw.WriteString(out); werr != nil {
					return werr
				}
			}
		}
		if err == io.EOF {
			return bw.Flush()
		}
		if err != nil {
			return errors.Join(err, bw.Flush())
		}
	}
}

// units splits s into runes, keeping each invalid UTF-8 byte as its own unit.
func units(s string) []string {
	out := make([]string, 0, len(s))
	for len(s) > 0 {
		_, size := utf8.DecodeRuneInString(s)
		out, s = append(out, s[:size]), s[size:]
	}
	return out
}

// Range is an inclusive 1-based position range. End 0 means to the end.
type Range struct{ Start, End int }

// ParseRanges parses a cut(1) list such as "1,3-5,7-" or "-2".
func ParseRanges(list string) ([]Range, error) {
	var out []Range
	for _, part := range strings.Split(list, ",") {
		bad := fmt.Errorf("invalid list value %q", part)
		num := func(s string) (int, error) {
			n, err := strconv.Atoi(s)
			if err != nil || n < 1 {
				return 0, bad
			}
			return n, nil
		}
		lo, hi, isRange := strings.Cut(part, "-")
		var r Range
		var err error
		switch {
		case !isRange:
			r.Start, err = num(lo)
			r.End = r.Start
		case lo == "" && hi == "":
			err = bad
		case lo == "":
			r.Start = 1
			r.End, err = num(hi)
		case hi == "":
			r.Start, err = num(lo)
		default:
			if r.Start, err = num(lo); err == nil {
				r.End, err = num(hi)
			}
			if err == nil && r.End < r.Start {
				err = fmt.Errorf("decreasing range %q", part)
			}
		}
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

// CutUnit selects what cut positions count.
type CutUnit int

// Cut units.
const (
	CutBytes CutUnit = iota
	CutChars
	CutFields
)

// CutOptions controls Cut. Delimiter must be one character; empty means tab.
type CutOptions struct {
	Unit          CutUnit
	Ranges        []Range
	Delimiter     string
	OnlyDelimited bool // With CutFields, drop lines that contain no delimiter.
}

// Cut writes the selected bytes, characters, or fields of each line, in input
// order. Lines without a delimiter are printed whole in field mode unless
// OnlyDelimited is set.
func Cut(ctx context.Context, dst io.Writer, src io.Reader, opts CutOptions) error {
	if len(opts.Ranges) == 0 {
		return errors.New("cut: no ranges")
	}
	delim := opts.Delimiter
	if delim == "" {
		delim = "\t"
	}
	if utf8.RuneCountInString(delim) != 1 {
		return fmt.Errorf("cut: delimiter %q must be a single character", delim)
	}
	selected := func(i int) bool {
		for _, r := range opts.Ranges {
			if i >= r.Start && (r.End == 0 || i <= r.End) {
				return true
			}
		}
		return false
	}
	pick := func(parts []string, sep string) string {
		var kept []string
		for i, p := range parts {
			if selected(i + 1) {
				kept = append(kept, p)
			}
		}
		return strings.Join(kept, sep)
	}
	return mapLines(ctx, dst, src, func(line string) (string, bool) {
		switch opts.Unit {
		case CutBytes:
			parts := make([]string, len(line))
			for i := range line {
				parts[i] = line[i : i+1]
			}
			return pick(parts, ""), true
		case CutChars:
			return pick(units(line), ""), true
		}
		if !strings.Contains(line, delim) {
			return line, !opts.OnlyDelimited
		}
		return pick(strings.Split(line, delim), delim), true
	})
}

var setEscapes = map[rune]rune{'\\': '\\', 'a': 7, 'b': 8, 'f': 12, 'n': 10, 'r': 13, 't': 9, 'v': 11}

var setClasses = map[string]func(rune) bool{
	"alnum":  func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) },
	"alpha":  unicode.IsLetter,
	"blank":  func(r rune) bool { return r == ' ' || r == '\t' },
	"cntrl":  func(r rune) bool { return r < 32 || r == 127 },
	"digit":  unicode.IsDigit,
	"graph":  func(r rune) bool { return r > 32 && r < 127 },
	"lower":  unicode.IsLower,
	"print":  func(r rune) bool { return r >= 32 && r < 127 },
	"punct":  func(r rune) bool { return r > 32 && r < 127 && !unicode.IsLetter(r) && !unicode.IsDigit(r) },
	"space":  func(r rune) bool { return r == ' ' || r >= 9 && r <= 13 },
	"upper":  unicode.IsUpper,
	"xdigit": func(r rune) bool { return r >= '0' && r <= '9' || r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F' },
}

// ExpandSet expands a tr(1) set: characters, backslash escapes (\n, \t,
// \\, \NNN octal, ...), ranges such as a-z, and ASCII classes such as
// [:upper:]. Classes expand in ascending ASCII order.
func ExpandSet(set string) ([]rune, error) {
	rs := []rune(set)
	read := func(i int) (rune, int) {
		if rs[i] != '\\' || i+1 >= len(rs) {
			return rs[i], i + 1
		}
		i++
		if c, ok := setEscapes[rs[i]]; ok {
			return c, i + 1
		}
		if rs[i] >= '0' && rs[i] <= '7' {
			v, j := rune(0), i
			for ; j < len(rs) && j < i+3 && rs[j] >= '0' && rs[j] <= '7'; j++ {
				v = v*8 + rs[j] - '0'
			}
			return v, j
		}
		return rs[i], i + 1
	}
	var out []rune
	for i := 0; i < len(rs); {
		if rs[i] == '[' && i+1 < len(rs) && rs[i+1] == ':' {
			if end := strings.Index(string(rs[i+2:]), ":]"); end >= 0 {
				name := string(rs[i+2:])[:end]
				class, ok := setClasses[name]
				if !ok {
					return nil, fmt.Errorf("invalid character class %q", name)
				}
				for r := rune(0); r < 128; r++ {
					if class(r) {
						out = append(out, r)
					}
				}
				i += 2 + utf8.RuneCountInString(name) + 2
				continue
			}
		}
		c, next := read(i)
		if next+1 < len(rs) && rs[next] == '-' {
			hi, after := read(next + 1)
			if hi < c {
				return nil, fmt.Errorf("range %q-%q is in reverse order", c, hi)
			}
			for r := c; r <= hi; r++ {
				out = append(out, r)
			}
			i = after
			continue
		}
		out, i = append(out, c), next
	}
	return out, nil
}

// TranslateOptions controls Translate.
type TranslateOptions struct {
	Delete     bool // Delete characters in set1.
	Squeeze    bool // Collapse runs of a repeated character from the last given set.
	Complement bool // Use every character not in set1 instead of set1.
}

// Translate implements tr(1) on Unicode characters. Without Delete, a
// non-empty set2 translates set1 positionally; a shorter set2 is padded with
// its last character. Squeeze applies to set2 when given, else set1. Invalid
// UTF-8 bytes are copied unchanged unless deleted through Complement.
func Translate(ctx context.Context, dst io.Writer, src io.Reader, set1, set2 string, opts TranslateOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	a, err := ExpandSet(set1)
	if err != nil {
		return err
	}
	b, err := ExpandSet(set2)
	if err != nil {
		return err
	}
	translating := !opts.Delete && set2 != ""
	if translating && len(b) == 0 || !opts.Delete && !opts.Squeeze && !translating {
		return errors.New("tr: set2 must be non-empty when translating")
	}
	member := map[rune]bool{}
	for _, r := range a {
		member[r] = true
	}
	inSet1 := func(r rune) bool { return member[r] != opts.Complement }
	mapping := map[rune]rune{}
	for i, r := range a {
		if len(b) > 0 {
			mapping[r] = b[min(i, len(b)-1)]
		}
	}
	translate := func(r rune) rune {
		if opts.Complement {
			return b[len(b)-1]
		}
		return mapping[r]
	}
	squeezeSet, squeezeComplement := b, false
	if set2 == "" {
		squeezeSet, squeezeComplement = a, opts.Complement
	}
	squeeze := map[rune]bool{}
	for _, r := range squeezeSet {
		squeeze[r] = true
	}
	br := bufio.NewReaderSize(reader{ctx, src}, chunkSize)
	bw := bufio.NewWriterSize(writer{ctx, dst}, chunkSize)
	last, haveLast := rune(0), false
	for {
		peek, _ := br.Peek(1)
		var first byte
		if len(peek) > 0 {
			first = peek[0]
		}
		r, size, err := br.ReadRune()
		if err == io.EOF {
			return bw.Flush()
		}
		if err != nil {
			return errors.Join(err, bw.Flush())
		}
		if opts.Delete && inSet1(r) {
			continue
		}
		out := string(r)
		if r == utf8.RuneError && size == 1 {
			out = string([]byte{first})
		}
		if translating && inSet1(r) {
			r = translate(r)
			out = string(r)
		}
		if opts.Squeeze && squeeze[r] != squeezeComplement && haveLast && r == last {
			continue
		}
		last, haveLast = r, true
		if _, err := bw.WriteString(out); err != nil {
			return err
		}
	}
}

// PasteOptions controls Paste. Delimiters are used cyclically between
// fields; nil means a tab. Empty strings are allowed.
type PasteOptions struct {
	Delimiters []string
	Serial     bool // Join all lines of each input into one line.
}

// Paste merges corresponding lines of the inputs. Passing the same reader
// more than once reads successive lines from it, like "paste - -".
func Paste(ctx context.Context, dst io.Writer, srcs []io.Reader, opts PasteOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	delims := opts.Delimiters
	if len(delims) == 0 {
		delims = []string{"\t"}
	}
	shared := map[io.Reader]*bufio.Reader{}
	readers := make([]*bufio.Reader, len(srcs))
	for i, src := range srcs {
		if shared[src] == nil {
			shared[src] = bufio.NewReaderSize(reader{ctx, src}, chunkSize)
		}
		readers[i] = shared[src]
	}
	bw := bufio.NewWriterSize(writer{ctx, dst}, chunkSize)
	readLine := func(r *bufio.Reader) (string, bool, error) {
		line, err := r.ReadString('\n')
		if err == io.EOF {
			err = nil
			if line == "" {
				return "", false, nil
			}
		}
		return strings.TrimSuffix(line, "\n"), true, err
	}
	if opts.Serial {
		for _, r := range readers {
			for n := 0; ; n++ {
				line, ok, err := readLine(r)
				if err != nil {
					return errors.Join(err, bw.Flush())
				}
				if !ok {
					break
				}
				if n > 0 {
					bw.WriteString(delims[(n-1)%len(delims)])
				}
				bw.WriteString(line)
			}
			if err := bw.WriteByte('\n'); err != nil {
				return err
			}
		}
		return bw.Flush()
	}
	for {
		var fields []string
		any := false
		for _, r := range readers {
			line, ok, err := readLine(r)
			if err != nil {
				return errors.Join(err, bw.Flush())
			}
			any = any || ok
			fields = append(fields, line)
		}
		if !any {
			return bw.Flush()
		}
		for i, f := range fields {
			if i > 0 {
				bw.WriteString(delims[(i-1)%len(delims)])
			}
			bw.WriteString(f)
		}
		if err := bw.WriteByte('\n'); err != nil {
			return err
		}
	}
}

// NumberStyle selects which lines NumberLines numbers.
type NumberStyle int

// Numbering styles.
const (
	NumberNonEmpty NumberStyle = iota // nl -b t
	NumberAll                         // nl -b a
	NumberNone                        // nl -b n
)

// NumberOptions controls NumberLines. Use DefaultNumberOptions for nl's
// defaults; zero Width or Increment fall back to 6 and 1.
type NumberOptions struct {
	Style     NumberStyle
	Width     int
	Separator string
	Start     int64
	Increment int64
}

// DefaultNumberOptions returns nl's defaults: non-empty lines, width 6,
// a tab separator, starting at 1.
func DefaultNumberOptions() NumberOptions {
	return NumberOptions{Width: 6, Separator: "\t", Start: 1, Increment: 1}
}

// NumberLines writes src with right-aligned line numbers and returns the next
// number, so numbering can continue across inputs. Unnumbered lines are
// indented by Width plus the separator length, as nl does.
func NumberLines(ctx context.Context, dst io.Writer, src io.Reader, opts NumberOptions) (int64, error) {
	width, inc := opts.Width, opts.Increment
	if width < 1 {
		width = 6
	}
	if inc == 0 {
		inc = 1
	}
	n := opts.Start
	blank := strings.Repeat(" ", width+len(opts.Separator))
	err := mapLines(ctx, dst, src, func(line string) (string, bool) {
		if opts.Style == NumberAll || opts.Style == NumberNonEmpty && line != "" {
			out := fmt.Sprintf("%*d%s%s", width, n, opts.Separator, line)
			n += inc
			return out, true
		}
		return blank + line, true
	})
	return n, err
}

// ReverseLines writes the lines of src in reverse order (tac). Every output
// line ends with a newline. Input is held in memory; nothing is written if
// reading fails.
func ReverseLines(ctx context.Context, dst io.Writer, src io.Reader) error {
	lines, err := ReadLines(ctx, src)
	if err != nil {
		return err
	}
	slices.Reverse(lines)
	return WriteLines(ctx, dst, lines)
}

// ReverseCharacters reverses the characters of each line (rev). Invalid
// UTF-8 bytes are moved as single units.
func ReverseCharacters(ctx context.Context, dst io.Writer, src io.Reader) error {
	return mapLines(ctx, dst, src, func(line string) (string, bool) {
		u := units(line)
		slices.Reverse(u)
		return strings.Join(u, ""), true
	})
}

// FoldOptions controls Fold. Width 0 means 80.
type FoldOptions struct {
	Width  int
	Bytes  bool // Count bytes instead of characters.
	Spaces bool // Break after the last blank within the width when possible.
}

// Fold wraps lines longer than Width. Every character, including tab, counts
// as one column.
func Fold(ctx context.Context, dst io.Writer, src io.Reader, opts FoldOptions) error {
	width := opts.Width
	if width < 1 {
		width = 80
	}
	return mapLines(ctx, dst, src, func(line string) (string, bool) {
		var parts []string
		if opts.Bytes {
			for i := range line {
				parts = append(parts, line[i:i+1])
			}
		} else {
			parts = units(line)
		}
		var b strings.Builder
		for len(parts) > width {
			cut := width
			if opts.Spaces {
				for j := width - 1; j >= 0; j-- {
					if parts[j] == " " || parts[j] == "\t" {
						cut = j + 1
						break
					}
				}
			}
			b.WriteString(strings.Join(parts[:cut], ""))
			b.WriteByte('\n')
			parts = parts[cut:]
		}
		b.WriteString(strings.Join(parts, ""))
		return b.String(), true
	})
}
