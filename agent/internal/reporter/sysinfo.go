package reporter

import (
	"crypto/sha256"
	"encoding/hex"
	"net"
	"os"
	"runtime"
	"strings"
)

// executeHostname 获取主机名
func executeHostname() (string, error) {
	return os.Hostname()
}

// getHostFingerprint 生成主机指纹 (基于硬件特征)
// 算法: hostname + MAC地址 + CPU型号 + 机器ID 的 SHA256
func getHostFingerprint() string {
	hostname, _ := os.Hostname()

	// 采集第一个非虚拟 MAC 地址
	macAddr := getPrimaryMAC()

	// 采集 CPU 型号
	cpuModel := getCPUModel()

	// 采集机器 ID (/etc/machine-id 或 /var/lib/dbus/machine-id)
	machineID := getMachineID()

	// 组合生成指纹
	data := hostname + "|" + macAddr + "|" + cpuModel + "|" + machineID + "|" + runtime.GOOS + "|" + runtime.GOARCH
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

// getPrimaryMAC 获取第一个非虚拟网卡的 MAC 地址
func getPrimaryMAC() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "unknown"
	}

	for _, iface := range interfaces {
		// 跳过回环和未启用的接口
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}
		// 跳过虚拟网卡 (通常 MAC 以 00:15:5d 开头的是 Hyper-V, fe:.. 是虚拟)
		mac := iface.HardwareAddr.String()
		if mac == "" {
			continue
		}
		// 跳过全零 MAC
		if mac == "00:00:00:00:00:00" {
			continue
		}
		// 跳过常见的虚拟网卡前缀
		if strings.HasPrefix(mac, "00:15:5d") || // Hyper-V
			strings.HasPrefix(mac, "08:00:27") || // VirtualBox
			strings.HasPrefix(mac, "00:0c:29") || // VMware
			strings.HasPrefix(mac, "00:50:56") || // VMware ESX
			strings.HasPrefix(mac, "52:54:00") { // QEMU/KVM
			continue
		}
		return mac
	}

	// 如果没有找到物理网卡，返回第一个有 MAC 的接口
	for _, iface := range interfaces {
		mac := iface.HardwareAddr.String()
		if mac != "" && mac != "00:00:00:00:00:00" {
			return mac
		}
	}

	return "unknown"
}

// getCPUModel 获取 CPU 型号
func getCPUModel() string {
	data, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return "unknown"
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "model name") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return "unknown"
}

// getMachineID 获取机器 ID
func getMachineID() string {
	// 尝试读取 systemd machine-id
	for _, path := range []string{"/etc/machine-id", "/var/lib/dbus/machine-id"} {
		data, err := os.ReadFile(path)
		if err == nil {
			return strings.TrimSpace(string(data))
		}
	}
	return "unknown"
}
