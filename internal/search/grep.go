// Package search implements grep-style matching and find-style walking.
package search

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"regexp/syntax"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Syntax selects how grep patterns are read.
type Syntax int

// Pattern syntaxes.
const (
	Basic    Syntax = iota // POSIX basic regular expressions (grep default)
	Extended               // POSIX extended regular expressions (grep -E)
	Fixed                  // literal strings (grep -F)
)

// PatternOptions controls Compile.
type PatternOptions struct {
	Syntax     Syntax
	IgnoreCase bool
	Word       bool // Match only whole words (letters, digits, underscore).
	Line       bool // Match only whole lines.
}

// Matcher matches lines against one or more patterns.
type Matcher struct {
	re   *regexp.Regexp
	word bool
}

// Compile builds a Matcher that matches when any pattern matches. An empty
// pattern matches every line; no patterns match nothing. Back-references are not supported because Go's
// RE2 engine has none. Matching is leftmost-longest, as in POSIX.
func Compile(patterns []string, opts PatternOptions) (*Matcher, error) {
	if len(patterns) == 0 { // e.g. grep -f with an empty file: match nothing
		return &Matcher{re: regexp.MustCompile(`[^\x00-\x{10FFFF}]`)}, nil
	}
	alts := make([]string, len(patterns))
	for i, p := range patterns {
		var expr string
		var err error
		switch opts.Syntax {
		case Fixed:
			expr = regexp.QuoteMeta(p)
		case Extended:
			expr, err = translate(p, true)
		default:
			expr, err = translate(p, false)
		}
		if err == nil {
			_, err = regexp.Compile(expr)
		}
		if err != nil {
			var syntaxErr *syntax.Error
			if errors.As(err, &syntaxErr) {
				err = fmt.Errorf("invalid pattern %q: %s", p, syntaxErr.Code)
			}
			return nil, err
		}
		alts[i] = "(?:" + expr + ")"
	}
	expr := strings.Join(alts, "|")
	if opts.Line {
		expr = "^(?:" + expr + ")$"
	}
	if opts.IgnoreCase {
		expr = "(?i)" + expr
	}
	re, err := regexp.Compile(expr)
	if err != nil {
		return nil, err
	}
	re.Longest()
	return &Matcher{re: re, word: opts.Word && !opts.Line}, nil
}

// CompileBRE compiles one POSIX basic regular expression to a leftmost-longest
// RE2 regexp, as used by expr and csplit.
func CompileBRE(p string) (*regexp.Regexp, error) {
	expr, err := translate(p, false)
	if err == nil {
		var re *regexp.Regexp
		if re, err = regexp.Compile(expr); err == nil {
			re.Longest()
			return re, nil
		}
	}
	var syntaxErr *syntax.Error
	if errors.As(err, &syntaxErr) {
		err = fmt.Errorf("invalid pattern %q: %s", p, syntaxErr.Code)
	}
	return nil, err
}

// translate converts a POSIX basic or extended expression to RE2 syntax,
// supporting the GNU extensions \< \> \b \B \w \W \s \S.
func translate(p string, extended bool) (string, error) {
	var b strings.Builder
	atStart := true // where '*' is literal (BRE) and '^' is an anchor
	for i := 0; i < len(p); i++ {
		c := p[i]
		start := atStart
		atStart = false
		switch {
		case c == '[':
			j := bracketEnd(p, i)
			if j < 0 {
				return "", fmt.Errorf("unmatched [ in %q", p)
			}
			b.WriteString(bracket(p[i : j+1]))
			i = j
		case c == '\\':
			if i+1 >= len(p) {
				return "", fmt.Errorf("trailing backslash in %q", p)
			}
			i++
			e := p[i]
			switch {
			case e >= '1' && e <= '9':
				return "", fmt.Errorf("back-references are not supported: \\%c", e)
			case e == '<' || e == '>':
				b.WriteString(`\b`)
			case strings.IndexByte("bBwWsS", e) >= 0:
				b.WriteByte('\\')
				b.WriteByte(e)
			case !extended && strings.IndexByte("(){}|+?", e) >= 0:
				b.WriteByte(e)
				atStart = e == '(' || e == '|'
			default:
				b.WriteString(regexp.QuoteMeta(string(e)))
			}
		case c == '^':
			if start || extended {
				b.WriteByte('^')
				atStart = true
			} else {
				b.WriteString(`\^`)
			}
		case c == '$':
			end := i+1 == len(p) || !extended && (strings.HasPrefix(p[i+1:], `\)`) || strings.HasPrefix(p[i+1:], `\|`))
			if end || extended {
				b.WriteByte('$')
			} else {
				b.WriteString(`\$`)
			}
		case c == '*' && start:
			b.WriteString(`\*`)
		case strings.IndexByte("(){}|+?", c) >= 0:
			if extended {
				if c == '{' && !validInterval(p[i:]) {
					b.WriteString(`\{`)
					continue
				}
				b.WriteByte(c)
				atStart = c == '(' || c == '|'
			} else {
				b.WriteString(regexp.QuoteMeta(string(c)))
			}
		default:
			b.WriteByte(c)
		}
	}
	return b.String(), nil
}

func validInterval(s string) bool {
	end := strings.IndexByte(s, '}')
	if end < 0 {
		return false
	}
	lo, hi, comma := strings.Cut(s[1:end], ",")
	if _, err := strconv.Atoi(lo); err != nil {
		return false
	}
	if comma && hi != "" {
		_, err := strconv.Atoi(hi)
		return err == nil
	}
	return true
}

