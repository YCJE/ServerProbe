package collector

import (
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/prometheus-community/pro-bing"
	sharedmodel "github.com/server-probe/shared/model"
)

// PingMethod Ping 探测方式
type PingMethod string

const (
	PingMethodICMP             PingMethod = "icmp"
	PingMethodICMPUnprivileged PingMethod = "icmp_unprivileged"
	PingMethodTCP              PingMethod = "tcp"
	PingMethodHTTP             PingMethod = "http"
	PingMethodAuto             PingMethod = "auto"
)

// PingCollector Ping 探测采集器
type PingCollector struct {
	method         PingMethod
	detectedOnce   bool
	detectedMethod PingMethod
}

// NewPingCollector 创建 Ping 采集器
func NewPingCollector(method string) *PingCollector {
	return &PingCollector{
		method: PingMethod(method),
	}
}

// Name 返回采集器名称
func (c *PingCollector) Name() string {
	return "ping"
}

// Collect 采集 Ping 数据（实现 Collector 接口）
func (c *PingCollector) Collect() (interface{}, error) {
	return nil, fmt.Errorf("请使用 PingTargets 方法")
}

// PingTargets 对多个目标执行 Ping 探测
func (c *PingCollector) PingTargets(targets []sharedmodel.PingTarget) []sharedmodel.PingResult {
	results := make([]sharedmodel.PingResult, 0, len(targets))

	for _, target := range targets {
		if !target.Enabled {
			continue
		}

		result := c.pingTarget(target)
		results = append(results, result)
	}

	return results
}

// pingTarget 对单个目标执行 Ping 探测
func (c *PingCollector) pingTarget(target sharedmodel.PingTarget) sharedmodel.PingResult {
	result := sharedmodel.PingResult{
		Target: target.Target,
		Name:   target.Name,
	}

	method := c.method
	if method == PingMethodAuto {
		method = c.detectBestMethod()
	}

	switch method {
	case PingMethodICMP, PingMethodICMPUnprivileged:
		c.doICMPPing(&result, target.Target, method)
	case PingMethodTCP:
		c.doTCPPing(&result, target.Target)
	case PingMethodHTTP:
		c.doHTTPPing(&result, target.Target)
	default:
		c.doICMPPing(&result, target.Target, PingMethodICMP)
	}

	return result
}

// detectBestMethod 自动检测最佳 Ping 方式（缓存检测结果）
func (c *PingCollector) detectBestMethod() PingMethod {
	// 如果已检测过，直接返回缓存的结果
	if c.detectedOnce {
		return c.detectedMethod
	}

	// 尝试 privileged ICMP
	if canPrivilegedICMP() {
		c.detectedMethod = PingMethodICMP
		c.detectedOnce = true
		return PingMethodICMP
	}

	// 尝试 unprivileged ICMP
	if canUnprivilegedICMP() {
		c.detectedMethod = PingMethodICMPUnprivileged
		c.detectedOnce = true
		return PingMethodICMPUnprivileged
	}

	// 降级到 TCP
	c.detectedMethod = PingMethodTCP
	c.detectedOnce = true
	return PingMethodTCP
}

// doICMPPing 执行 ICMP Ping
func (c *PingCollector) doICMPPing(result *sharedmodel.PingResult, target string, method PingMethod) {
	pinger, err := probing.NewPinger(target)
	if err != nil {
		result.Loss = 100
		result.Method = string(method)
		return
	}

	pinger.Count = 10
	pinger.Interval = 500 * time.Millisecond
	pinger.Timeout = 15 * time.Second

	// 设置探测方式
	if method == PingMethodICMPUnprivileged {
		pinger.SetPrivileged(false)
	} else {
		pinger.SetPrivileged(true)
	}

	// 预解析 DNS，排除 DNS 时间
	if ip := net.ParseIP(target); ip == nil {
		// 是域名，预解析
		ips, err := net.LookupIP(target)
		if err != nil || len(ips) == 0 {
			result.Loss = 100
			result.Method = string(method)
			return
		}
		// 使用解析到的 IP 创建新的 pinger
		pinger, err = probing.NewPinger(ips[0].String())
		if err != nil {
			result.Loss = 100
			result.Method = string(method)
			return
		}
		pinger.Count = 10
		pinger.Interval = 500 * time.Millisecond
		pinger.Timeout = 15 * time.Second
		if method == PingMethodICMPUnprivileged {
			pinger.SetPrivileged(false)
		} else {
			pinger.SetPrivileged(true)
		}
	}

	err = pinger.Run()
	if err != nil {
		result.Loss = 100
		result.Method = string(method)
		return
	}

	stats := pinger.Statistics()
	result.Method = string(method)
	result.AvgLatency = float64(stats.AvgRtt.Microseconds()) / 1000.0
	result.MinLatency = float64(stats.MinRtt.Microseconds()) / 1000.0
	result.MaxLatency = float64(stats.MaxRtt.Microseconds()) / 1000.0
	result.Jitter = float64(stats.StdDevRtt.Microseconds()) / 1000.0
	result.PacketsSent = stats.PacketsSent
	result.PacketsRecv = stats.PacketsRecv

	if stats.PacketsSent > 0 {
		result.Loss = float64(stats.PacketsSent-stats.PacketsRecv) / float64(stats.PacketsSent) * 100
	}
}

