//go:build !darwin && !linux

package dashboard

func statfs(string) (uint64, bool) { return 0, false }
