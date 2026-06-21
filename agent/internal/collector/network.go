package collector

import (
	"fmt"
	"strings"
	"time"

	"github.com/server-probe/shared/model"
)

// NetworkCollector 网络采集器
type NetworkCollector struct {
	reader      FileReader
	prevRx      uint64
	prevTx      uint64
	prevTime    time.Time
	elapsedSecs float64 // 可设置的间隔（用于测试）
}

// NewNetworkCollector 创建网络采集器
func NewNetworkCollector(reader FileReader) *NetworkCollector {
	return &NetworkCollector{reader: reader}
}

// SetElapsed 设置采集间隔（用于测试）
func (c *NetworkCollector) SetElapsed(secs float64) {
	c.elapsedSecs = secs
}

// Name 返回采集器名称
func (c *NetworkCollector) Name() string {
	return "network"
}

// Collect 采集网络数据
func (c *NetworkCollector) Collect() (interface{}, error) {
	devData, err := c.reader.ReadFile(ProcPath + "/net/dev")
	if err != nil {
		return nil, fmt.Errorf("读取 /proc/net/dev 失败: %w", err)
	}

	currentRx, currentTx, err := parseNetDev(string(devData))
	if err != nil {
		return nil, fmt.Errorf("解析 /proc/net/dev 失败: %w", err)
	}

	// 计算速率
	var rxSpeed, txSpeed uint64
	now := time.Now()

	if c.prevRx > 0 || c.prevTx > 0 {
		var elapsed float64
		if c.elapsedSecs > 0 {
			elapsed = c.elapsedSecs
		} else {
			elapsed = now.Sub(c.prevTime).Seconds()
		}

		if elapsed > 0 {
			if currentRx >= c.prevRx {
				rxSpeed = uint64(float64(currentRx-c.prevRx) / elapsed)
			}
			if currentTx >= c.prevTx {
				txSpeed = uint64(float64(currentTx-c.prevTx) / elapsed)
			}
		}
	}

	c.prevRx = currentRx
	c.prevTx = currentTx
	c.prevTime = now
	c.elapsedSecs = 0 // 重置

	// 统计 TCP/UDP 连接数
	tcpData, err := c.reader.ReadFile(ProcPath + "/net/tcp")
	if err != nil {
		return nil, fmt.Errorf("读取 /proc/net/tcp 失败: %w", err)
	}
	tcpCount := countConnections(string(tcpData))

	udpData, err := c.reader.ReadFile(ProcPath + "/net/udp")
	if err != nil {
		return nil, fmt.Errorf("读取 /proc/net/udp 失败: %w", err)
	}
	udpCount := countConnections(string(udpData))

	return model.NetworkInfo{
		RxSpeed:        rxSpeed,
		TxSpeed:        txSpeed,
		TCPConnections: tcpCount,
		UDPConnections: udpCount,
	}, nil
}

// parseNetDev 解析 /proc/net/dev，返回总 RX 和 TX 字节数
// 排除 lo 回环接口
func parseNetDev(data string) (uint64, uint64, error) {
	var totalRx, totalTx uint64
	lines := strings.Split(data, "\n")

	for i, line := range lines {
		if i < 2 {
			continue // 跳过前两行表头
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// 格式: "eth0: 1048576000 1234567 0 0 0 0 0 0 524288000 987654 0 0 0 0 0 0"
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		iface := strings.TrimSpace(parts[0])
		// 排除回环接口
		if iface == "lo" {
			continue
		}

		fields := strings.Fields(parts[1])
		if len(fields) < 16 {
			continue
		}

		// fields[0] = RX bytes, fields[8] = TX bytes
		rx, err := parseUint(fields[0])
		if err != nil {
			continue
		}
		tx, err := parseUint(fields[8])
		if err != nil {
			continue
		}

		totalRx += rx
		totalTx += tx
	}

	return totalRx, totalTx, nil
}

// countConnections 统计 /proc/net/tcp 或 /proc/net/udp 的连接数
// 跳过表头行
func countConnections(data string) int {
	lines := strings.Split(data, "\n")
	count := 0

	for i, line := range lines {
		if i == 0 {
			continue // 跳过表头
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) >= 4 {
			count++
		}
	}

	return count
}

// parseUint 解析无符号整数
func parseUint(s string) (uint64, error) {
	var result uint64
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("无效数字: %s", s)
		}
		result = result*10 + uint64(c-'0')
	}
	return result, nil
}