// doTCPPing 执行 TCP Ping
func (c *PingCollector) doTCPPing(result *sharedmodel.PingResult, target string) {
	result.Method = string(PingMethodTCP)

	// 预解析 DNS
	host, port, err := net.SplitHostPort(target)
	if err != nil {
		// 没有端口，使用默认端口 80
		host = target
		port = "80"
	}

	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		result.Loss = 100
		return
	}

	addr := net.JoinHostPort(ips[0].String(), port)

	count := 5
	successCount := 0
	var latencies []float64

	for i := 0; i < count; i++ {
		start := time.Now()
		conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
		elapsed := time.Since(start)

		if err == nil {
			conn.Close()
			successCount++
			latencies = append(latencies, float64(elapsed.Microseconds())/1000.0)
		}

		if i < count-1 {
			time.Sleep(500 * time.Millisecond)
		}
	}

	result.PacketsSent = count
	result.PacketsRecv = successCount

	if count > 0 {
		result.Loss = float64(count-successCount) / float64(count) * 100
	}

	if len(latencies) > 0 {
		var sum, min, max float64
		min = latencies[0]
		max = latencies[0]

		for _, lat := range latencies {
			sum += lat
			if lat < min {
				min = lat
			}
			if lat > max {
				max = lat
			}
		}

		result.AvgLatency = sum / float64(len(latencies))
		result.MinLatency = min
		result.MaxLatency = max

		// 计算抖动（标准差）
		if len(latencies) > 1 {
			mean := result.AvgLatency
			var variance float64
			for _, lat := range latencies {
				variance += (lat - mean) * (lat - mean)
			}
			result.Jitter = sqrtFloat(variance / float64(len(latencies)))
		}
	}
}

// doHTTPPing 执行 HTTP Ping
func (c *PingCollector) doHTTPPing(result *sharedmodel.PingResult, target string) {
	result.Method = string(PingMethodHTTP)

	count := 3
	successCount := 0
	var latencies []float64

	for i := 0; i < count; i++ {
		// 使用自定义 DialContext 排除 DNS 时间
		// 简化版：直接测量总时间
		elapsed := measureHTTPTime(target)
		if elapsed > 0 {
			successCount++
			latencies = append(latencies, elapsed)
		}

		if i < count-1 {
			time.Sleep(500 * time.Millisecond)
		}
	}

	result.PacketsSent = count
	result.PacketsRecv = successCount

	if count > 0 {
		result.Loss = float64(count-successCount) / float64(count) * 100
	}

	if len(latencies) > 0 {
		var sum float64
		for _, lat := range latencies {
			sum += lat
		}
		result.AvgLatency = sum / float64(len(latencies))
		result.MinLatency = latencies[0]
		result.MaxLatency = latencies[0]
		for _, lat := range latencies {
			if lat < result.MinLatency {
				result.MinLatency = lat
			}
			if lat > result.MaxLatency {
				result.MaxLatency = lat
			}
		}
	}
}

// measureHTTPTime 测量 HTTP 请求时间（排除 DNS）
func measureHTTPTime(url string) float64 {
	// 预解析 DNS
	parsed, err := parseURL(url)
	if err != nil {
		return -1
	}

	ips, err := net.LookupIP(parsed.host)
	if err != nil || len(ips) == 0 {
		return -1
	}

	// 连接到预解析的 IP
	start := time.Now()
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(ips[0].String(), parsed.port), 10*time.Second)
	if err != nil {
		return -1
	}
	defer conn.Close()

	elapsed := time.Since(start)
	return float64(elapsed.Microseconds()) / 1000.0
}

// parsedURL 解析后的 URL
type parsedURL struct {
	host string
	port string
}

// parseURL 解析 URL（使用 net/url 标准库，支持 IPv6）
func parseURL(rawURL string) (*parsedURL, error) {
	// 如果没有 scheme，添加临时的 http:// 以便 url.Parse 正确解析主机和端口
	if !strings.Contains(rawURL, "://") {
		rawURL = "http://" + rawURL
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	host := u.Hostname()
	if host == "" {
		return nil, fmt.Errorf("URL 中缺少主机名")
	}

	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}

	return &parsedURL{host: host, port: port}, nil
}

// sqrtFloat 计算平方根
func sqrtFloat(x float64) float64 {
	if x <= 0 {
		return 0
	}
	// 牛顿法
	z := x
	for i := 0; i < 10; i++ {
		z = z - (z*z-x)/(2*z)
	}
	return z
}

// canPrivilegedICMP 检查是否可以使用 privileged ICMP
func canPrivilegedICMP() bool {
	// 尝试创建 privileged pinger
	pinger, err := probing.NewPinger("127.0.0.1")
	if err != nil {
		return false
	}
	pinger.SetPrivileged(true)
	pinger.Count = 1
	pinger.Timeout = 1 * time.Second
	err = pinger.Run()
	return err == nil
}

// canUnprivilegedICMP 检查是否可以使用 unprivileged ICMP
func canUnprivilegedICMP() bool {
	pinger, err := probing.NewPinger("127.0.0.1")
	if err != nil {
		return false
	}
	pinger.SetPrivileged(false)
	pinger.Count = 1
	pinger.Timeout = 1 * time.Second
	err = pinger.Run()
	return err == nil
}
