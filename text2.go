package uniz

import (
	"context"
	"io"

	"github.com/pkar/uniz/internal/digest"
	"github.com/pkar/uniz/internal/split"
	"github.com/pkar/uniz/internal/text"
)

// TabStops holds one repeating interval or several ascending stop columns.
type TabStops = text.TabStops

// ParseTabStops parses "N" or a list such as "4,8,12".
func ParseTabStops(s string) (TabStops, error) { return text.ParseTabStops(s) }

// ExpandOptions controls Expand.
type ExpandOptions = text.ExpandOptions

// Expand converts tabs to spaces.
func Expand(ctx context.Context, dst io.Writer, src io.Reader, opts ExpandOptions) error {
	return text.Expand(ctx, dst, src, opts)
}

// UnexpandOptions controls Unexpand.
type UnexpandOptions = text.UnexpandOptions

// Unexpand converts runs of blanks that reach a tab stop into tabs.
func Unexpand(ctx context.Context, dst io.Writer, src io.Reader, opts UnexpandOptions) error {
	return text.Unexpand(ctx, dst, src, opts)
}

// FmtOptions controls Fmt.
type FmtOptions = text.FmtOptions

// Fmt fills paragraphs to a maximum width.
func Fmt(ctx context.Context, dst io.Writer, src io.Reader, opts FmtOptions) error {
	return text.Fmt(ctx, dst, src, opts)
}

// CommOptions controls Comm.
type CommOptions = text.CommOptions

// Comm writes the three-column comparison of two sorted inputs.
func Comm(ctx context.Context, dst io.Writer, a, b io.Reader, opts CommOptions) error {
	return text.Comm(ctx, dst, a, b, opts)
}

// JoinField names an output field for Join; File 0 is the join field.
type JoinField = text.JoinField

// JoinOptions controls Join.
type JoinOptions = text.JoinOptions

// ParseJoinFields parses a join -o list such as "0,1.2,2.1".
func ParseJoinFields(list string) ([]JoinField, error) { return text.ParseJoinFields(list) }

// Join writes the relational join of two inputs sorted on their join fields.
func Join(ctx context.Context, dst io.Writer, a, b io.Reader, opts JoinOptions) error {
	return text.Join(ctx, dst, a, b, opts)
}

// ShuffleOptions controls Shuffle; set Rand for reproducible output.
type ShuffleOptions = text.ShuffleOptions

// Shuffle writes items in random order, one per line.
func Shuffle(ctx context.Context, dst io.Writer, items []string, opts ShuffleOptions) error {
	return text.Shuffle(ctx, dst, items, opts)
}

// ErrLoop is wrapped by Tsort when its input has a cycle.
var ErrLoop = text.ErrLoop

// Tsort writes a topological order of the pairs read from src.
func Tsort(ctx context.Context, dst io.Writer, src io.Reader) error { return text.Tsort(ctx, dst, src) }

// OdType is one od output format, such as {Kind: 'x', Size: 1}.
type OdType = text.OdType

// OdOptions controls Od.
type OdOptions = text.OdOptions

// ParseOdTypes parses an od -t specification such as "x1" or "d4c".
func ParseOdTypes(spec string) ([]OdType, error) { return text.ParseOdTypes(spec) }

// Od writes an od-style dump of src.
func Od(ctx context.Context, dst io.Writer, src io.Reader, opts OdOptions) error {
	return text.Od(ctx, dst, src, opts)
}

// Base32Encode writes standard base32, wrapping lines at wrap columns (0 disables).
func Base32Encode(ctx context.Context, dst io.Writer, src io.Reader, wrap int) error {
	return digest.Base32Encode(ctx, dst, src, wrap)
}

// Base32Decode decodes standard base32, ignoring line breaks.
func Base32Decode(ctx context.Context, dst io.Writer, src io.Reader) error {
	return digest.Base32Decode(ctx, dst, src)
}

// SplitOptions controls Split.
type SplitOptions = split.Options

// Split writes src to numbered files and returns their names. Symlinks at
// output names are refused.
func Split(ctx context.Context, src io.Reader, opts SplitOptions) ([]string, error) {
	return split.Split(ctx, src, opts)
}

// CsplitOptions controls Csplit.
type CsplitOptions = split.CsplitOptions

// Csplit splits src at line-number and regexp patterns and returns the
// files written and their sizes.
func Csplit(ctx context.Context, src io.Reader, patterns []string, opts CsplitOptions) ([]string, []int64, error) {
	return split.Csplit(ctx, src, patterns, opts)
}
