package collector

import (
	"strconv"
	"strings"
)

// ThermalCollector 温度采集器
// 读取 /sys/class/thermal/thermal_zone*/temp（单位：毫摄氏度），取最大有效值作为 CPU 温度。
// 多数 VPS 无 thermal zone（虚拟机不透传传感器），此时返回 0 表示不可用，不视为错误。
type ThermalCollector struct {
	reader FileReader
}

// NewThermalCollector 创建温度采集器
func NewThermalCollector(reader FileReader) *ThermalCollector {
	return &ThermalCollector{reader: reader}
}

// Name 采集器名称
func (c *ThermalCollector) Name() string {
	return "thermal"
}

// SysClassThermalPath /sys/class/thermal 路径，可被测试覆盖
var SysClassThermalPath = "/sys/class/thermal"

// Collect 采集温度，返回 float64（摄氏度）
func (c *ThermalCollector) Collect() (interface{}, error) {
	entries, err := c.reader.ReadDir(SysClassThermalPath)
	if err != nil {
		// 目录不存在：无温度传感器（常见于 VPS），返回 0
		return float64(0), nil
	}

	maxTemp := 0.0
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "thermal_zone") {
			continue
		}
		raw, err := c.reader.ReadFile(SysClassThermalPath + "/" + entry.Name() + "/temp")
		if err != nil {
			continue
		}
		milli, err := strconv.ParseFloat(strings.TrimSpace(string(raw)), 64)
		if err != nil {
			continue
		}
		// 过滤无效读数（< -60°C 或 > 150°C 视为传感器异常）
		celsius := milli / 1000.0
		if celsius < -60 || celsius > 150 {
			continue
		}
		if celsius > maxTemp {
			maxTemp = celsius
		}
	}

	return maxTemp, nil
}
