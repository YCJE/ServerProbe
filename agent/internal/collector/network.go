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
	// 仅统计 ESTABLISHED 状态(st=="01")，排除 LISTEN/TIME_WAIT 等非活跃连接
	tcpCount := countConnections(string(tcpData), "01")

	// 同时读取 /proc/net/tcp6 统计 IPv6 上的 ESTABLISHED 连接
	// （IPv6 禁用时该文件可能不存在，忽略错误）
	if tcp6Data, err := c.reader.ReadFile(ProcPath + "/net/tcp6"); err == nil {
		tcpCount += countConnections(string(tcp6Data), "01")
	}

	udpData, err := c.reader.ReadFile(ProcPath + "/net/udp")
	if err != nil {
		return nil, fmt.Errorf("读取 /proc/net/udp 失败: %w", err)
	}
	// UDP 无连接概念，统计所有 socket
	udpCount := countConnections(string(udpData), "")

	// 同时读取 /proc/net/udp6 统计 IPv6 上的 UDP socket
	// （IPv6 禁用时该文件可能不存在，忽略错误）
	if udp6Data, err := c.reader.ReadFile(ProcPath + "/net/udp6"); err == nil {
		udpCount += countConnections(string(udp6Data), "")
	}

	return model.NetworkInfo{
		RxSpeed:        rxSpeed,
		TxSpeed:        txSpeed,
		TotalRx:        currentRx,
		TotalTx:        currentTx,
		TCPConnections: tcpCount,
		UDPConnections: udpCount,
	}, nil
}

// virtualPrefixes 容器/虚拟网络接口前缀，这些接口在 Docker/K8s 宿主上
// 会导致流量被重复计算（宿主与容器间流量计入两次），采集时需排除。
var virtualPrefixes = []string{"veth", "br-", "docker", "cni", "flannel", "cilium"}

// isVirtualIface 判断是否为虚拟/容器接口
func isVirtualIface(name string) bool {
	for _, p := range virtualPrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// parseNetDev 解析 /proc/net/dev，返回总 RX 和 TX 字节数
// 排除 lo 回环接口以及 veth/br-/docker 等虚拟接口
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
		// 排除回环接口与虚拟/容器接口，避免双重计数
		if iface == "lo" || isVirtualIface(iface) {
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
// 跳过表头行。stateFilter 为空时统计所有条目；非空时仅统计指定状态
// （如 "01" 表示 TCP ESTABLISHED），用于排除 LISTEN/TIME_WAIT 等非活跃状态。
func countConnections(data string, stateFilter string) int {
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
		// fields[3] 为 st（状态）字段
		if len(fields) >= 4 {
			if stateFilter == "" || fields[3] == stateFilter {
				count++
			}
		}
	}

	return count
}

// parseUint 解析无符号整数
func parseUint(s string) (uint64, error) {
	// 空字符串不进入下方循环，会直接返回 (0, nil)，掩盖格式错误，
	// 因此在此显式拦截
	if s == "" {
		return 0, fmt.Errorf("空字符串不是有效的无符号整数")
	}
	var result uint64
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("无效数字: %s", s)
		}
		result = result*10 + uint64(c-'0')
	}
	return result, nil
}
