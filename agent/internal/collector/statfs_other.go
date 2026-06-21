//go:build !linux

package collector

import (
	"fmt"
)

// statFS 非 Linux 平台的占位实现
// 在 Linux 服务器上运行时使用 linux 版本
func statFS(path string) (total uint64, free uint64, err error) {
	return 0, 0, fmt.Errorf("statfs not supported on this platform")
}
