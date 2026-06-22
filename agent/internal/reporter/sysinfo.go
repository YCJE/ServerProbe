package reporter

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"runtime"
)

// executeHostname 获取主机名
func executeHostname() (string, error) {
	return os.Hostname()
}

// getHostFingerprint 生成主机指纹 (基于机器特征)
func getHostFingerprint() string {
	hostname, _ := os.Hostname()
	data := hostname + "|" + runtime.GOOS + "|" + runtime.GOARCH
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}
