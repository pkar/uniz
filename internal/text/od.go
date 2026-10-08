package text

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

// OdType is one od output format: Kind is a (named character), c
// (character), d (signed decimal), o (octal), u (unsigned decimal), x (hex),
// or f (floating point); Size is the bytes per value.
type OdType struct {
	Kind byte
	Size int
}

// ParseOdTypes parses an od -t specification such as "x1", "d4", "c", or
// "o2x1". Integer sizes are 1, 2, 4, 8 or C S I L; float sizes 4, 8 or F D.
func ParseOdTypes(spec string) ([]OdType, error) {
	var types []OdType
	for i := 0; i < len(spec); {
		t := OdType{Kind: spec[i]}
		i++
		switch t.Kind {
		case 'a', 'c':
			t.Size = 1
		case 'd', 'o', 'u', 'x', 'f':
			t.Size = 4
			if t.Kind == 'f' {
				t.Size = 8
			}
			if i < len(spec) {
				named := map[byte]int{'C': 1, 'S': 2, 'I': 4, 'L': 8}
				if t.Kind == 'f' {
					named = map[byte]int{'F': 4, 'D': 8}
				}
				if n, ok := named[spec[i]]; ok {
					t.Size = n
					i++
				} else if j := i + len(spec[i:]) - len(strings.TrimLeft(spec[i:], "0123456789")); j > i {
					t.Size, _ = strconv.Atoi(spec[i:j])
					i = j
				}
			}
			valid := t.Size == 1 || t.Size == 2 || t.Size == 4 || t.Size == 8
			if t.Kind == 'f' {
				valid = t.Size == 4 || t.Size == 8
			}
			if !valid {
				return nil, fmt.Errorf("invalid type size in %q", spec)
			}
		default:
			return nil, fmt.Errorf("invalid type %q in %q", t.Kind, spec)
		}
		types = append(types, t)
	}
	if len(types) == 0 {
		return nil, errors.New("empty type specification")
	}
	return types, nil
}

// OdOptions controls Od.
type OdOptions struct {
	Types []OdType // Default o2.
	Radix byte     // Address radix: 'o' (default), 'd', 'x', or 'n' for none.
	Skip  int64    // Bytes to skip first.
	Limit int64    // Bytes to dump; negative means all.
	All   bool     // Do not replace repeated lines with "*".
	Width int      // Bytes per line; default 16.
}

var odNames = [...]string{"nul", "soh", "stx", "etx", "eot", "enq", "ack", "bel", "bs", "ht", "nl", "vt", "ff", "cr", "so", "si",
	"dle", "dc1", "dc2", "dc3", "dc4", "nak", "syn", "etb", "can", "em", "sub", "esc", "fs", "gs", "rs", "us", "sp"}

func (t OdType) width() int {
	bits := t.Size * 8
	switch t.Kind {
	case 'a', 'c':
		return 3
	case 'o':
		return (bits + 2) / 3
	case 'x':
		return t.Size * 2
	case 'u':
		return len(strconv.FormatUint(math.MaxUint64>>(64-bits), 10))
	case 'd':
		return len(strconv.FormatInt(math.MinInt64>>(64-bits), 10))
	}
	if t.Size == 4 {
		return 14
	}
	return 23
}

