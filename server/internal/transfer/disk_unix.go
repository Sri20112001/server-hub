//go:build linux || darwin

package transfer

import "golang.org/x/sys/unix"

func diskUsage(path string) (total, used, available uint64, err error) {
	var stat unix.Statfs_t
	if err = unix.Statfs(path, &stat); err != nil {
		return
	}
	total = stat.Blocks * uint64(stat.Bsize)
	available = stat.Bavail * uint64(stat.Bsize)
	used = total - uint64(stat.Bfree)*uint64(stat.Bsize)
	return
}
