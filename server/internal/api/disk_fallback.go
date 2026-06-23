//go:build !linux

package api

// getDiskSpace 获取指定路径的磁盘空间 (跨平台 fallback)
// 返回: 可用空间, 总空间
// 在非 Linux 平台上返回 0, 0
func getDiskSpace(path string) (free uint64, total uint64) {
	return 0, 0
}
