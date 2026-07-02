package collector

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/server-probe/shared/model"
)

// memInfo 内存信息原始数据
type memInfo struct {
	total     uint64 // MemTotal（字节）
	available uint64 // MemAvailable（字节）
	hasAvail  bool   // 是否解析到 MemAvailable 键（旧内核无此字段）
	free      uint64 // MemFree（字节，旧内核回退用）
	buffers   uint64 // Buffers（字节，旧内核回退用）
	cached    uint64 // Cached（字节，旧内核回退用）
	swapTotal uint64 // SwapTotal（字节）
	swapFree  uint64 // SwapFree（字节）
}

// MemoryCollector 内存采集器
type MemoryCollector struct {
	reader FileReader
}

// NewMemoryCollector 创建内存采集器
func NewMemoryCollector(reader FileReader) *MemoryCollector {
	return &MemoryCollector{reader: reader}
}

// Name 返回采集器名称
func (c *MemoryCollector) Name() string {
	return "memory"
}

// Collect 采集内存数据
func (c *MemoryCollector) Collect() (interface{}, error) {
	data, err := c.reader.ReadFile(ProcPath + "/meminfo")
	if err != nil {
		return nil, fmt.Errorf("读取 /proc/meminfo 失败: %w", err)
	}

	info, err := parseMemInfo(string(data))
	if err != nil {
		return nil, fmt.Errorf("解析 /proc/meminfo 失败: %w", err)
	}

	// Used = Total - Available
	// 旧内核 /proc/meminfo 不含 MemAvailable，回退用 total - free - buffers - cached。
	// 注意 MemAvailable 合法可为 0（内存耗尽时），因此用 hasAvail 标志判断键是否存在，
	// 而非用 available == 0 判断，避免把"内存耗尽"误判为"字段缺失"。
	available := info.available
	if !info.hasAvail {
		available = info.free + info.buffers + info.cached
	}
	used := uint64(0)
	if available <= info.total {
		used = info.total - available
	}

	swapUsed := uint64(0)
	if info.swapFree <= info.swapTotal {
		swapUsed = info.swapTotal - info.swapFree
	}

	return model.MemoryInfo{
		Total:     info.total,
		Used:      used,
		SwapTotal: info.swapTotal,
		SwapUsed:  swapUsed,
	}, nil
}

// parseMemInfo 解析 /proc/meminfo
// 格式: MemTotal:        8388608 kB
func parseMemInfo(data string) (memInfo, error) {
	info := memInfo{}
	lines := strings.Split(data, "\n")

	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		key := strings.TrimSuffix(fields[0], ":")
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}

		// 值以 kB 为单位，转换为字节
		if len(fields) >= 3 && fields[2] == "kB" {
			value *= 1024
		}

		switch key {
		case "MemTotal":
			info.total = value
		case "MemAvailable":
			info.available = value
			info.hasAvail = true
		case "MemFree":
			info.free = value
		case "Buffers":
			info.buffers = value
		case "Cached":
			info.cached = value
		case "SwapTotal":
			info.swapTotal = value
		case "SwapFree":
			info.swapFree = value
		}
	}

	if info.total == 0 {
		return info, fmt.Errorf("未找到 MemTotal")
	}

	return info, nil
}
