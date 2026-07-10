package collector

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"
)

// DNSResult 单条 DNS 解析结果
type DNSResult struct {
	RecordType string `json:"record_type"` // 记录类型：A, AAAA, MX, NS, CNAME, TXT, PTR
	Value      string `json:"value"`       // 解析值
	TTL        int    `json:"ttl"`         // TTL（秒），标准库不提供 TTL，固定为 0
	ElapsedMs  int64  `json:"elapsed_ms"`  // 查询耗时（毫秒）
}

// DNSCollector DNS 查询采集器
// 不是定期采集的，而是按需调用（通过 Server 下发任务）
type DNSCollector struct{}

// NewDNSCollector 创建 DNS 查询采集器
func NewDNSCollector() *DNSCollector {
	return &DNSCollector{}
}

// Name 返回采集器名称
func (c *DNSCollector) Name() string {
	return "dns"
}

// Collect 实现 Collector 接口
// DNS 采集器按需调用，不通过 Collect 定期采集
func (c *DNSCollector) Collect() (interface{}, error) {
	return nil, fmt.Errorf("请使用 QueryDNS 方法")
}

// QueryDNS 执行 DNS 查询
// domain: 要查询的域名
// recordType: 记录类型（A, AAAA, MX, NS, CNAME, TXT, PTR）
// 返回解析结果列表（包含查询耗时）
// 所有查询均使用 context.WithTimeout 设置 5 秒超时，避免标准库默认超时过长
func (c *DNSCollector) QueryDNS(domain string, recordType string) ([]DNSResult, error) {
	start := time.Now()
	var results []DNSResult
	upperType := strings.ToUpper(recordType)

	// 使用 net.Resolver 配合 context 实现超时控制
	resolver := &net.Resolver{}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	switch upperType {
	case "A", "AAAA":
		// 使用 LookupIPAddr 获取所有 IP 地址，然后按类型过滤
		ipAddrs, err := resolver.LookupIPAddr(ctx, domain)
		if err != nil {
			return nil, fmt.Errorf("DNS 查询失败 (%s): %w", upperType, err)
		}
		elapsed := time.Since(start).Milliseconds()
		for _, ipAddr := range ipAddrs {
			// A 记录只返回 IPv4（To4() != nil），AAAA 记录只返回 IPv6（To4() == nil）
			if upperType == "A" && ipAddr.IP.To4() != nil {
				results = append(results, DNSResult{
					RecordType: "A",
					Value:      ipAddr.IP.String(),
					TTL:        0,
					ElapsedMs:  elapsed,
				})
			} else if upperType == "AAAA" && ipAddr.IP.To4() == nil {
				results = append(results, DNSResult{
					RecordType: "AAAA",
					Value:      ipAddr.IP.String(),
					TTL:        0,
					ElapsedMs:  elapsed,
				})
			}
		}

	case "MX":
		mxs, err := resolver.LookupMX(ctx, domain)
		if err != nil {
			return nil, fmt.Errorf("DNS 查询失败 (MX): %w", err)
		}
		elapsed := time.Since(start).Milliseconds()
		for _, mx := range mxs {
			results = append(results, DNSResult{
				RecordType: "MX",
				Value:      fmt.Sprintf("%d %s", mx.Pref, mx.Host),
				TTL:        0,
				ElapsedMs:  elapsed,
			})
		}

	case "NS":
		nss, err := resolver.LookupNS(ctx, domain)
		if err != nil {
			return nil, fmt.Errorf("DNS 查询失败 (NS): %w", err)
		}
		elapsed := time.Since(start).Milliseconds()
		for _, ns := range nss {
			results = append(results, DNSResult{
				RecordType: "NS",
				Value:      ns.Host,
				TTL:        0,
				ElapsedMs:  elapsed,
			})
		}

	case "CNAME":
		cname, err := resolver.LookupCNAME(ctx, domain)
		if err != nil {
			return nil, fmt.Errorf("DNS 查询失败 (CNAME): %w", err)
		}
		elapsed := time.Since(start).Milliseconds()
		results = append(results, DNSResult{
			RecordType: "CNAME",
			Value:      cname,
			TTL:        0,
			ElapsedMs:  elapsed,
		})

	case "TXT":
		txts, err := resolver.LookupTXT(ctx, domain)
		if err != nil {
			return nil, fmt.Errorf("DNS 查询失败 (TXT): %w", err)
		}
		elapsed := time.Since(start).Milliseconds()
		for _, txt := range txts {
			results = append(results, DNSResult{
				RecordType: "TXT",
				Value:      txt,
				TTL:        0,
				ElapsedMs:  elapsed,
			})
		}

	case "PTR":
		names, err := resolver.LookupAddr(ctx, domain)
		if err != nil {
			return nil, fmt.Errorf("DNS 查询失败 (PTR): %w", err)
		}
		elapsed := time.Since(start).Milliseconds()
		for _, name := range names {
			results = append(results, DNSResult{
				RecordType: "PTR",
				Value:      name,
				TTL:        0,
				ElapsedMs:  elapsed,
			})
		}

	default:
		return nil, fmt.Errorf("不支持的记录类型: %s（支持 A, AAAA, MX, NS, CNAME, TXT, PTR）", recordType)
	}

	return results, nil
}
