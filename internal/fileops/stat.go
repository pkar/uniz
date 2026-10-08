package fileops

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// StatInfo is file metadata. Fields after Target are filled only when
// HasSys is true (Unix); Blocks counts 512-byte units.
type StatInfo struct {
	Path   string
	Info   fs.FileInfo
	Target string // symlink target when Info is a symlink

	HasSys    bool
	Dev, Ino  uint64
	Nlink     uint64
	UID, GID  uint32
	Blocks    int64
	BlockSize int64
	Atime     time.Time
	Ctime     time.Time
	Birth     time.Time // zero when unknown
}

// Stat returns metadata for path; follow selects stat over lstat.
func Stat(path string, follow bool) (StatInfo, error) {
	var info fs.FileInfo
	var err error
	if follow {
		info, err = os.Stat(path)
	} else {
		info, err = os.Lstat(path)
	}
	if err != nil {
		return StatInfo{}, err
	}
	si := StatInfo{Path: path, Info: info}
	if info.Mode()&fs.ModeSymlink != 0 {
		si.Target, _ = os.Readlink(path)
	}
	fillSys(&si)
	return si, nil
}

// RawMode returns the Unix st_mode bits for m.
func RawMode(m fs.FileMode) uint32 {
	raw := uint32(m.Perm())
	switch {
	case m.IsDir():
		raw |= 0o040000
	case m&fs.ModeSymlink != 0:
		raw |= 0o120000
	case m&fs.ModeNamedPipe != 0:
		raw |= 0o010000
	case m&fs.ModeSocket != 0:
		raw |= 0o140000
	case m&fs.ModeCharDevice != 0:
		raw |= 0o020000
	case m&fs.ModeDevice != 0:
		raw |= 0o060000
	default:
		raw |= 0o100000
	}
	if m&fs.ModeSetuid != 0 {
		raw |= 0o4000
	}
	if m&fs.ModeSetgid != 0 {
		raw |= 0o2000
	}
	if m&fs.ModeSticky != 0 {
		raw |= 0o1000
	}
	return raw
}

// ModeString renders m like ls -l: "-rw-r--r--", "drwxr-xr-t", "lrwxrwxrwx".
func ModeString(m fs.FileMode) string {
	raw := RawMode(m)
	b := []byte("?rwxrwxrwx")
	b[0] = map[uint32]byte{0o040000: 'd', 0o120000: 'l', 0o010000: 'p', 0o140000: 's', 0o020000: 'c', 0o060000: 'b', 0o100000: '-'}[raw&0o170000]
	for i := range 9 {
		if raw&(1<<(8-i)) == 0 {
			b[i+1] = '-'
		}
	}
	special := func(bit uint32, pos int, set, unset byte) {
		if raw&bit != 0 {
			if b[pos] == '-' {
				b[pos] = unset
			} else {
				b[pos] = set
			}
		}
	}
	special(0o4000, 3, 's', 'S')
	special(0o2000, 6, 's', 'S')
	special(0o1000, 9, 't', 'T')
	return string(b)
}

// FileType names the type like GNU stat %F.
func FileType(info fs.FileInfo) string {
	switch raw := RawMode(info.Mode()) & 0o170000; raw {
	case 0o040000:
		return "directory"
	case 0o120000:
		return "symbolic link"
	case 0o010000:
		return "fifo"
	case 0o140000:
		return "socket"
	case 0o020000:
		return "character special file"
	case 0o060000:
		return "block special file"
	}
	if info.Size() == 0 {
		return "regular empty file"
	}
	return "regular file"
}

const statTime = "2006-01-02 15:04:05.000000000 -0700"

// DefaultStatFormat is the multi-line layout used when no format is given.
const DefaultStatFormat = `  File: %N
  Size: %-10s	Blocks: %-10b IO Block: %-6o %F
Device: %d	Inode: %-11i Links: %h
Access: (%04a/%A)  Uid: (%5u/%8U)   Gid: (%5g/%8G)
Access: %x
Modify: %y
Change: %z
 Birth: %w
`

// FormatStat expands GNU stat -c directives: %n name, %N quoted name with
// link target, %s size, %b blocks, %B block unit (512), %o I/O block size,
// %f raw mode (hex), %F type, %a octal permissions, %A permission string,
// %u/%U owner id/name, %g/%G group id/name, %h links, %i inode, %d device,
// %x/%y/%z/%w access/modify/change/birth times and %X/%Y/%Z/%W as epoch
// seconds, %% a percent sign. Flags - and 0 and a width may precede the
// letter. Unknown values print "?" (or "-" for an unknown birth time).
func FormatStat(si StatInfo, format string) string {
	var b strings.Builder
	for i := 0; i < len(format); i++ {
		if format[i] != '%' || i+1 == len(format) {
			b.WriteByte(format[i])
			continue
		}
		j := i + 1
		for j < len(format) && strings.IndexByte("-0#+ ", format[j]) >= 0 {
			j++
		}
		flags := format[i+1 : j]
		k := j
		for k < len(format) && format[k] >= '0' && format[k] <= '9' {
			k++
		}
		width, _ := strconv.Atoi(format[j:k])
		if k == len(format) {
			b.WriteString(format[i:])
			break
		}
		v, numeric, ok := statField(si, format[k])
		if !ok {
			b.WriteString(format[i : k+1])
			i = k
			continue
		}
		switch {
		case len(v) >= width:
		case strings.Contains(flags, "-"):
			v += strings.Repeat(" ", width-len(v))
		case strings.Contains(flags, "0") && numeric:
			v = strings.Repeat("0", width-len(v)) + v
		default:
			v = strings.Repeat(" ", width-len(v)) + v
		}
		b.WriteString(v)
		i = k
	}
	return b.String()
}

