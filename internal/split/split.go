// Package split implements split and csplit, which write pieces of an input
// to numbered files.
package split

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/pkar/uniz/internal/search"
)

// create opens path for writing, truncating a regular file but refusing to
// write through a symlink or into a non-regular file.
func create(path string) (*os.File, error) {
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&fs.ModeSymlink != 0 {
			return nil, fmt.Errorf("%s: refusing to write through a symbolic link", path)
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("%s: not a regular file", path)
		}
	}
	return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o666)
}

// Suffix returns the n-th (0-based) output suffix of the given length:
// aa, ab, ... or 00, 01, ... when numeric.
func Suffix(n, length int, numeric bool) (string, error) {
	base, digits := 26, "abcdefghijklmnopqrstuvwxyz"
	if numeric {
		base, digits = 10, "0123456789"
	}
	b := make([]byte, length)
	for i := length - 1; i >= 0; i-- {
		b[i] = digits[n%base]
		n /= base
	}
	if n > 0 {
		return "", errors.New("output file suffixes exhausted")
	}
	return string(b), nil
}

// Options controls Split. Exactly one of Lines, Bytes, or Chunks is used, in
// that order of preference; all zero means 1000 lines.
type Options struct {
	Lines        int64
	Bytes        int64
	Chunks       int   // Split into this many files of near-equal size.
	Size         int64 // Input size, required with Chunks.
	Prefix       string
	SuffixLength int // Default 2.
	Numeric      bool
}

// Split writes src to files named Prefix+suffix and returns their names.
// Existing regular files are overwritten; symlinks are refused.
func Split(ctx context.Context, src io.Reader, opts Options) ([]string, error) {
	if opts.Prefix == "" {
		opts.Prefix = "x"
	}
	if opts.SuffixLength <= 0 {
		opts.SuffixLength = 2
	}
	var names []string
	var cur *os.File
	next := func() error {
		if cur != nil {
			if err := cur.Close(); err != nil {
				return err
			}
		}
		suffix, err := Suffix(len(names), opts.SuffixLength, opts.Numeric)
		if err != nil {
			return err
		}
		name := opts.Prefix + suffix
		if cur, err = create(name); err != nil {
			return err
		}
		names = append(names, name)
		return nil
	}
	err := func() error {
		r := bufio.NewReader(src)
		switch {
		case opts.Lines > 0 || (opts.Bytes <= 0 && opts.Chunks <= 0):
			limit := opts.Lines
			if limit <= 0 {
				limit = 1000
			}
			count := limit
			for {
				if err := ctx.Err(); err != nil {
					return err
				}
				line, err := r.ReadBytes('\n')
				if len(line) > 0 {
					if count == limit {
						if err := next(); err != nil {
							return err
						}
						count = 0
					}
					if _, err := cur.Write(line); err != nil {
						return err
					}
					count++
				}
				if err == io.EOF {
					return nil
				}
				if err != nil {
					return err
				}
			}
		case opts.Bytes > 0:
			for {
				if err := ctx.Err(); err != nil {
					return err
				}
				if _, err := r.Peek(1); err == io.EOF {
					return nil
				} else if err != nil {
					return err
				}
				if err := next(); err != nil {
					return err
				}
				if _, err := io.CopyN(cur, r, opts.Bytes); err != nil && err != io.EOF {
					return err
				}
			}
		default:
			per := opts.Size / int64(opts.Chunks)
			for i := range opts.Chunks {
				if err := ctx.Err(); err != nil {
					return err
				}
				if err := next(); err != nil {
					return err
				}
				n := per
				if i == opts.Chunks-1 {
					n = opts.Size - per*int64(opts.Chunks-1)
				}
				if _, err := io.CopyN(cur, r, n); err != nil && err != io.EOF {
					return err
				}
			}
			return nil
		}
	}()
	if cur != nil {
		err = errors.Join(err, cur.Close())
	}
	return names, err
}

// CsplitOptions controls Csplit.
type CsplitOptions struct {
	Prefix     string // Default "xx".
	Digits     int    // Suffix digits; default 2.
	KeepFiles  bool   // Keep files already written when an error occurs.
	ElideEmpty bool   // Do not write empty pieces.
}

type csplitPattern struct {
	line   int            // line-number pattern when re is nil
	re     *regexp.Regexp // regexp pattern
	skip   bool           // %RE%: discard instead of writing
	offset int
	repeat int // extra repetitions; -1 means as many as match
	text   string
}

