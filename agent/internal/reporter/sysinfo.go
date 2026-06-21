package reporter

import (
	"os"
	"runtime"
)

// executeHostname 获取主机名
func executeHostname() (string, error) {
	return os.Hostname()
}

// executeOS 获取操作系统信息
func executeOS() string {
	return runtime.GOOS + "/" + runtime.GOARCH
}