func statField(si StatInfo, c byte) (v string, numeric, ok bool) {
	sys := func(s string) string {
		if si.HasSys {
			return s
		}
		return "?"
	}
	u := func(n uint64) string { return sys(strconv.FormatUint(n, 10)) }
	stamp := func(t time.Time, epoch bool) string {
		switch {
		case t.IsZero() && c == 'w' || c == 'W' && t.IsZero():
			return "-"
		case t.IsZero():
			return "?"
		case epoch:
			return strconv.FormatInt(t.Unix(), 10)
		}
		return t.Format(statTime)
	}
	info := si.Info
	switch c {
	case '%':
		return "%", false, true
	case 'n':
		return si.Path, false, true
	case 'N':
		if si.Target != "" {
			return strconv.Quote(si.Path) + " -> " + strconv.Quote(si.Target), false, true
		}
		return strconv.Quote(si.Path), false, true
	case 's':
		return strconv.FormatInt(info.Size(), 10), true, true
	case 'b':
		return sys(strconv.FormatInt(si.Blocks, 10)), true, true
	case 'B':
		return "512", true, true
	case 'o':
		return sys(strconv.FormatInt(si.BlockSize, 10)), true, true
	case 'f':
		return strconv.FormatUint(uint64(RawMode(info.Mode())), 16), false, true
	case 'F':
		return FileType(info), false, true
	case 'a':
		return strconv.FormatUint(uint64(RawMode(info.Mode())&0o7777), 8), true, true
	case 'A':
		return ModeString(info.Mode()), false, true
	case 'u':
		return u(uint64(si.UID)), true, true
	case 'g':
		return u(uint64(si.GID)), true, true
	case 'U', 'G':
		if !si.HasSys {
			return "?", false, true
		}
		if c == 'U' {
			id := strconv.FormatUint(uint64(si.UID), 10)
			if usr, err := user.LookupId(id); err == nil {
				return usr.Username, false, true
			}
			return "UNKNOWN", false, true
		}
		if g, err := user.LookupGroupId(strconv.FormatUint(uint64(si.GID), 10)); err == nil {
			return g.Name, false, true
		}
		return "UNKNOWN", false, true
	case 'h':
		return u(si.Nlink), true, true
	case 'i':
		return u(si.Ino), true, true
	case 'd':
		return u(si.Dev), true, true
	case 'x', 'X':
		return stamp(si.Atime, c == 'X'), c == 'X', true
	case 'y', 'Y':
		return stamp(info.ModTime(), c == 'Y'), c == 'Y', true
	case 'z', 'Z':
		return stamp(si.Ctime, c == 'Z'), c == 'Z', true
	case 'w', 'W':
		return stamp(si.Birth, c == 'W'), c == 'W', true
	}
	return "", false, false
}

// DuOptions controls DiskUsage.
type DuOptions struct {
	All      bool // Report files too, not only directories.
	MaxDepth int  // Report entries at most this deep below an operand; negative means no limit.
	Apparent bool // Count byte sizes instead of allocated blocks.
}

// DuEntry is one DiskUsage report: Bytes is the total under Path.
type DuEntry struct {
	Path  string
	Bytes int64
}

// DiskUsage walks root without following symlinks and calls report for
// every directory (and, with All, every file) in post-order, children first.
// Hard-linked files are counted once per call. Unreadable entries are
// reported in the joined error while the walk continues; the total is
// returned either way.
func DiskUsage(ctx context.Context, root string, opts DuOptions, report func(DuEntry) error) (int64, error) {
	seen := map[[2]uint64]bool{}
	var errs []error
	size := func(path string, info fs.FileInfo) int64 {
		si := StatInfo{Path: path, Info: info}
		fillSys(&si)
		if si.HasSys && si.Nlink > 1 && !info.IsDir() {
			key := [2]uint64{si.Dev, si.Ino}
			if seen[key] {
				return 0
			}
			seen[key] = true
		}
		if si.HasSys && !opts.Apparent {
			return si.Blocks * 512
		}
		return info.Size()
	}
	var walk func(path string, info fs.FileInfo, depth int) (int64, error)
	walk = func(path string, info fs.FileInfo, depth int) (int64, error) {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		total := size(path, info)
		show := opts.MaxDepth < 0 || depth <= opts.MaxDepth
		if info.IsDir() {
			entries, err := os.ReadDir(path)
			if err != nil {
				errs = append(errs, err)
			}
			for _, e := range entries {
				child := filepath.Join(path, e.Name())
				ci, err := os.Lstat(child)
				if err != nil {
					errs = append(errs, err)
					continue
				}
				n, err := walk(child, ci, depth+1)
				if err != nil {
					return total, err
				}
				total += n
			}
		} else if !opts.All && depth > 0 {
			show = false
		}
		if show {
			if err := report(DuEntry{Path: path, Bytes: total}); err != nil {
				return total, err
			}
		}
		return total, nil
	}
	info, err := os.Lstat(root)
	if err != nil {
		return 0, err
	}
	total, err := walk(root, info, 0)
	if err != nil {
		return total, err
	}
	return total, errors.Join(errs...)
}

