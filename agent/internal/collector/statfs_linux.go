//go:build linux

package collector

import (
	"syscall"
)

// statFS 获取文件系统统计信息（Linux 实现）
// 返回值 total = Blocks*Bsize，free = Bfree*Bsize（含 root 保留块）。
// 调用方按 used = total - free = (Blocks - Bfree)*Bsize 计算，与标准 df 的 used 一致；
// 若改用 Bavail（不含保留块）会让 used 把 reserved 算进去，导致与 df 不一致。
func statFS(path string) (total uint64, free uint64, err error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, err
	}

	total = stat.Blocks * uint64(stat.Bsize)
	free = stat.Bfree * uint64(stat.Bsize)

	return total, free, nil
}
