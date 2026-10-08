package uniz

import (
	"context"
	"io"

	"github.com/pkar/uniz/internal/pwd"
	"github.com/pkar/uniz/internal/text"
)

// Cat copies src unchanged to dst and returns the number of bytes written.
// Streams must be non-nil. Cancellation is cooperative between reads and writes;
// an already-blocked read or write cannot be interrupted.
func Cat(ctx context.Context, dst io.Writer, src io.Reader) (int64, error) {
	return text.Cat(ctx, dst, src)
}

// Counts contains Lines (newline count), Words, Bytes, and Chars (Unicode code
// points). Invalid UTF-8 bytes each count as one character.
type Counts = text.Counts

// Count reads src to EOF, using Unicode whitespace to separate words. It returns
// partial counts on failure. The reader must be non-nil. Cancellation cannot
// interrupt a blocked read.
func Count(ctx context.Context, src io.Reader) (Counts, error) { return text.Count(ctx, src) }

// DirectoryOptions controls whether WorkingDirectory resolves symlinks.
type DirectoryOptions = pwd.Options

// WorkingDirectory returns the process working directory without changing it.
// By default it preserves a valid logical PWD; Physical resolves symbolic links.
func WorkingDirectory(ctx context.Context, opts DirectoryOptions) (string, error) {
	return pwd.Directory(ctx, opts)
}
