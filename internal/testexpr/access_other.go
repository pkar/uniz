//go:build !unix

package testexpr

import "os"

// access approximates access(2) from permission bits on non-Unix systems.
func access(path string, mode uint32) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	perm := uint32(info.Mode().Perm())
	return perm&(mode<<6) != 0 || perm&(mode<<3) != 0 || perm&mode != 0
}
