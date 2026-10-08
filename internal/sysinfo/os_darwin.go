package sysinfo

import (
	"syscall"
)

const ioctlGetTermios = syscall.TIOCGETA

func kernelPriority(p int) int { return p }

func cstring(b []int8) string {
	out := make([]byte, 0, len(b))
	for _, c := range b {
		if c == 0 {
			break
		}
		out = append(out, byte(c))
	}
	return string(out)
}

func fromStatfs(st *syscall.Statfs_t) FSUsage {
	bs := uint64(st.Bsize)
	return FSUsage{
		Filesystem: cstring(st.Mntfromname[:]),
		MountPoint: cstring(st.Mntonname[:]),
		Type:       cstring(st.Fstypename[:]),
		Total:      st.Blocks * bs, Free: st.Bfree * bs, Avail: st.Bavail * bs,
	}
}

// DiskFree reports the filesystem containing path.
func DiskFree(path string) (FSUsage, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return FSUsage{}, err
	}
	return fromStatfs(&st), nil
}

// Mounts reports every mounted filesystem that has a non-zero size.
func Mounts() ([]FSUsage, error) {
	const mntNoWait = 2
	n, err := syscall.Getfsstat(nil, mntNoWait)
	if err != nil {
		return nil, err
	}
	buf := make([]syscall.Statfs_t, n)
	if n, err = syscall.Getfsstat(buf, mntNoWait); err != nil {
		return nil, err
	}
	var out []FSUsage
	for i := range buf[:n] {
		if u := fromStatfs(&buf[i]); u.Total > 0 {
			out = append(out, u)
		}
	}
	return out, nil
}
