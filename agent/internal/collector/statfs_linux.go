//go:build linux

package collector

import (
	"syscall"
)

// statFS 获取文件系统统计信息（Linux 实现）
func statFS(path string) (total uint64, free uint64, err error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, err
	}

	total = stat.Blocks * uint64(stat.Bsize)
	free = stat.Bavail * uint64(stat.Bsize)

	return total, free, nil
}