func (t OdType) format(b []byte) string {
	if t.Kind == 'a' || t.Kind == 'c' {
		c := b[0]
		if t.Kind == 'a' {
			c &= 0x7f
			switch {
			case c <= ' ':
				return odNames[c]
			case c == 0x7f:
				return "del"
			}
			return string(rune(c))
		}
		if s, ok := map[byte]string{0: `\0`, '\a': `\a`, '\b': `\b`, '\f': `\f`, '\n': `\n`, '\r': `\r`, '\t': `\t`, '\v': `\v`}[c]; ok {
			return s
		}
		if c >= ' ' && c < 0x7f {
			return string(rune(c))
		}
		return fmt.Sprintf("%03o", c)
	}
	var u uint64
	switch t.Size {
	case 1:
		u = uint64(b[0])
	case 2:
		u = uint64(binary.NativeEndian.Uint16(b))
	case 4:
		u = uint64(binary.NativeEndian.Uint32(b))
	default:
		u = binary.NativeEndian.Uint64(b)
	}
	w := t.width()
	switch t.Kind {
	case 'o':
		return fmt.Sprintf("%0*o", w, u)
	case 'x':
		return fmt.Sprintf("%0*x", w, u)
	case 'u':
		return strconv.FormatUint(u, 10)
	case 'd':
		shift := 64 - t.Size*8
		return strconv.FormatInt(int64(u<<shift)>>shift, 10)
	}
	if t.Size == 4 {
		return strconv.FormatFloat(float64(math.Float32frombits(uint32(u))), 'g', -1, 32)
	}
	return strconv.FormatFloat(math.Float64frombits(u), 'g', -1, 64)
}

// Od writes a dump of src like POSIX od. Integers use native byte order;
// a short final value is padded with zero bytes.
func Od(ctx context.Context, dst io.Writer, src io.Reader, opts OdOptions) error {
	types := opts.Types
	if len(types) == 0 {
		types = []OdType{{'o', 2}}
	}
	width := opts.Width
	if width <= 0 {
		width = 16
	}
	for _, t := range types {
		if width%t.Size != 0 {
			return fmt.Errorf("line width %d is not a multiple of %d", width, t.Size)
		}
	}
	radix := opts.Radix
	if radix == 0 {
		radix = 'o'
	}
	addr := func(n int64) string {
		switch radix {
		case 'd':
			return fmt.Sprintf("%07d", n)
		case 'x':
			return fmt.Sprintf("%06x", n)
		case 'n':
			return ""
		}
		return fmt.Sprintf("%07o", n)
	}
	// Align columns: every type gets the same characters per byte.
	perByte := 0
	for _, t := range types {
		perByte = max(perByte, (t.width()+1+t.Size-1)/t.Size)
	}
	field := func(t OdType) int {
		if len(types) == 1 {
			return t.width() + 1
		}
		return perByte * t.Size
	}
	r := bufio.NewReader(reader{ctx, src})
	if opts.Skip > 0 {
		if n, err := io.CopyN(io.Discard, r, opts.Skip); err != nil {
			if err == io.EOF {
				return fmt.Errorf("cannot skip past end of input (skipped %d bytes)", n)
			}
			return err
		}
	}
	var in io.Reader = r
	if opts.Limit >= 0 {
		in = io.LimitReader(r, opts.Limit)
	}
	bw := bufio.NewWriter(writer{ctx, dst})
	offset := opts.Skip
	buf, prev := make([]byte, width), []byte(nil)
	starred := false
	for {
		n, err := io.ReadFull(in, buf)
		if n > 0 {
			line := buf[:n]
			if !opts.All && n == width && prev != nil && bytes.Equal(line, prev) {
				if !starred {
					bw.WriteString("*\n")
					starred = true
				}
			} else {
				starred = false
				prev = append(prev[:0], line...)
				for ti, t := range types {
					a := addr(offset)
					if ti > 0 {
						a = strings.Repeat(" ", len(a))
					}
					bw.WriteString(a)
					padded := append(append([]byte(nil), line...), make([]byte, (t.Size-n%t.Size)%t.Size)...)
					for i := 0; i < len(padded); i += t.Size {
						fmt.Fprintf(bw, "%*s", field(t), t.format(padded[i:i+t.Size]))
					}
					bw.WriteString("\n")
				}
			}
			offset += int64(n)
		}
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if err != nil {
			return errors.Join(err, bw.Flush())
		}
	}
	if radix != 'n' {
		bw.WriteString(addr(offset) + "\n")
	}
	return bw.Flush()
}
