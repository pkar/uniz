package fileops

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ModeChange computes a new mode from a file's current mode.
type ModeChange func(old fs.FileMode, isDir bool) fs.FileMode

const permMask = fs.ModePerm | fs.ModeSetuid | fs.ModeSetgid | fs.ModeSticky

// ParseMode parses an octal mode (755, 4755) or a comma-separated symbolic
// mode such as "u+x,go-w", "a=r", "g=u", or "+X". When no u/g/o/a is given,
// "a" is used and the process umask is not applied.
func ParseMode(spec string) (ModeChange, error) {
	if spec == "" {
		return nil, errors.New("empty mode")
	}
	if strings.Trim(spec, "01234567") == "" {
		v, err := strconv.ParseUint(spec, 8, 32)
		if err != nil || v > 0o7777 {
			return nil, fmt.Errorf("invalid mode %q", spec)
		}
		mode := fs.FileMode(v & 0o777)
		for bit, flag := range map[uint64]fs.FileMode{0o4000: fs.ModeSetuid, 0o2000: fs.ModeSetgid, 0o1000: fs.ModeSticky} {
			if v&bit != 0 {
				mode |= flag
			}
		}
		return func(old fs.FileMode, _ bool) fs.FileMode { return old&^permMask | mode }, nil
	}
	type action struct {
		op    byte
		perms string
	}
	type clause struct {
		who     string
		actions []action
	}
	var clauses []clause
	for _, part := range strings.Split(spec, ",") {
		bad := fmt.Errorf("invalid mode %q", spec)
		i := 0
		for i < len(part) && strings.IndexByte("ugoa", part[i]) >= 0 {
			i++
		}
		c := clause{who: part[:i]}
		if c.who == "" || strings.Contains(c.who, "a") {
			c.who = "ugo"
		}
		if i == len(part) {
			return nil, bad
		}
		for i < len(part) {
			op := part[i]
			if strings.IndexByte("+-=", op) < 0 {
				return nil, bad
			}
			i++
			j := i
			if j < len(part) && strings.IndexByte("ugo", part[j]) >= 0 {
				j++
			} else {
				for j < len(part) && strings.IndexByte("rwxXst", part[j]) >= 0 {
					j++
				}
			}
			c.actions = append(c.actions, action{op, part[i:j]})
			i = j
		}
		clauses = append(clauses, c)
	}
	shift := map[byte]uint{'u': 6, 'g': 3, 'o': 0}
	return func(old fs.FileMode, isDir bool) fs.FileMode {
		mode := old
		for _, c := range clauses {
			for _, a := range c.actions {
				var bits fs.FileMode
				for k := 0; k < len(a.perms); k++ {
					p := a.perms[k]
					if src, ok := shift[p]; ok { // copy another class's bits
						v := (mode.Perm() >> src) & 7
						for w := range len(c.who) {
							bits |= v << shift[c.who[w]]
						}
						continue
					}
					for w := range len(c.who) {
						who := c.who[w]
						s := shift[who]
						switch p {
						case 'r':
							bits |= 4 << s
						case 'w':
							bits |= 2 << s
						case 'x':
							bits |= 1 << s
						case 'X':
							if isDir || mode&0o111 != 0 {
								bits |= 1 << s
							}
						case 's':
							if who == 'u' {
								bits |= fs.ModeSetuid
							} else if who == 'g' {
								bits |= fs.ModeSetgid
							}
						case 't':
							if who == 'o' {
								bits |= fs.ModeSticky
							}
						}
					}
				}
				switch a.op {
				case '+':
					mode |= bits
				case '-':
					mode &^= bits
				case '=':
					var clear fs.FileMode
					for w := range len(c.who) {
						clear |= 7 << shift[c.who[w]]
						switch c.who[w] {
						case 'u':
							clear |= fs.ModeSetuid
						case 'g':
							clear |= fs.ModeSetgid
						case 'o':
							clear |= fs.ModeSticky
						}
					}
					mode = mode&^clear | bits
				}
			}
		}
		return mode
	}, nil
}

// ChmodOptions controls Chmod.
type ChmodOptions struct {
	Recursive bool // Also change everything below directories; symlinks inside are skipped.
}

