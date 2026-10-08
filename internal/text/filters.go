package text

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"math"
	"slices"
	"strconv"
	"strings"
)

// ErrNegativeCount reports a negative Head or Tail count.
var ErrNegativeCount = errors.New("count must not be negative")

// ErrZeroStep reports a zero sequence increment.
var ErrZeroStep = errors.New("step must not be zero")

const chunkSize = 32 * 1024

// SliceOptions selects lines (default) or bytes for Head and Tail.
type SliceOptions struct {
	Count     int64 // Number of lines or bytes.
	Bytes     bool  // Count bytes instead of newline-terminated lines.
	FromStart bool  // Tail only: start at line/byte Count (1-based), like tail -n +N.
}

// Head copies the first Count lines or bytes of src.
func Head(ctx context.Context, dst io.Writer, src io.Reader, opts SliceOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if opts.Count < 0 {
		return ErrNegativeCount
	}
	w, r := writer{ctx, dst}, reader{ctx, src}
	if opts.Bytes {
		_, err := io.CopyN(w, r, opts.Count)
		if err == io.EOF {
			return nil
		}
		return err
	}
	remaining := opts.Count
	buf := make([]byte, chunkSize)
	for remaining > 0 {
		n, err := r.Read(buf)
		chunk := buf[:n]
		end := len(chunk)
		for off := 0; remaining > 0; {
			i := bytes.IndexByte(chunk[off:], '\n')
			if i < 0 {
				break
			}
			off += i + 1
			if remaining--; remaining == 0 {
				end = off
			}
		}
		if end > 0 {
			if _, werr := w.Write(chunk[:end]); werr != nil {
				return werr
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// Tail copies the last Count lines or bytes of src, or everything from
// position Count when FromStart is set. Memory is bounded by the retained
// output. Nothing is written if reading fails before EOF.
func Tail(ctx context.Context, dst io.Writer, src io.Reader, opts SliceOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if opts.Count < 0 {
		return ErrNegativeCount
	}
	w, r := writer{ctx, dst}, reader{ctx, src}
	if opts.FromStart {
		return skipThenCopy(w, r, max(opts.Count-1, 0), opts.Bytes)
	}
	if opts.Count == 0 {
		return nil
	}
	if opts.Bytes {
		var keep []byte
		buf := make([]byte, chunkSize)
		for {
			n, err := r.Read(buf)
			keep = append(keep, buf[:n]...)
			if int64(len(keep)) > opts.Count {
				keep = keep[int64(len(keep))-opts.Count:]
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}
		}
		_, err := w.Write(keep)
		return err
	}
	br := bufio.NewReaderSize(r, chunkSize)
	var lines [][]byte
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			lines = append(lines, line)
			if int64(len(lines)) > opts.Count {
				lines[0] = nil
				lines = lines[1:]
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	bw := bufio.NewWriterSize(w, chunkSize)
	for _, line := range lines {
		if _, err := bw.Write(line); err != nil {
			return err
		}
	}
	return bw.Flush()
}

func skipThenCopy(w io.Writer, r io.Reader, skip int64, byBytes bool) error {
	buf := make([]byte, chunkSize)
	for {
		n, err := r.Read(buf)
		chunk := buf[:n]
		for skip > 0 && len(chunk) > 0 {
			if byBytes {
				k := min(int64(len(chunk)), skip)
				chunk, skip = chunk[k:], skip-k
				continue
			}
			i := bytes.IndexByte(chunk, '\n')
			if i < 0 {
				chunk = nil
				break
			}
			chunk, skip = chunk[i+1:], skip-1
		}
		if len(chunk) > 0 {
			if _, werr := w.Write(chunk); werr != nil {
				return werr
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// ReadLines reads lines without their trailing newline. A final unterminated
// line is included. Lines read before an error are returned with it.
func ReadLines(ctx context.Context, src io.Reader) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	br := bufio.NewReaderSize(reader{ctx, src}, chunkSize)
	var lines []string
	for {
		line, err := br.ReadString('\n')
		if len(line) > 0 {
			lines = append(lines, strings.TrimSuffix(line, "\n"))
		}
		if err == io.EOF {
			return lines, nil
		}
		if err != nil {
			return lines, err
		}
	}
}

// WriteLines writes each line followed by a newline.
func WriteLines(ctx context.Context, dst io.Writer, lines []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	bw := bufio.NewWriterSize(writer{ctx, dst}, chunkSize)
	for _, line := range lines {
		if _, err := bw.WriteString(line); err != nil {
			return err
		}
		if err := bw.WriteByte('\n'); err != nil {
			return err
		}
	}
	return bw.Flush()
}

// SortOptions controls line ordering.
type SortOptions struct {
	Reverse    bool // Reverse the final order.
	Numeric    bool // Compare leading numbers as float64; non-numeric lines are 0.
	Unique     bool // Keep only the first line of each run of equal keys.
	IgnoreCase bool // Compare with Unicode upper-case folding.
}

// SortLines returns a sorted copy. Lines with equal keys are ordered by byte
// value unless Unique is set; Unique keeps the first input line of each key.
func SortLines(lines []string, opts SortOptions) []string {
	out := slices.Clone(lines)
	key := func(a, b string) int {
		switch {
		case opts.Numeric:
			x, y := numericPrefix(a), numericPrefix(b)
			switch {
			case x < y:
				return -1
			case x > y:
				return 1
			}
			return 0
		case opts.IgnoreCase:
			return strings.Compare(strings.ToUpper(a), strings.ToUpper(b))
		}
		return strings.Compare(a, b)
	}
	slices.SortStableFunc(out, func(a, b string) int {
		c := key(a, b)
		if c == 0 && !opts.Unique {
			c = strings.Compare(a, b)
		}
		if opts.Reverse {
			c = -c
		}
		return c
	})
	if opts.Unique {
		out = slices.CompactFunc(out, func(a, b string) bool { return key(a, b) == 0 })
	}
	return out
}

// numericPrefix parses optional blanks, '-', digits, and a decimal fraction.
func numericPrefix(s string) float64 {
	s = strings.TrimLeft(s, " \t")
	end := 0
	if end < len(s) && s[end] == '-' {
		end++
	}
	digits := func() {
		for end < len(s) && s[end] >= '0' && s[end] <= '9' {
			end++
		}
	}
	digits()
	if end < len(s) && s[end] == '.' {
		end++
		digits()
	}
	v, err := strconv.ParseFloat(s[:end], 64)
	if err != nil {
		return 0
	}
	return v
}

// Sort reads all lines from src, sorts them, and writes them with newlines.
// Nothing is written if reading fails.
func Sort(ctx context.Context, dst io.Writer, src io.Reader, opts SortOptions) error {
	lines, err := ReadLines(ctx, src)
	if err != nil {
		return err
	}
	return WriteLines(ctx, dst, SortLines(lines, opts))
}

// UniqOptions controls adjacent-duplicate filtering.
type UniqOptions struct {
	Count      bool // Prefix lines with their run length.
	Repeated   bool // Print only runs longer than one line.
	Unique     bool // Print only runs of exactly one line.
	IgnoreCase bool // Compare with Unicode case folding.
}

// Uniq collapses adjacent equal lines. A missing final newline is ignored for
// comparison; every output line ends with a newline.
func Uniq(ctx context.Context, dst io.Writer, src io.Reader, opts UniqOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	br := bufio.NewReaderSize(reader{ctx, src}, chunkSize)
	bw := bufio.NewWriterSize(writer{ctx, dst}, chunkSize)
	var prev string
	var count int64
	flush := func() error {
		if count == 0 || opts.Repeated && count < 2 || opts.Unique && count > 1 {
			return nil
		}
		if opts.Count {
			_, err := fmt.Fprintf(bw, "%7d %s\n", count, prev)
			return err
		}
		_, err := bw.WriteString(prev + "\n")
		return err
	}
	equal := func(a, b string) bool {
		if opts.IgnoreCase {
			return strings.EqualFold(a, b)
		}
		return a == b
	}
	for {
		line, err := br.ReadString('\n')
		if len(line) > 0 {
			line = strings.TrimSuffix(line, "\n")
			if count > 0 && equal(prev, line) {
				count++
			} else {
				if ferr := flush(); ferr != nil {
					return ferr
				}
				prev, count = line, 1
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return errors.Join(err, flush(), bw.Flush())
		}
	}
	if err := flush(); err != nil {
		return err
	}
	return bw.Flush()
}

// SeqOptions describes an integer sequence. Separator is written verbatim
// between numbers; output always ends with a newline when non-empty.
type SeqOptions struct {
	First, Step, Last int64
	Separator         string
}

// Sequence yields first, first+step, ... while not past last. A zero step
// yields nothing. Iteration stops before int64 overflow.
func Sequence(first, step, last int64) iter.Seq[int64] {
	return func(yield func(int64) bool) {
		if step == 0 {
			return
		}
		for i := first; step > 0 && i <= last || step < 0 && i >= last; i += step {
			if !yield(i) {
				return
			}
			if step > 0 && i > math.MaxInt64-step || step < 0 && i < math.MinInt64-step {
				return
			}
		}
	}
}

// Seq writes a sequence in decimal.
func Seq(ctx context.Context, dst io.Writer, opts SeqOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if opts.Step == 0 {
		return ErrZeroStep
	}
	bw := bufio.NewWriterSize(writer{ctx, dst}, chunkSize)
	first := true
	for i := range Sequence(opts.First, opts.Step, opts.Last) {
		if !first {
			if _, err := bw.WriteString(opts.Separator); err != nil {
				return err
			}
		}
		first = false
		if _, err := bw.WriteString(strconv.FormatInt(i, 10)); err != nil {
			return err
		}
	}
	if !first {
		if err := bw.WriteByte('\n'); err != nil {
			return err
		}
	}
	return bw.Flush()
}

// Yes writes line and a newline repeatedly until writing fails or ctx ends.
func Yes(ctx context.Context, dst io.Writer, line string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	unit := line + "\n"
	chunk := strings.Repeat(unit, max(1, chunkSize/len(unit)))
	w := writer{ctx, dst}
	for {
		if _, err := io.WriteString(w, chunk); err != nil {
			return err
		}
	}
}
