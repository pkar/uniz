package fileops

import (
	"syscall"
	"time"
)

func fillSys(si *StatInfo) {
	st, ok := si.Info.Sys().(*syscall.Stat_t)
	if !ok {
		return
	}
	si.HasSys = true
	si.Dev, si.Ino, si.Nlink = uint64(uint32(st.Dev)), st.Ino, uint64(st.Nlink)
	si.UID, si.GID = st.Uid, st.Gid
	si.Blocks, si.BlockSize = st.Blocks, int64(st.Blksize)
	si.Atime = time.Unix(st.Atimespec.Unix())
	si.Ctime = time.Unix(st.Ctimespec.Unix())
	if st.Birthtimespec.Sec != 0 || st.Birthtimespec.Nsec != 0 {
		si.Birth = time.Unix(st.Birthtimespec.Unix())
	}
}