// bracketEnd returns the index of the ']' closing the bracket expression at i.
func bracketEnd(p string, i int) int {
	j := i + 1
	if j < len(p) && p[j] == '^' {
		j++
	}
	if j < len(p) && p[j] == ']' {
		j++
	}
	for ; j < len(p); j++ {
		if p[j] == '[' && j+1 < len(p) && strings.IndexByte(":.=", p[j+1]) >= 0 {
			if k := strings.Index(p[j+2:], string(p[j+1])+"]"); k >= 0 {
				j += k + 3
				continue
			}
		}
		if p[j] == ']' {
			return j
		}
	}
	return -1
}

// bracket rewrites a POSIX bracket expression for RE2, where backslash is an
// escape inside brackets but literal in POSIX.
func bracket(s string) string {
	return strings.ReplaceAll(s, `\`, `\\`)
}

func isWord(r rune) bool { return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) }

// Find returns the [start, end) byte ranges of the matches in line.
func (m *Matcher) Find(line []byte) [][]int {
	all := m.re.FindAllIndex(line, -1)
	if !m.word {
		return all
	}
	var kept [][]int
	for _, loc := range all {
		before, _ := utf8.DecodeLastRune(line[:loc[0]])
		after, _ := utf8.DecodeRune(line[loc[1]:])
		if (loc[0] == 0 || !isWord(before)) && (loc[1] == len(line) || !isWord(after)) {
			kept = append(kept, loc)
		}
	}
	return kept
}

// Match reports whether line contains a match.
func (m *Matcher) Match(line []byte) bool {
	if !m.word {
		return m.re.Match(line)
	}
	return len(m.Find(line)) > 0
}

// GrepOptions controls Grep output.
type GrepOptions struct {
	Invert       bool   // Select non-matching lines.
	Count        bool   // Print only the number of selected lines.
	ListMatches  bool   // Print only Name, once, if a line is selected (-l).
	ListMissing  bool   // Print only Name if no line is selected (-L).
	LineNumbers  bool   // Prefix lines with their 1-based number.
	OnlyMatching bool   // Print each match on its own line (ignored with Invert).
	Quiet        bool   // Print nothing; stop at the first selected line.
	Name         string // Prefix for output lines when WithName is set.
	WithName     bool
	MaxCount     int64 // Stop after this many selected lines; 0 means no limit.
	Binary       bool  // Report "Binary file NAME matches" instead of lines when src has a NUL byte.
}

// Grep writes the selected lines of src and returns how many were selected.
// Output lines always end with a newline.
func Grep(ctx context.Context, dst io.Writer, src io.Reader, m *Matcher, opts GrepOptions) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	br := bufio.NewReaderSize(src, 64*1024)
	bw := bufio.NewWriter(dst)
	binary := false
	if opts.Binary {
		peek, _ := br.Peek(8 * 1024)
		binary = bytes.IndexByte(peek, 0) >= 0
	}
	quietOutput := opts.Quiet || opts.Count || opts.ListMatches || opts.ListMissing || binary
	prefix := func(n int64) {
		if opts.WithName {
			bw.WriteString(opts.Name)
			bw.WriteByte(':')
		}
		if opts.LineNumbers {
			bw.WriteString(strconv.FormatInt(n, 10))
			bw.WriteByte(':')
		}
	}
	var selected, lineNo int64
	var readErr error
	for opts.MaxCount == 0 || selected < opts.MaxCount {
		if err := ctx.Err(); err != nil {
			return selected, errors.Join(err, bw.Flush())
		}
		line, err := br.ReadBytes('\n')
		if len(line) == 0 && err != nil {
			if err != io.EOF {
				readErr = err
			}
			break
		}
		lineNo++
		line = bytes.TrimSuffix(line, []byte{'\n'})
		matches := m.Match(line)
		if matches != opts.Invert {
			selected++
			if opts.Quiet || opts.ListMatches || opts.ListMissing {
				break
			}
			if !quietOutput {
				if opts.OnlyMatching && !opts.Invert {
					for _, loc := range m.Find(line) {
						if loc[0] == loc[1] {
							continue
						}
						prefix(lineNo)
						bw.Write(line[loc[0]:loc[1]])
						bw.WriteByte('\n')
					}
				} else {
					prefix(lineNo)
					bw.Write(line)
					bw.WriteByte('\n')
				}
			}
			if bw.Buffered() > 32*1024 {
				if err := bw.Flush(); err != nil {
					return selected, err
				}
			}
		}
		if err != nil {
			if err != io.EOF {
				readErr = err
			}
			break
		}
	}
	switch {
	case opts.Quiet:
	case opts.Count:
		if opts.WithName {
			fmt.Fprintf(bw, "%s:", opts.Name)
		}
		fmt.Fprintf(bw, "%d\n", selected)
	case opts.ListMatches && selected > 0, opts.ListMissing && selected == 0:
		fmt.Fprintf(bw, "%s\n", opts.Name)
	case binary && selected > 0 && !opts.ListMissing && !opts.ListMatches:
		fmt.Fprintf(bw, "Binary file %s matches\n", opts.Name)
	}
	return selected, errors.Join(readErr, bw.Flush())
}
