//go:build linux

package api

import "syscall"

// getDiskSpace 获取指定路径的磁盘空间 (Linux)
// 返回: 可用空间, 总空间
func getDiskSpace(path string) (free uint64, total uint64) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0
	}
	total = stat.Blocks * uint64(stat.Bsize)
	free = stat.Bavail * uint64(stat.Bsize)
	return free, total
}
