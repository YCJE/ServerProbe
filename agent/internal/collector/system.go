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

	return model.SystemInfo{
		OS:           osName,
		Arch:         arch,
		Kernel:       kernel,
		Hostname:     hostname,
		AgentVersion: c.agentVersion,
	}, nil
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
	entries, err := os.ReadDir(ProcPath)
	if err != nil {
		return 0, fmt.Errorf("读取 /proc 目录失败: %w", err)
	}

	var dirs []string
	for _, entry := range entries {
		dirs = append(dirs, entry.Name())
	}
	return countProcessDirs(dirs), nil
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

// countProcessDirs 统计 /proc 目录中纯数字目录的数量（即进程数）
func countProcessDirs(dirs []string) int {
	count := 0
	for _, dir := range dirs {
		if _, err := strconv.Atoi(dir); err == nil {
			count++
		}
	}
	return count
}
