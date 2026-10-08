package uniz

import (
	"context"
	"io"
	"iter"

	"github.com/pkar/uniz/internal/text"
)

// Streaming operations below require non-nil streams, leave them open, and
// check cancellation between reads and writes; they cannot interrupt a
// blocked read or write.

// ErrNegativeCount reports a negative Head or Tail count.
var ErrNegativeCount = text.ErrNegativeCount

// ErrZeroStep reports a zero Seq step.
var ErrZeroStep = text.ErrZeroStep

// SliceOptions selects lines (default) or bytes for Head and Tail. FromStart
// applies to Tail only and starts output at 1-based position Count.
type SliceOptions = text.SliceOptions

// Head copies the first opts.Count lines or bytes from src to dst.
func Head(ctx context.Context, dst io.Writer, src io.Reader, opts SliceOptions) error {
	return text.Head(ctx, dst, src, opts)
}

// Tail copies the last opts.Count lines or bytes, or everything from position
// Count with FromStart. It reads src to EOF; memory is bounded by the retained
// output. Nothing is written when reading fails.
func Tail(ctx context.Context, dst io.Writer, src io.Reader, opts SliceOptions) error {
	return text.Tail(ctx, dst, src, opts)
}

// SortOptions controls Sort and SortLines. Numeric takes precedence over
// IgnoreCase and compares leading numbers as float64.
type SortOptions = text.SortOptions

// SortLines returns a sorted copy of lines without modifying the input. Equal
// keys are ordered by bytes; with Unique, only the first input line of each
// key is kept.
func SortLines(lines []string, opts SortOptions) []string { return text.SortLines(lines, opts) }

// Sort reads all lines from src into memory and writes them sorted, each with
// a trailing newline. Nothing is written when reading fails.
func Sort(ctx context.Context, dst io.Writer, src io.Reader, opts SortOptions) error {
	return text.Sort(ctx, dst, src, opts)
}

// ReadLines reads lines without trailing newlines; a final unterminated line
// is included. Lines read before an error are returned with it.
func ReadLines(ctx context.Context, src io.Reader) ([]string, error) {
	return text.ReadLines(ctx, src)
}

// UniqOptions controls Uniq.
type UniqOptions = text.UniqOptions

// Uniq collapses adjacent equal lines from src. Output lines always end with
// a newline; with Count they are formatted like uniq -c ("%7d line").
func Uniq(ctx context.Context, dst io.Writer, src io.Reader, opts UniqOptions) error {
	return text.Uniq(ctx, dst, src, opts)
}

// SeqOptions describes an integer sequence. Separator is written verbatim
// between numbers; use "\n" for one number per line.
type SeqOptions = text.SeqOptions

// Seq writes the sequence in decimal followed by a newline. A zero Step
// returns ErrZeroStep.
func Seq(ctx context.Context, dst io.Writer, opts SeqOptions) error {
	return text.Seq(ctx, dst, opts)
}

// Sequence yields first, first+step, ... up to and including last. A zero
// step yields nothing; iteration stops before int64 overflow.
func Sequence(first, step, last int64) iter.Seq[int64] { return text.Sequence(first, step, last) }

// Yes writes line plus newline until writing fails or ctx is canceled. With a
// writer that never fails, it runs until ctx is canceled.
func Yes(ctx context.Context, dst io.Writer, line string) error { return text.Yes(ctx, dst, line) }
