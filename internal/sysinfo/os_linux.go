package sysinfo

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

const ioctlGetTermios = syscall.TCGETS

// The raw getpriority system call returns 20 - nice.
func kernelPriority(p int) int { return 20 - p }

type mountEntry struct{ source, target, fstype string }

func unescapeMount(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+3 < len(s) {
			if n, err := strconv.ParseUint(s[i+1:i+4], 8, 8); err == nil {
				b.WriteByte(byte(n))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func mountTable() ([]mountEntry, error) {
	f, err := os.Open("/proc/self/mounts")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []mountEntry
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if fields := strings.Fields(sc.Text()); len(fields) >= 3 {
			out = append(out, mountEntry{unescapeMount(fields[0]), unescapeMount(fields[1]), fields[2]})
		}
	}
	return out, sc.Err()
}

func usage(m mountEntry) (FSUsage, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(m.target, &st); err != nil {
		return FSUsage{}, err
	}
	bs := uint64(st.Frsize)
	if bs == 0 {
		bs = uint64(st.Bsize)
	}
	return FSUsage{Filesystem: m.source, MountPoint: m.target, Type: m.fstype,
		Total: uint64(st.Blocks) * bs, Free: uint64(st.Bfree) * bs, Avail: uint64(st.Bavail) * bs}, nil
}

// DiskFree reports the filesystem containing path.
func DiskFree(path string) (FSUsage, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return FSUsage{}, err
	}
	if _, err := os.Stat(abs); err != nil {
		return FSUsage{}, err
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		abs = real
	}
	mounts, err := mountTable()
	if err != nil {
		return FSUsage{}, err
	}
	best := mountEntry{source: "-", target: "/"}
	for _, m := range mounts {
		if (abs == m.target || strings.HasPrefix(abs, strings.TrimSuffix(m.target, "/")+"/")) && len(m.target) >= len(best.target) {
			best = m
		}
	}
	u, err := usage(mountEntry{best.source, abs, best.fstype})
	u.MountPoint = best.target
	return u, err
}

// Mounts reports every mounted filesystem that has a non-zero size.
func Mounts() ([]FSUsage, error) {
	mounts, err := mountTable()
	if err != nil {
		return nil, err
	}
	var out []FSUsage
	for _, m := range mounts {
		if u, err := usage(m); err == nil && u.Total > 0 {
			out = append(out, u)
		}
	}
	return out, nil
}
