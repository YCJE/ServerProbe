package collector

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
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
	detectedMethod PingMethod
	detectOnce     sync.Once
	insecureTLS    bool
}

// NewPingCollector 创建 Ping 采集器
func NewPingCollector(method string, insecureTLS bool) *PingCollector {
	return &PingCollector{
		method:      PingMethod(method),
		insecureTLS: insecureTLS,
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

// PingTargets 对多个目标执行 Ping 探测（并发执行，限制并发数 5）
func (c *PingCollector) PingTargets(targets []sharedmodel.PingTarget) []sharedmodel.PingResult {
	// 先筛选启用的目标，保留原始顺序
	enabled := make([]sharedmodel.PingTarget, 0, len(targets))
	for _, t := range targets {
		if t.Enabled {
			enabled = append(enabled, t)
		}
	}

	n := len(enabled)
	if n == 0 {
		return []sharedmodel.PingResult{}
	}

	// 预分配结果切片，按索引写入（不同索引为独立内存，无需加锁）
	results := make([]sharedmodel.PingResult, n)

	// 使用信号量限制并发数为 5
	sem := make(chan struct{}, 5)
	var wg sync.WaitGroup

	for i := range enabled {
		wg.Add(1)
		go func(idx int, target sharedmodel.PingTarget) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[idx] = c.pingTarget(target)
		}(i, enabled[i])
	}

	wg.Wait()
	return results
}

// pingTarget 对单个目标执行 Ping 探测
func (c *PingCollector) pingTarget(target sharedmodel.PingTarget) sharedmodel.PingResult {
	result := sharedmodel.PingResult{
		Target: target.Target,
		Name:   target.Name,
	}

	method := c.method
	// 优先使用 target 配置的探测方式
	if target.Method != "" {
		switch strings.ToLower(target.Method) {
		case "icmp":
			method = PingMethodICMP
		case "icmp_unprivileged":
			method = PingMethodICMPUnprivileged
		case "tcp":
			method = PingMethodTCP
		case "http":
			method = PingMethodHTTP
		}
	}
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

// detectBestMethod 自动检测最佳 Ping 方式（使用 sync.Once 保证并发安全）
func (c *PingCollector) detectBestMethod() PingMethod {
	c.detectOnce.Do(func() {
		// 如果配置指定了明确的方法，直接使用
		if c.method != PingMethodAuto {
			c.detectedMethod = c.method
			return
		}
		// 自动检测
		if canPrivilegedICMP() {
			c.detectedMethod = PingMethodICMP
		} else if canUnprivilegedICMP() {
			c.detectedMethod = PingMethodICMPUnprivileged
		} else {
			c.detectedMethod = PingMethodTCP
		}
	})
	return c.detectedMethod
}

// sortIPAddrs 对 LookupIPAddr 返回的 IP 列表按字节序排序后返回首个 IP 字符串。
// Go resolver 在返回多 IP 时顺序不稳定（会重排），若直接取 ips[0] 会导致
// 每个采集周期 ping 到不同 IP，结果跳变。排序后保证每次取到同一 IP。
func pickStableIP(ips []net.IPAddr) string {
	if len(ips) == 0 {
		return ""
	}
	sort.Slice(ips, func(i, j int) bool {
		return bytes.Compare(ips[i].IP.To16(), ips[j].IP.To16()) < 0
	})
	return ips[0].String()
}

// doICMPPing 执行 ICMP Ping
func (c *PingCollector) doICMPPing(result *sharedmodel.PingResult, target string, method PingMethod) {
	pinger, err := probing.NewPinger(target)
	if err != nil {
		result.Loss = 100
		result.Method = string(method)
		return
	}

	pinger.Count = 1000
	pinger.Interval = 10 * time.Millisecond
	pinger.Timeout = 15 * time.Second

	// 设置探测方式
	if method == PingMethodICMPUnprivileged {
		pinger.SetPrivileged(false)
	} else {
		pinger.SetPrivileged(true)
	}

	// 预解析 DNS，排除 DNS 时间
	if ip := net.ParseIP(target); ip == nil {
		// 是域名，预解析（带超时，防止 DNS 解析阻塞）
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		resolver := &net.Resolver{}
		ips, err := resolver.LookupIPAddr(ctx, target)
		if err != nil || len(ips) == 0 {
			result.Loss = 100
			result.Method = string(method)
			return
		}
		// 排序后取首个 IP，避免 Go resolver 重排导致每周期 ping 不同 IP
		pinger, err = probing.NewPinger(pickStableIP(ips))
		if err != nil {
			result.Loss = 100
			result.Method = string(method)
			return
		}
		pinger.Count = 1000
		pinger.Interval = 10 * time.Millisecond
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

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	resolver := &net.Resolver{}
	ips, err := resolver.LookupIPAddr(ctx, host)
	if err != nil || len(ips) == 0 {
		result.Loss = 100
		return
	}

	// 排序后取首个 IP，避免 Go resolver 重排导致每周期 ping 不同 IP
	addr := net.JoinHostPort(pickStableIP(ips), port)

	count := 200
	successCount := 0
	attempts := 0
	var latencies []float64
	deadline := time.Now().Add(25 * time.Second) // 整体超时 25s，防止不可达目标阻塞太久

	for i := 0; i < count; i++ {
		if time.Now().After(deadline) {
			break // 超时提前退出
		}
		attempts++
		start := time.Now()
		conn, err := net.DialTimeout("tcp", addr, 1*time.Second)
		elapsed := time.Since(start)

		if err == nil {
			conn.Close()
			successCount++
			latencies = append(latencies, float64(elapsed.Microseconds())/1000.0)
		}

		if i < count-1 {
			time.Sleep(50 * time.Millisecond)
		}
	}

	result.PacketsSent = attempts
	result.PacketsRecv = successCount

	if attempts > 0 {
		result.Loss = float64(attempts-successCount) / float64(attempts) * 100
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
			result.Jitter = math.Sqrt(variance / float64(len(latencies)))
		}
	}
}

// doHTTPPing 执行 HTTP Ping
func (c *PingCollector) doHTTPPing(result *sharedmodel.PingResult, target string) {
	result.Method = string(PingMethodHTTP)

	// 预解析 DNS，排除 DNS 时间
	parsed, err := parseURL(target)
	if err != nil {
		result.Loss = 100
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	resolver := &net.Resolver{}
	ips, err := resolver.LookupIPAddr(ctx, parsed.host)
	if err != nil || len(ips) == 0 {
		result.Loss = 100
		return
	}

	// 排序后取首个 IP，避免 Go resolver 重排导致每周期 ping 不同 IP
	stableIP := pickStableIP(ips)

	// 构建 URL scheme：优先使用 parseURL 解析出的 scheme，
	// 修复 example.com:8443（带 https://）等非标准端口被误判为 http 的问题
	scheme := parsed.scheme
	if scheme == "" {
		scheme = "http"
		if parsed.port == "443" {
			scheme = "https"
		}
	}

	count := 200
	successCount := 0
	attempts := 0
	var latencies []float64
	deadline := time.Now().Add(25 * time.Second) // 整体超时 25s

	// 创建自定义 Transport，使用预解析的 IP 排除 DNS 时间
	dialer := &net.Dialer{Timeout: 1 * time.Second}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			// 替换 addr 中的域名为预解析的 IP
			_, port, err := net.SplitHostPort(addr)
			if err != nil {
				port = parsed.port
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(stableIP, port))
		},
		TLSHandshakeTimeout: 1 * time.Second,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: c.insecureTLS,
			MinVersion:         tls.VersionTLS12,
		},
	}

	client := &http.Client{
		Timeout: 2 * time.Second,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	for i := 0; i < count; i++ {
		if time.Now().After(deadline) {
			break // 超时提前退出
		}
		attempts++
		reqURL := target
		if !strings.Contains(target, "://") {
			reqURL = fmt.Sprintf("%s://%s", scheme, target)
		}

		req, err := http.NewRequest("GET", reqURL, nil)
		if err != nil {
			continue
		}
		// 保留非标准端口到 Host 头，避免按端口路由的虚拟主机路由错误
		if parsed.port != "" && parsed.port != "80" && parsed.port != "443" {
			req.Host = net.JoinHostPort(parsed.host, parsed.port)
		} else {
			req.Host = parsed.host
		}

		start := time.Now()
		resp, err := client.Do(req)
		elapsed := time.Since(start)

		if err == nil {
			// 排空响应体以便复用 TCP 连接（减少 TLS 握手开销）
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			// 仅 2xx/3xx 视为成功，4xx/5xx 计为失败
			if resp.StatusCode >= 200 && resp.StatusCode < 400 {
				successCount++
				latencies = append(latencies, float64(elapsed.Microseconds())/1000.0)
			}
		}

		if i < count-1 {
			time.Sleep(50 * time.Millisecond)
		}
	}

	// 关闭空闲连接，避免连接堆积
	transport.CloseIdleConnections()

	result.PacketsSent = attempts
	result.PacketsRecv = successCount

	if attempts > 0 {
		result.Loss = float64(attempts-successCount) / float64(attempts) * 100
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
			result.Jitter = math.Sqrt(variance / float64(len(latencies)))
		}
	}
}

// parsedURL 解析后的 URL
type parsedURL struct {
	scheme string
	host   string
	port   string
}

// parseURL 解析 URL（使用 net/url 标准库，支持 IPv6）
func parseURL(rawURL string) (*parsedURL, error) {
	// 记录原始输入是否带有 scheme，用于决定最终保留的 scheme
	hadScheme := strings.Contains(rawURL, "://")
	// 如果没有 scheme，添加临时的 http:// 以便 url.Parse 正确解析主机和端口
	if !hadScheme {
		rawURL = "http://" + rawURL
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	// 保留 url.Parse 解析出的 scheme（仅当原始输入确实带 scheme 时才信任），
	// 避免非标准端口（如 example.com:8443 带 https://）被误判为 http
	scheme := u.Scheme
	if scheme == "" || !hadScheme {
		scheme = "http"
	}

	host := u.Hostname()
	if host == "" {
		return nil, fmt.Errorf("URL 中缺少主机名")
	}

	port := u.Port()
	if port == "" {
		if scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}

	return &parsedURL{scheme: scheme, host: host, port: port}, nil
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