func parsePatterns(args []string) ([]csplitPattern, error) {
	var out []csplitPattern
	for _, a := range args {
		if strings.HasPrefix(a, "{") && strings.HasSuffix(a, "}") {
			if len(out) == 0 {
				return nil, fmt.Errorf("%s: repeat count without a pattern", a)
			}
			n := -1
			if inner := a[1 : len(a)-1]; inner != "*" {
				var err error
				if n, err = strconv.Atoi(inner); err != nil || n < 0 {
					return nil, fmt.Errorf("%s: invalid repeat count", a)
				}
			}
			out[len(out)-1].repeat = n
			continue
		}
		p := csplitPattern{text: a}
		if a != "" && (a[0] == '/' || a[0] == '%') {
			end := strings.LastIndexByte(a, a[0])
			if end == 0 {
				return nil, fmt.Errorf("%s: missing closing delimiter", a)
			}
			re, err := search.CompileBRE(a[1:end])
			if err != nil {
				return nil, err
			}
			p.re, p.skip = re, a[0] == '%'
			if off := a[end+1:]; off != "" {
				if p.offset, err = strconv.Atoi(off); err != nil {
					return nil, fmt.Errorf("%s: invalid offset", a)
				}
			}
		} else {
			n, err := strconv.Atoi(a)
			if err != nil || n < 1 {
				return nil, fmt.Errorf("%s: invalid pattern", a)
			}
			p.line = n
		}
		out = append(out, p)
	}
	return out, nil
}

// Csplit splits src (held in memory) at the given patterns:
//
//	N          before line N
//	/RE/[OFF]  before the next line matching the basic regexp, plus OFF lines
//	%RE%[OFF]  like /RE/ but discard the skipped lines
//	{N} {*}    repeat the previous pattern N more times, or until no match
//
// The rest of the input goes to a final piece. It returns the files written
// and their sizes. On error, written files are removed unless KeepFiles.
func Csplit(ctx context.Context, src io.Reader, patterns []string, opts CsplitOptions) (names []string, sizes []int64, err error) {
	pats, err := parsePatterns(patterns)
	if err != nil {
		return nil, nil, err
	}
	if opts.Prefix == "" {
		opts.Prefix = "xx"
	}
	if opts.Digits <= 0 {
		opts.Digits = 2
	}
	data, err := io.ReadAll(src)
	if err != nil {
		return nil, nil, err
	}
	var lines []string
	for s := string(data); s != ""; {
		i := strings.IndexByte(s, '\n')
		if i < 0 {
			lines, s = append(lines, s), ""
		} else {
			lines, s = append(lines, s[:i+1]), s[i+1:]
		}
	}
	defer func() {
		if err != nil && !opts.KeepFiles {
			for _, n := range names {
				os.Remove(n)
			}
			names, sizes = nil, nil
		}
	}()
	piece := 0
	writePiece := func(part []string) error {
		var size int64
		for _, l := range part {
			size += int64(len(l))
		}
		if size == 0 && opts.ElideEmpty {
			return nil
		}
		name := fmt.Sprintf("%s%0*d", opts.Prefix, opts.Digits, piece)
		if len(name)-len(opts.Prefix) > opts.Digits {
			return errors.New("output file suffixes exhausted")
		}
		piece++
		f, err := create(name)
		if err != nil {
			return err
		}
		names = append(names, name)
		w := bufio.NewWriter(f)
		for _, l := range part {
			w.WriteString(l)
		}
		if err := errors.Join(w.Flush(), f.Close()); err != nil {
			return err
		}
		sizes = append(sizes, size)
		return nil
	}
	cur, lastSplit := 0, -1
	for _, p := range pats {
		for rep := 0; p.repeat < 0 || rep <= p.repeat; rep++ {
			if err := ctx.Err(); err != nil {
				return names, sizes, err
			}
			var target int
			if p.re == nil {
				target = p.line - 1 + rep*p.line
				if target <= lastSplit || target > len(lines) {
					return names, sizes, fmt.Errorf("%s: line number out of range", p.text)
				}
			} else {
				start := cur
				if cur == lastSplit {
					start++ // the line we split at matched last time
				}
				match := -1
				for i := start; i < len(lines); i++ {
					if p.re.MatchString(strings.TrimSuffix(lines[i], "\n")) {
						match = i
						break
					}
				}
				if match < 0 {
					if p.repeat < 0 {
						break
					}
					return names, sizes, fmt.Errorf("%s: match not found", p.text)
				}
				target = match + p.offset
				if target < cur || target > len(lines) {
					return names, sizes, fmt.Errorf("%s: line number out of range", p.text)
				}
			}
			if p.skip {
				cur = target
			} else {
				if err := writePiece(lines[cur:target]); err != nil {
					return names, sizes, err
				}
				cur = target
			}
			lastSplit = target
			if p.re == nil && p.repeat < 0 && target+p.line > len(lines) {
				break
			}
		}
	}
	if err := writePiece(lines[cur:]); err != nil {
		return names, sizes, err
	}
	return names, sizes, nil
}