// Chmod applies change to path, following a symlink operand like chmod(1).
// With Recursive, entries below directories are changed without following
// symlinks. Errors are collected and the walk continues.
func Chmod(ctx context.Context, path string, change ModeChange, opts ChmodOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	apply := func(p string, info fs.FileInfo) error {
		return os.Chmod(p, change(info.Mode(), info.IsDir())&permMask)
	}
	if err := apply(path, info); err != nil || !opts.Recursive || !info.IsDir() {
		return err
	}
	var errs []error
	walkErr := filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if cerr := ctx.Err(); cerr != nil {
			return cerr
		}
		if err != nil {
			errs = append(errs, err)
			return nil
		}
		if p == path || d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		info, err := d.Info()
		if err == nil {
			err = apply(p, info)
		}
		if err != nil {
			errs = append(errs, err)
		}
		return nil
	})
	return errors.Join(append(errs, walkErr)...)
}

const tempChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// MakeTemp creates a new file (mode 0600) or directory (mode 0700) named by
// replacing the trailing run of at least three X characters in template with
// random characters, inside dir ("" keeps template's own directory). It
// never reuses an existing name and returns the created path.
func MakeTemp(dir, template string, directory bool) (string, error) {
	trimmed := strings.TrimRight(template, "X")
	if len(template)-len(trimmed) < 3 {
		return "", fmt.Errorf("template %q must end with at least 3 X characters", template)
	}
	n := len(template) - len(trimmed)
	for range 100 {
		b := []byte(trimmed)
		for range n {
			b = append(b, tempChars[rand.IntN(len(tempChars))])
		}
		name := string(b)
		if dir != "" {
			name = filepath.Join(dir, name)
		}
		var err error
		if directory {
			err = os.Mkdir(name, 0o700)
		} else {
			var f *os.File
			if f, err = os.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600); err == nil {
				err = f.Close()
			}
		}
		if !errors.Is(err, fs.ErrExist) {
			return name, err
		}
	}
	return "", fmt.Errorf("could not create a unique name from %q", template)
}

// SizeChange describes a truncate size: Op is 0 (set), '+', '-', '<' (at
// most), '>' (at least), '/' (round down to a multiple), or '%' (round up).
type SizeChange struct {
	Op   byte
	Size int64
}

// ParseSize parses truncate(1) sizes such as "100", "+1K", "<2MB", "%4KiB".
// K M G T P E are powers of 1024 (also KiB...); KB MB GB... are powers of 1000.
func ParseSize(s string) (SizeChange, error) {
	var c SizeChange
	bad := fmt.Errorf("invalid size %q", s)
	if s != "" && strings.IndexByte("+-<>/%", s[0]) >= 0 {
		c.Op, s = s[0], s[1:]
	}
	num := strings.TrimRightFunc(s, func(r rune) bool { return r < '0' || r > '9' })
	suffix := s[len(num):]
	n, err := strconv.ParseInt(num, 10, 64)
	if err != nil || n < 0 {
		return c, bad
	}
	mult := int64(1)
	if suffix != "" {
		power := strings.IndexByte("KMGTPE", suffix[0]) + 1
		base := int64(1024)
		switch suffix[1:] {
		case "", "iB":
		case "B":
			base = 1000
		default:
			power = 0
		}
		if power == 0 {
			return c, bad
		}
		for range power {
			if mult > math.MaxInt64/base {
				return c, bad
			}
			mult *= base
		}
	}
	if n > math.MaxInt64/mult {
		return c, bad
	}
	c.Size = n * mult
	if (c.Op == '/' || c.Op == '%') && c.Size == 0 {
		return c, errors.New("division by zero")
	}
	return c, nil
}

// Apply returns the new size for a file of size old.
func (c SizeChange) Apply(old int64) int64 {
	switch c.Op {
	case '+':
		if old > math.MaxInt64-c.Size {
			return math.MaxInt64
		}
		return old + c.Size
	case '-':
		return max(0, old-c.Size)
	case '<':
		return min(old, c.Size)
	case '>':
		return max(old, c.Size)
	case '/':
		return old / c.Size * c.Size
	case '%':
		return (old + c.Size - 1) / c.Size * c.Size
	}
	return c.Size
}

// TruncateOptions controls Truncate.
type TruncateOptions struct {
	NoCreate bool // Skip missing files instead of creating them.
}

// Truncate sets path's size, extending with zero bytes or cutting data.
func Truncate(ctx context.Context, path string, change SizeChange, opts TruncateOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	flag := os.O_WRONLY
	if !opts.NoCreate {
		flag |= os.O_CREATE
	}
	f, err := os.OpenFile(path, flag, 0o666)
	if opts.NoCreate && errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err == nil {
		err = f.Truncate(change.Apply(info.Size()))
	}
	return errors.Join(err, f.Close())
}

// Unlink removes a single non-directory without following symlinks.
func Unlink(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return pathErr("unlink", path, ErrIsDirectory)
	}
	return os.Remove(path)
}
