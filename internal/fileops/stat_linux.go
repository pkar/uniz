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
	si.Dev, si.Ino, si.Nlink = uint64(st.Dev), uint64(st.Ino), uint64(st.Nlink)
	si.UID, si.GID = st.Uid, st.Gid
	si.Blocks, si.BlockSize = int64(st.Blocks), int64(st.Blksize)
	si.Atime = time.Unix(st.Atim.Unix())
	si.Ctime = time.Unix(st.Ctim.Unix())
}
