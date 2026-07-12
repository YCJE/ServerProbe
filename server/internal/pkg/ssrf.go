package pkg

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// SSRFProtector SSRF 防护器
type SSRFProtector struct {
	client *http.Client
}

// NewSSRFProtector 创建 SSRF 防护器
func NewSSRFProtector() *SSRFProtector {
	// 自定义 Transport，强制使用预解析 IP
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}

			// 解析所有 IP 地址
			ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
			if err != nil {
				return nil, fmt.Errorf("DNS 解析失败: %w", err)
			}

			// 检查 DNS 解析是否返回了 IP
			if len(ips) == 0 {
				return nil, fmt.Errorf("DNS 解析未返回任何 IP: %s", host)
			}

			// 检查所有解析到的 IP
			for _, ip := range ips {
				if isPrivateIP(ip) {
					return nil, fmt.Errorf("目标地址 %s 解析到内网 IP %s", host, ip)
				}
			}

			// 连接到第一个非内网 IP
			dialer := &net.Dialer{Timeout: 10 * time.Second}
			return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
		},
	}

	client := &http.Client{
		Timeout:   10 * time.Second,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			// 检查重定向目标
			if err := CheckURL(req.URL.String()); err != nil {
				return fmt.Errorf("重定向到不安全地址: %w", err)
			}
			return nil
		},
	}

	return &SSRFProtector{client: client}
}

// CheckURL 检查 URL 是否安全
func CheckURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("URL 解析失败: %w", err)
	}

	// 只允许 http/https
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("只允许 http/https 协议")
	}

	// 检查主机名是否是 IP 地址
	host := parsed.Hostname()
	if ip := net.ParseIP(host); ip != nil {
		if isPrivateIP(ip) {
			return fmt.Errorf("目标地址是内网 IP: %s", host)
		}
	}

	// 解析主机名（带超时，避免 DNS 不响应时阻塞告警通知）
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return fmt.Errorf("DNS 解析失败: %w", err)
	}

	for _, ip := range ips {
		if isPrivateIP(ip) {
			return fmt.Errorf("目标地址 %s 解析到内网 IP %s", host, ip)
		}
	}

	return nil
}

// SendWebhook 发送 Webhook 请求（带 SSRF 防护）
func (p *SSRFProtector) SendWebhook(targetURL string, body []byte) (int, []byte, error) {
	// 发送前检查 URL
	if err := CheckURL(targetURL); err != nil {
		return 0, nil, fmt.Errorf("SSRF 防护拦截: %w", err)
	}

	req, err := http.NewRequest("POST", targetURL, strings.NewReader(string(body)))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	// 限制响应体读取（最多 1KB）
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1024))
	if err != nil {
		return resp.StatusCode, nil, err
	}

	return resp.StatusCode, respBody, nil
}

// CheckHostPort 检查主机名:端口是否安全（供非 HTTP 协议如 SMTP 复用）
// 与 CheckURL 相同的内网 IP 检查逻辑，但不限制协议
func CheckHostPort(host string, port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("端口范围无效: %d", port)
	}

	// 检查主机名是否是 IP 地址
	if ip := net.ParseIP(host); ip != nil {
		if isPrivateIP(ip) {
			return fmt.Errorf("目标地址是内网 IP: %s", host)
		}
	}

	// 解析主机名（带超时）
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return fmt.Errorf("DNS 解析失败: %w", err)
	}

	for _, ip := range ips {
		if isPrivateIP(ip) {
			return fmt.Errorf("目标地址 %s 解析到内网 IP %s", host, ip)
		}
	}

	return nil
}

// CheckIP 检查单个 IP 是否安全（非内网）
func CheckIP(ip net.IP) error {
	if isPrivateIP(ip) {
		return fmt.Errorf("目标地址是内网 IP: %s", ip)
	}
	return nil
}

// isPrivateIP 检查是否是内网 IP
func isPrivateIP(ip net.IP) bool {
	// 未指定地址 (0.0.0.0, ::) - 某些系统上等同于 localhost
	if ip.IsUnspecified() {
		return true
	}
	// IPv4 内网地址段
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}

	if ip4 := ip.To4(); ip4 != nil {
		// 0.0.0.0/8 ("本网络" 网段)
		if ip4[0] == 0 {
			return true
		}
		// 10.0.0.0/8
		if ip4[0] == 10 {
			return true
		}
		// 100.64.0.0/10 - CGNAT (RFC 6598)
		if ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127 {
			return true
		}
		// 172.16.0.0/12
		if ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31 {
			return true
		}
		// 192.168.0.0/16
		if ip4[0] == 192 && ip4[1] == 168 {
			return true
		}
		// 127.0.0.0/8
		if ip4[0] == 127 {
			return true
		}
		// 169.254.0.0/16
		if ip4[0] == 169 && ip4[1] == 254 {
			return true
		}
		return false
	}

	// IPv6
	// ::1
	if ip.IsLoopback() {
		return true
	}
	// fc00::/7
	if len(ip) == 16 && (ip[0]&0xfe) == 0xfc {
		return true
	}

	return false
}