// refuseSpecial rejects ".", "..", and root directories as targets.
func refuseSpecial(op, path string) error {
	if base := filepath.Base(trimSlash(path)); base == "." || base == ".." {
		return pathErr(op, path, ErrRefused)
	}
	if abs, err := filepath.Abs(path); err == nil && filepath.Dir(abs) == abs {
		return pathErr(op, path, ErrRefused)
	}
	return nil
}

// InstallOptions controls Install.
type InstallOptions struct {
	Mode          fs.FileMode // Permissions of the installed file; 0 means 0755.
	MakeParents   bool        // Create missing parent directories (install -D).
	PreserveTimes bool        // Copy the source's modification time (install -p).
}

// Install copies the regular file src to the file path dst with the given
// mode. It writes a temporary file beside dst and renames it into place,
// so a symlink at dst is replaced rather than followed. dst may not be a
// directory, ".", "..", or a root, nor the same file as src.
func Install(ctx context.Context, src, dst string, opts InstallOptions) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := refuseSpecial("write", dst); err != nil {
		return err
	}
	mode := opts.Mode
	if mode == 0 {
		mode = 0o755
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	srcInfo, err := in.Stat()
	if err != nil {
		return err
	}
	if !srcInfo.Mode().IsRegular() {
		return pathErr("read", src, errors.New("not a regular file"))
	}
	if dstInfo, err := os.Lstat(dst); err == nil {
		if dstInfo.IsDir() {
			return pathErr("write", dst, errors.New("is a directory"))
		}
		if os.SameFile(srcInfo, dstInfo) {
			return fmt.Errorf("install: %s and %s are the same file", src, dst)
		}
	}
	dir := filepath.Dir(dst)
	if opts.MakeParents {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	tmp, err := os.CreateTemp(dir, ".uniz-install-*")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tmp.Close()
			os.Remove(tmp.Name())
		}
	}()
	if _, err = io.Copy(tmp, contextReader{ctx, in}); err != nil {
		return err
	}
	if err = tmp.Chmod(mode); err != nil {
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	if opts.PreserveTimes {
		if err = os.Chtimes(tmp.Name(), srcInfo.ModTime(), srcInfo.ModTime()); err != nil {
			return err
		}
	}
	return os.Rename(tmp.Name(), dst)
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (c contextReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}

// ShredOptions controls Shred.
type ShredOptions struct {
	Passes int       // Random overwrite passes; 0 means 3.
	Zero   bool      // Finish with a pass of zeros.
	Remove bool      // Truncate and remove the file afterwards.
	Random io.Reader // Source of overwrite data; nil means crypto/rand.
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

// Shred overwrites the regular file at path in place, syncing after each
// pass. It refuses symlinks, non-regular files, ".", "..", and roots, and
// checks that the file opened is the one inspected. Overwriting cannot
// reach old copies kept by journaling or copy-on-write filesystems or SSDs.
func Shred(ctx context.Context, path string, opts ShredOptions) error {
	if err := refuseSpecial("overwrite", path); err != nil {
		return err
	}
	before, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if before.Mode()&fs.ModeSymlink != 0 {
		return pathErr("overwrite", path, errors.New("refusing to follow a symbolic link"))
	}
	if !before.Mode().IsRegular() {
		return pathErr("overwrite", path, errors.New("not a regular file"))
	}
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	after, err := f.Stat()
	if err != nil {
		return err
	}
	if !os.SameFile(before, after) {
		return pathErr("overwrite", path, errors.New("file changed while opening"))
	}
	passes := opts.Passes
	if passes <= 0 {
		passes = 3
	}
	src := opts.Random
	if src == nil {
		src = cryptoRand{}
	}
	size := after.Size()
	var sources []io.Reader
	for range passes {
		sources = append(sources, src)
	}
	if opts.Zero {
		sources = append(sources, zeroReader{})
	}
	for _, r := range sources {
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return err
		}
		if _, err := io.CopyN(f, contextReader{ctx, r}, size); err != nil {
			return err
		}
		if err := f.Sync(); err != nil {
			return err
		}
	}
	if !opts.Remove {
		return f.Close()
	}
	if err := f.Truncate(0); err != nil {
		return err
	}
	if err := errors.Join(f.Sync(), f.Close()); err != nil {
		return err
	}
	return os.Remove(path)
}

// SyncFile flushes one file's data to storage.
func SyncFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	return errors.Join(f.Sync(), f.Close())
}
