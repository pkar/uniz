package uniz

import (
	"context"
	"io"
	"time"

	"github.com/pkar/uniz/internal/digest"
	"github.com/pkar/uniz/internal/format"
	"github.com/pkar/uniz/internal/testexpr"
	"github.com/pkar/uniz/internal/text"
)

// Range is an inclusive 1-based position range for Cut. End 0 means to the
// end of the line.
type Range = text.Range

// ParseRanges parses a cut list such as "1,3-5,7-" or "-2".
func ParseRanges(list string) ([]Range, error) { return text.ParseRanges(list) }

// CutUnit selects whether Cut positions count bytes, characters, or fields.
type CutUnit = text.CutUnit

// Cut units.
const (
	CutBytes  = text.CutBytes
	CutChars  = text.CutChars
	CutFields = text.CutFields
)

// CutOptions controls Cut. Delimiter is one character; empty means tab.
type CutOptions = text.CutOptions

// Cut writes the selected parts of each line in input order. A missing final
// newline is preserved.
func Cut(ctx context.Context, dst io.Writer, src io.Reader, opts CutOptions) error {
	return text.Cut(ctx, dst, src, opts)
}

// TranslateOptions controls Translate.
type TranslateOptions = text.TranslateOptions

// ExpandSet expands a tr set (escapes, ranges such as a-z, ASCII classes such
// as [:upper:]) into its characters in order.
func ExpandSet(set string) ([]rune, error) { return text.ExpandSet(set) }

// Translate translates, deletes, or squeezes Unicode characters like tr.
// Without Delete, a non-empty set2 translates set1 positionally. Squeeze
// applies to set2 when given, otherwise set1.
func Translate(ctx context.Context, dst io.Writer, src io.Reader, set1, set2 string, opts TranslateOptions) error {
	return text.Translate(ctx, dst, src, set1, set2, opts)
}

// PasteOptions controls Paste. Delimiters are used cyclically; nil means tab.
type PasteOptions = text.PasteOptions

// Paste merges corresponding lines of srcs. Repeating the same reader reads
// successive lines from it.
func Paste(ctx context.Context, dst io.Writer, srcs []io.Reader, opts PasteOptions) error {
	return text.Paste(ctx, dst, srcs, opts)
}

// NumberStyle selects which lines NumberLines numbers.
type NumberStyle = text.NumberStyle

// Numbering styles.
const (
	NumberNonEmpty = text.NumberNonEmpty
	NumberAll      = text.NumberAll
	NumberNone     = text.NumberNone
)

// NumberOptions controls NumberLines; start from DefaultNumberOptions.
type NumberOptions = text.NumberOptions

// DefaultNumberOptions returns nl's defaults.
func DefaultNumberOptions() NumberOptions { return text.DefaultNumberOptions() }

// NumberLines writes src with line numbers like nl and returns the next line
// number so numbering can continue across inputs.
func NumberLines(ctx context.Context, dst io.Writer, src io.Reader, opts NumberOptions) (int64, error) {
	return text.NumberLines(ctx, dst, src, opts)
}

// ReverseLines writes lines last to first like tac. Input is held in memory;
// every output line ends with a newline.
func ReverseLines(ctx context.Context, dst io.Writer, src io.Reader) error {
	return text.ReverseLines(ctx, dst, src)
}

// ReverseCharacters reverses the characters of each line like rev.
func ReverseCharacters(ctx context.Context, dst io.Writer, src io.Reader) error {
	return text.ReverseCharacters(ctx, dst, src)
}

// FoldOptions controls Fold. Width 0 means 80.
type FoldOptions = text.FoldOptions

// Fold wraps lines longer than Width like fold; every character counts as one
// column.
func Fold(ctx context.Context, dst io.Writer, src io.Reader, opts FoldOptions) error {
	return text.Fold(ctx, dst, src, opts)
}

// HashAlgorithm names a checksum algorithm for Checksum.
type HashAlgorithm = digest.Algorithm

// Hash algorithms.
const (
	MD5    = digest.MD5
	SHA1   = digest.SHA1
	SHA224 = digest.SHA224
	SHA256 = digest.SHA256
	SHA384 = digest.SHA384
	SHA512 = digest.SHA512
)

// Checksum returns the lowercase hex digest of src.
func Checksum(ctx context.Context, alg HashAlgorithm, src io.Reader) (string, error) {
	return digest.Sum(ctx, alg, src)
}

// ParseChecksumLine parses a "<hex>  <name>" line as written by sha256sum.
func ParseChecksumLine(line string) (sum, name string, ok bool) { return digest.ParseLine(line) }

// CRC returns the POSIX cksum checksum and byte count of src.
func CRC(ctx context.Context, src io.Reader) (uint32, int64, error) { return digest.CRC(ctx, src) }

// Base64Encode writes standard base64, wrapping at wrap columns (0 disables).
// Wrapped output ends with a newline.
func Base64Encode(ctx context.Context, dst io.Writer, src io.Reader, wrap int) error {
	return digest.Encode(ctx, dst, src, wrap)
}

// Base64Decode decodes standard base64, ignoring line breaks.
func Base64Decode(ctx context.Context, dst io.Writer, src io.Reader) error {
	return digest.Decode(ctx, dst, src)
}

// Printf formats args like printf(1), reusing the format while arguments
// remain. Invalid numbers print as 0 and are reported in the error, which
// accompanies the complete output.
func Printf(spec string, args []string) (string, error) { return format.Printf(spec, args) }

// Escapes expands echo -e escapes; stop reports that \c ended output.
func Escapes(s string) (out string, stop bool) { return format.Escapes(s) }

// DefaultDateFormat is date's default Strftime layout.
const DefaultDateFormat = format.DefaultDateFormat

// Strftime formats t with strftime conversions such as %Y-%m-%d %H:%M:%S.
func Strftime(t time.Time, layout string) string { return format.Strftime(t, layout) }

// ParseDate parses "now", "@<unix seconds>", RFC 3339, "YYYY-MM-DD[ HH:MM[:SS]]",
// RFC 1123, or date's default output. Times without a zone use loc.
func ParseDate(s string, now time.Time, loc *time.Location) (time.Time, error) {
	return format.ParseDate(s, now, loc)
}

// Test evaluates a test(1) expression such as []string{"-f", "go.mod"}.
// Errors indicate bad syntax or invalid integers.
func Test(args []string) (bool, error) { return testexpr.Evaluate(args) }
