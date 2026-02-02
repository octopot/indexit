//go:build darwin || linux

package dashboard

import "syscall"

func statfs(path string) (uint64, bool) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, false
	}
	return st.Bavail * uint64(st.Bsize), true //nolint:gosec,unconvert // Bsize is uint32 on darwin, int64 on linux
}
