package collector

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/server-probe/shared/model"
)

// cpuStat 保存 /proc/stat 中 cpu 行的各字段值
type cpuStat struct {
	User      uint64
	Nice      uint64
	System    uint64
	Idle      uint64
	IOWait    uint64
	IRQ       uint64
	SoftIRQ   uint64
	Steal     uint64
	Guest     uint64
	GuestNice uint64
}

// total 返回所有字段的总和
func (s cpuStat) total() uint64 {
	return s.User + s.Nice + s.System + s.Idle + s.IOWait + s.IRQ + s.SoftIRQ + s.Steal + s.Guest + s.GuestNice
}

// busy 返回非空闲字段的总和
func (s cpuStat) busy() uint64 {
	return s.User + s.Nice + s.System + s.IRQ + s.SoftIRQ + s.Steal
}

// CPUCollector CPU 采集器
type CPUCollector struct {
	reader    FileReader
	prevStat  *cpuStat
	prevUsage float64
}

// NewCPUCollector 创建 CPU 采集器
func NewCPUCollector(reader FileReader) *CPUCollector {
	return &CPUCollector{
		reader: reader,
	}
}

// Name 返回采集器名称
func (c *CPUCollector) Name() string {
	return "cpu"
}

// Collect 采集 CPU 数据
func (c *CPUCollector) Collect() (interface{}, error) {
	statData, err := c.reader.ReadFile(ProcPath + "/stat")
	if err != nil {
		return nil, fmt.Errorf("读取 /proc/stat 失败: %w", err)
	}

	currentStat, err := parseCPUStat(string(statData))
	if err != nil {
		return nil, fmt.Errorf("解析 /proc/stat 失败: %w", err)
	}

	// 计算使用率
	var usage float64
	if c.prevStat != nil {
		prevTotal := c.prevStat.total()
		prevBusy := c.prevStat.busy()
		currentTotal := currentStat.total()
		currentBusy := currentStat.busy()

		if currentTotal > prevTotal {
			totalDelta := currentTotal - prevTotal
			var busyDelta uint64
			if currentBusy > prevBusy {
				busyDelta = currentBusy - prevBusy
			}
			usage = float64(busyDelta) / float64(totalDelta) * 100
		} else {
			usage = c.prevUsage
		}
	}

	c.prevStat = &currentStat
	c.prevUsage = usage

	// 解析 CPU 信息
	cpuinfoData, err := c.reader.ReadFile(ProcPath + "/cpuinfo")
	if err != nil {
		return nil, fmt.Errorf("读取 /proc/cpuinfo 失败: %w", err)
	}

	cores, cpuModel, err := parseCPUInfo(string(cpuinfoData))
	if err != nil {
		return nil, fmt.Errorf("解析 /proc/cpuinfo 失败: %w", err)
	}

	// 解析负载
	loadavgData, err := c.reader.ReadFile(ProcPath + "/loadavg")
	if err != nil {
		return nil, fmt.Errorf("读取 /proc/loadavg 失败: %w", err)
	}

	load1, load5, load15, err := parseLoadavg(string(loadavgData))
	if err != nil {
		return nil, fmt.Errorf("解析 /proc/loadavg 失败: %w", err)
	}

	return model.CPUInfo{
		Usage:  roundFloat(usage, 2),
		Cores:  cores,
		Model:  cpuModel,
		Load1:  load1,
		Load5:  load5,
		Load15: load15,
	}, nil
}

// parseCPUStat 解析 /proc/stat 中的 cpu 行
func parseCPUStat(data string) (cpuStat, error) {
	lines := strings.Split(data, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "cpu ") {
			fields := strings.Fields(line)
			if len(fields) < 5 {
				return cpuStat{}, fmt.Errorf("cpu 行字段不足")
			}

			stat := cpuStat{}
			// fields[0] 是 "cpu"
			values := make([]uint64, len(fields)-1)
			for i := 1; i < len(fields); i++ {
				val, err := strconv.ParseUint(fields[i], 10, 64)
				if err != nil {
					return cpuStat{}, fmt.Errorf("解析字段 %d 失败: %w", i, err)
				}
				values[i-1] = val
			}

			// 按顺序赋值
			if len(values) > 0 {
				stat.User = values[0]
			}
			if len(values) > 1 {
				stat.Nice = values[1]
			}
			if len(values) > 2 {
				stat.System = values[2]
			}
			if len(values) > 3 {
				stat.Idle = values[3]
			}
			if len(values) > 4 {
				stat.IOWait = values[4]
			}
			if len(values) > 5 {
				stat.IRQ = values[5]
			}
			if len(values) > 6 {
				stat.SoftIRQ = values[6]
			}
			if len(values) > 7 {
				stat.Steal = values[7]
			}
			if len(values) > 8 {
				stat.Guest = values[8]
			}
			if len(values) > 9 {
				stat.GuestNice = values[9]
			}

			return stat, nil
		}
	}
	return cpuStat{}, fmt.Errorf("未找到 cpu 行")
}

// parseCPUInfo 解析 /proc/cpuinfo，返回核心数和型号
func parseCPUInfo(data string) (int, string, error) {
	cores := 0
	cpuModel := ""

	lines := strings.Split(data, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "processor") {
			cores++
		} else if strings.HasPrefix(line, "model name") {
			// 取最后一个 model name（多核同型号）
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				cpuModel = strings.TrimSpace(parts[1])
			}
		}
	}

	if cores == 0 {
		return 0, "", fmt.Errorf("未找到 processor 行")
	}

	return cores, cpuModel, nil
}

// parseLoadavg 解析 /proc/loadavg
// 格式: 0.52 0.48 0.50 2/156 3456
func parseLoadavg(data string) (float64, float64, float64, error) {
	fields := strings.Fields(strings.TrimSpace(data))
	if len(fields) < 3 {
		return 0, 0, 0, fmt.Errorf("loadavg 字段不足")
	}

	load1, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("解析 load1 失败: %w", err)
	}

	load5, err := strconv.ParseFloat(fields[1], 64)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("解析 load5 失败: %w", err)
	}

	load15, err := strconv.ParseFloat(fields[2], 64)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("解析 load15 失败: %w", err)
	}

	return load1, load5, load15, nil
}

// roundFloat 保留 n 位小数
func roundFloat(val float64, n int) float64 {
	multiplier := 1.0
	for i := 0; i < n; i++ {
		multiplier *= 10
	}
	return float64(int64(val*multiplier+0.5)) / multiplier
}
