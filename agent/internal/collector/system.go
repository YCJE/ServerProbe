package collector

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/server-probe/shared/model"
)

// SystemCollector 系统信息采集器
type SystemCollector struct {
	reader       FileReader
	agentVersion string
}

// NewSystemCollector 创建系统信息采集器
func NewSystemCollector(reader FileReader, agentVersion string) *SystemCollector {
	return &SystemCollector{
		reader:       reader,
		agentVersion: agentVersion,
	}
}

// Name 返回采集器名称
func (c *SystemCollector) Name() string {
	return "system"
}

// Collect 采集系统信息
func (c *SystemCollector) Collect() (interface{}, error) {
	// 获取主机名
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	// 获取系统信息
	osName := runtime.GOOS
	arch := runtime.GOARCH

	// 读取内核版本
	kernel := ""
	if kernelData, err := c.reader.ReadFile(ProcPath + "/sys/kernel/osrelease"); err == nil {
		kernel = strings.TrimSpace(string(kernelData))
	}

	// 检测虚拟化类型
	virtualization := c.detectVirtualization()

	// 检测 Linux 发行版
	distro := c.detectDistro()

	return model.SystemInfo{
		OS:             osName,
		Arch:           arch,
		Kernel:         kernel,
		Hostname:       hostname,
		AgentVersion:   c.agentVersion,
		Virtualization: virtualization,
		Distro:         distro,
	}, nil
}

// detectVirtualization 检测虚拟化/容器环境
// 检查顺序：容器（cgroup > environ）> 虚拟机（cpuinfo hypervisor flag + DMI）
// 返回值如 "KVM", "LXC", "Docker", "OpenVZ", "VMware", ""（空表示物理机）
func (c *SystemCollector) detectVirtualization() string {
	// 1. 检查 /proc/1/cgroup 中的容器痕迹（docker/lxc/k8s）
	if cgroupData, err := c.reader.ReadFile(ProcPath + "/1/cgroup"); err == nil {
		cgroupStr := string(cgroupData)
		switch {
		case strings.Contains(cgroupStr, "docker"):
			return "Docker"
		case strings.Contains(cgroupStr, "containerd"):
			return "containerd"
		case strings.Contains(cgroupStr, "kubepods"):
			return "Kubernetes"
		case strings.Contains(cgroupStr, "lxc"):
			return "LXC"
		}
	}

	// 2. 检查 /proc/1/environ 中的容器环境变量
	// /proc/1/environ 是以 null 分隔的 KEY=VALUE 列表
	if environData, err := c.reader.ReadFile(ProcPath + "/1/environ"); err == nil {
		envVars := strings.Split(string(environData), "\x00")
		for _, env := range envVars {
			// container=lxc 表示 LXC 容器
			if strings.HasPrefix(env, "container=lxc") {
				return "LXC"
			}
			// KUBERNETES_SERVICE_HOST 存在表示运行在 K8s Pod 中
			if strings.HasPrefix(env, "KUBERNETES_SERVICE_HOST=") {
				return "Kubernetes"
			}
		}
	}

	// 3. 检查 OpenVZ（/proc/bc/ 或 /proc/vz/ 存在）
	if _, err := c.reader.ReadFile(ProcPath + "/bc/0/resources"); err == nil {
		return "OpenVZ"
	}
	if _, err := c.reader.ReadFile(ProcPath + "/vz/veinfo"); err == nil {
		return "OpenVZ"
	}

	// 4. 检查 /proc/cpuinfo 中的 hypervisor flag
	if cpuinfoData, err := c.reader.ReadFile(ProcPath + "/cpuinfo"); err == nil {
		if hasHypervisorFlag(string(cpuinfoData)) {
			// 存在 hypervisor flag，说明运行在虚拟机中
			// 进一步通过 DMI 信息确定具体的虚拟化平台
			return c.detectVMType()
		}
	}

	return "" // 物理机
}

// detectVMType 通过 DMI 信息确定具体的虚拟化类型
// 仅当 cpuinfo 中存在 hypervisor flag 时调用
func (c *SystemCollector) detectVMType() string {
	// 读取 DMI 系统厂商信息
	if vendorData, err := c.reader.ReadFile("/sys/class/dmi/id/sys_vendor"); err == nil {
		vendor := strings.ToLower(strings.TrimSpace(string(vendorData)))
		switch {
		case strings.Contains(vendor, "vmware"):
			return "VMware"
		case strings.Contains(vendor, "qemu"):
			return "KVM"
		case strings.Contains(vendor, "kvm"):
			return "KVM"
		case strings.Contains(vendor, "microsoft"):
			return "Hyper-V"
		case strings.Contains(vendor, "xen"):
			return "Xen"
		}
	}

	// 读取 DMI 产品名称作为补充判断
	if productData, err := c.reader.ReadFile("/sys/class/dmi/id/product_name"); err == nil {
		product := strings.ToLower(strings.TrimSpace(string(productData)))
		switch {
		case strings.Contains(product, "vmware"):
			return "VMware"
		case strings.Contains(product, "kvm"):
			return "KVM"
		case strings.Contains(product, "qemu"):
			return "KVM"
		case strings.Contains(product, "virtualbox"):
			return "KVM"
		}
	}

	// 有 hypervisor flag 但无法确定具体类型，默认返回 KVM
	return "KVM"
}

// hasHypervisorFlag 检查 /proc/cpuinfo 中 flags 行是否包含 hypervisor 标志
func hasHypervisorFlag(cpuinfoData string) bool {
	lines := strings.Split(cpuinfoData, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "flags") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				flags := strings.Fields(parts[1])
				for _, flag := range flags {
					if flag == "hypervisor" {
						return true
					}
				}
			}
		}
	}
	return false
}

// detectDistro 检测 Linux 发行版名称
// 读取 /etc/os-release 文件，解析 PRETTY_NAME 字段
func (c *SystemCollector) detectDistro() string {
	data, err := c.reader.ReadFile("/etc/os-release")
	if err != nil {
		return ""
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "PRETTY_NAME=") {
			// PRETTY_NAME="Ubuntu 22.04 LTS"
			value := strings.TrimPrefix(line, "PRETTY_NAME=")
			// 去除首尾引号
			value = strings.Trim(value, "\"'")
			return strings.TrimSpace(value)
		}
	}

	return ""
}

// CollectUptime 单独采集运行时间
func (c *SystemCollector) CollectUptime() (uint64, error) {
	uptimeData, err := c.reader.ReadFile(ProcPath + "/uptime")
	if err != nil {
		return 0, fmt.Errorf("读取 /proc/uptime 失败: %w", err)
	}
	return parseUptime(string(uptimeData))
}

// CollectProcessCount 单独采集进程数
func (c *SystemCollector) CollectProcessCount() (int, error) {
	entries, err := c.reader.ReadDir(ProcPath)
	if err != nil {
		return 0, fmt.Errorf("读取 /proc 目录失败: %w", err)
	}

	// 直接在循环内计数，避免先收集所有 entry 名字到 []string 再计数的多余 O(n) 拷贝
	count := 0
	for _, entry := range entries {
		if _, err := strconv.Atoi(entry.Name()); err == nil {
			count++
		}
	}
	return count, nil
}

// parseUptime 解析 /proc/uptime
// 格式: 86400.50 85432.20
func parseUptime(data string) (uint64, error) {
	fields := strings.Fields(strings.TrimSpace(data))
	if len(fields) < 1 {
		return 0, fmt.Errorf("uptime 格式无效")
	}

	uptime, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, fmt.Errorf("解析 uptime 失败: %w", err)
	}

	return uint64(uptime), nil
}
