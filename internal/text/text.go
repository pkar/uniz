// Package text implements streaming text operations.
package text

import (
	"bufio"
	"context"
	"io"
	"unicode"
)

// Counts contains newline, word, byte, and Unicode character counts.
type Counts struct{ Lines, Words, Bytes, Chars int64 }

type reader struct {
	ctx context.Context
	r   io.Reader
}

func (r reader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}

type writer struct {
	ctx context.Context
	w   io.Writer
}

func (w writer) Write(p []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	return w.w.Write(p)
}

// Cat copies input unchanged. Cancellation is checked between reads and writes.
func Cat(ctx context.Context, dst io.Writer, src io.Reader) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return io.Copy(writer{ctx, dst}, reader{ctx, src})
}

// Count reads to EOF. Words are separated by Unicode whitespace. Lines counts
// newline bytes (not an unterminated final line). Invalid UTF-8 bytes each count
// as one character. Partial counts are returned alongside read errors.
func Count(ctx context.Context, src io.Reader) (Counts, error) {
	var counts Counts
	if err := ctx.Err(); err != nil {
		return counts, err
	}
	r := bufio.NewReader(reader{ctx, src})
	inWord := false
	for {
		if err := ctx.Err(); err != nil {
			return counts, err
		}
		ch, n, err := r.ReadRune()
		if err == io.EOF {
			return counts, nil
		}
		if err != nil {
			return counts, err
		}
		counts.Bytes += int64(n)
		counts.Chars++
		if ch == '\n' {
			counts.Lines++
		}
		if unicode.IsSpace(ch) {
			inWord = false
		} else if !inWord {
			counts.Words++
			inWord = true
		}
	}
}
