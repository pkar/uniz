// Package digest implements checksums and base64 encoding.
package digest

import (
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"strings"

	"github.com/pkar/uniz/internal/text"
)

// Algorithm names a cryptographic hash.
type Algorithm string

// Supported algorithms.
const (
	MD5    Algorithm = "md5"
	SHA1   Algorithm = "sha1"
	SHA224 Algorithm = "sha224"
	SHA256 Algorithm = "sha256"
	SHA384 Algorithm = "sha384"
	SHA512 Algorithm = "sha512"
)

// New returns a new hash for alg.
func New(alg Algorithm) (hash.Hash, error) {
	switch alg {
	case MD5:
		return md5.New(), nil
	case SHA1:
		return sha1.New(), nil
	case SHA224:
		return sha256.New224(), nil
	case SHA256:
		return sha256.New(), nil
	case SHA384:
		return sha512.New384(), nil
	case SHA512:
		return sha512.New(), nil
	}
	return nil, fmt.Errorf("unknown hash algorithm %q", alg)
}

// Sum returns the lowercase hex digest of src.
func Sum(ctx context.Context, alg Algorithm, src io.Reader) (string, error) {
	h, err := New(alg)
	if err != nil {
		return "", err
	}
	if _, err := text.Cat(ctx, h, src); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ParseLine parses a "<hex>  <name>" or "<hex> *<name>" checksum line as
// written by sha256sum and friends.
func ParseLine(line string) (sum, name string, ok bool) {
	sum, rest, found := strings.Cut(strings.TrimSuffix(line, "\r"), " ")
	if !found || sum == "" || len(rest) < 2 || rest[0] != ' ' && rest[0] != '*' {
		return "", "", false
	}
	if _, err := hex.DecodeString(sum); err != nil {
		return "", "", false
	}
	return strings.ToLower(sum), rest[1:], true
}

var crcTable = func() (t [256]uint32) {
	for i := range t {
		c := uint32(i) << 24
		for range 8 {
			if c&0x80000000 != 0 {
				c = c<<1 ^ 0x04C11DB7
			} else {
				c <<= 1
			}
		}
		t[i] = c
	}
	return t
}()

type crc struct {
	v uint32
	n int64
}

func (c *crc) Write(p []byte) (int, error) {
	for _, b := range p {
		c.v = c.v<<8 ^ crcTable[byte(c.v>>24)^b]
	}
	c.n += int64(len(p))
	return len(p), nil
}

// CRC returns the POSIX cksum CRC and byte count of src.
func CRC(ctx context.Context, src io.Reader) (uint32, int64, error) {
	var c crc
	if _, err := text.Cat(ctx, &c, src); err != nil {
		return 0, c.n, err
	}
	v := c.v
	for n := c.n; n > 0; n >>= 8 {
		v = v<<8 ^ crcTable[byte(v>>24)^byte(n)]
	}
	return ^v, c.n, nil
}

type wrapWriter struct {
	w          io.Writer
	width, col int
}

func (w *wrapWriter) Write(p []byte) (int, error) {
	written := 0
	for len(p) > 0 {
		n := min(len(p), w.width-w.col)
		if _, err := w.w.Write(p[:n]); err != nil {
			return written, err
		}
		written, p, w.col = written+n, p[n:], w.col+n
		if w.col == w.width {
			if _, err := w.w.Write([]byte{'\n'}); err != nil {
				return written, err
			}
			w.col = 0
		}
	}
	return written, nil
}

// Encode writes standard base64, wrapping lines at wrap columns (0 disables
// wrapping). Wrapped output ends with a newline; unwrapped output does not.
func Encode(ctx context.Context, dst io.Writer, src io.Reader, wrap int) error {
	out := dst
	var ww *wrapWriter
	if wrap > 0 {
		ww = &wrapWriter{w: dst, width: wrap}
		out = ww
	}
	enc := base64.NewEncoder(base64.StdEncoding, out)
	if _, err := text.Cat(ctx, enc, src); err != nil {
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}
	if ww != nil && ww.col > 0 {
		_, err := dst.Write([]byte{'\n'})
		return err
	}
	return nil
}

// Base32Encode writes standard base32 like Encode writes base64.
func Base32Encode(ctx context.Context, dst io.Writer, src io.Reader, wrap int) error {
	out := dst
	var ww *wrapWriter
	if wrap > 0 {
		ww = &wrapWriter{w: dst, width: wrap}
		out = ww
	}
	enc := base32.NewEncoder(base32.StdEncoding, out)
	if _, err := text.Cat(ctx, enc, src); err != nil {
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}
	if ww != nil && ww.col > 0 {
		_, err := dst.Write([]byte{'\n'})
		return err
	}
	return nil
}

// Base32Decode writes the bytes encoded by standard base32 input; line
// breaks are ignored.
func Base32Decode(ctx context.Context, dst io.Writer, src io.Reader) error {
	_, err := text.Cat(ctx, dst, base32.NewDecoder(base32.StdEncoding, src))
	return err
}

// Decode writes the bytes encoded by standard base64 input. Line breaks are
// ignored; other characters are errors. Output before an error is kept.
func Decode(ctx context.Context, dst io.Writer, src io.Reader) error {
	_, err := text.Cat(ctx, dst, base64.NewDecoder(base64.StdEncoding, src))
	return err
}
