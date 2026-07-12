package api

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/server-probe/server/internal/service"
)

// PrometheusHandler Prometheus 指标端点处理器
// 零依赖实现（不使用 prometheus client library），直接输出 Prometheus text format
type PrometheusHandler struct {
	monitor *service.MonitorService
}

// NewPrometheusHandler 创建 Prometheus 指标处理器
func NewPrometheusHandler(monitor *service.MonitorService) *PrometheusHandler {
	return &PrometheusHandler{monitor: monitor}
}

// escapeLabelValue 转义 Prometheus 标签值中的特殊字符
func escapeLabelValue(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	return s
}

// HandleMetrics 输出 Prometheus text format 指标
// 路由: GET /metrics
func (h *PrometheusHandler) HandleMetrics(c *gin.Context) {
	c.Header("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

	items := h.monitor.GetDashboardData()

	w := c.Writer

	// --- CPU 使用率 ---
	fmt.Fprintf(w, "# HELP server_probe_cpu_usage CPU usage percentage\n")
	fmt.Fprintf(w, "# TYPE server_probe_cpu_usage gauge\n")
	for _, item := range items {
		fmt.Fprintf(w, "server_probe_cpu_usage{agent_id=\"%d\",hostname=\"%s\"} %v\n",
			item.AgentID, escapeLabelValue(item.Hostname), item.CPU)
	}

	// --- 内存使用率 ---
	fmt.Fprintf(w, "# HELP server_probe_mem_usage Memory usage percentage\n")
	fmt.Fprintf(w, "# TYPE server_probe_mem_usage gauge\n")
	for _, item := range items {
		fmt.Fprintf(w, "server_probe_mem_usage{agent_id=\"%d\",hostname=\"%s\"} %v\n",
			item.AgentID, escapeLabelValue(item.Hostname), item.Mem)
	}

	// --- 内存总量 ---
	fmt.Fprintf(w, "# HELP server_probe_mem_total_bytes Total memory in bytes\n")
	fmt.Fprintf(w, "# TYPE server_probe_mem_total_bytes gauge\n")
	for _, item := range items {
		fmt.Fprintf(w, "server_probe_mem_total_bytes{agent_id=\"%d\",hostname=\"%s\"} %d\n",
			item.AgentID, escapeLabelValue(item.Hostname), item.MemTotal)
	}

	// --- 内存已用 ---
	fmt.Fprintf(w, "# HELP server_probe_mem_used_bytes Used memory in bytes\n")
	fmt.Fprintf(w, "# TYPE server_probe_mem_used_bytes gauge\n")
	for _, item := range items {
		fmt.Fprintf(w, "server_probe_mem_used_bytes{agent_id=\"%d\",hostname=\"%s\"} %d\n",
			item.AgentID, escapeLabelValue(item.Hostname), item.MemUsed)
	}

	// --- 磁盘使用率 ---
	fmt.Fprintf(w, "# HELP server_probe_disk_usage Disk usage percentage\n")
	fmt.Fprintf(w, "# TYPE server_probe_disk_usage gauge\n")
	for _, item := range items {
		fmt.Fprintf(w, "server_probe_disk_usage{agent_id=\"%d\",hostname=\"%s\"} %v\n",
			item.AgentID, escapeLabelValue(item.Hostname), item.DiskUsage)
	}

	// --- 网络接收速率 ---
	fmt.Fprintf(w, "# HELP server_probe_net_rx_bytes Network receive speed in bytes per second\n")
	fmt.Fprintf(w, "# TYPE server_probe_net_rx_bytes gauge\n")
	for _, item := range items {
		fmt.Fprintf(w, "server_probe_net_rx_bytes{agent_id=\"%d\",hostname=\"%s\"} %d\n",
			item.AgentID, escapeLabelValue(item.Hostname), item.NetRx)
	}

	// --- 网络发送速率 ---
	fmt.Fprintf(w, "# HELP server_probe_net_tx_bytes Network transmit speed in bytes per second\n")
	fmt.Fprintf(w, "# TYPE server_probe_net_tx_bytes gauge\n")
	for _, item := range items {
		fmt.Fprintf(w, "server_probe_net_tx_bytes{agent_id=\"%d\",hostname=\"%s\"} %d\n",
			item.AgentID, escapeLabelValue(item.Hostname), item.NetTx)
	}

	// --- 运行时间 ---
	fmt.Fprintf(w, "# HELP server_probe_uptime_seconds Server uptime in seconds\n")
	fmt.Fprintf(w, "# TYPE server_probe_uptime_seconds gauge\n")
	for _, item := range items {
		fmt.Fprintf(w, "server_probe_uptime_seconds{agent_id=\"%d\",hostname=\"%s\"} %d\n",
			item.AgentID, escapeLabelValue(item.Hostname), item.Uptime)
	}

	// --- 在线状态 ---
	fmt.Fprintf(w, "# HELP server_probe_online Agent online status (1=online, 0=offline)\n")
	fmt.Fprintf(w, "# TYPE server_probe_online gauge\n")
	for _, item := range items {
		onlineVal := 0
		if item.Online {
			onlineVal = 1
		}
		fmt.Fprintf(w, "server_probe_online{agent_id=\"%d\",hostname=\"%s\"} %d\n",
			item.AgentID, escapeLabelValue(item.Hostname), onlineVal)
	}

	// --- TCP 连接数 ---
	fmt.Fprintf(w, "# HELP server_probe_tcp_connections Number of TCP connections\n")
	fmt.Fprintf(w, "# TYPE server_probe_tcp_connections gauge\n")
	for _, item := range items {
		fmt.Fprintf(w, "server_probe_tcp_connections{agent_id=\"%d\",hostname=\"%s\"} %d\n",
			item.AgentID, escapeLabelValue(item.Hostname), item.TCPConns)
	}

	// --- UDP 连接数 ---
	fmt.Fprintf(w, "# HELP server_probe_udp_connections Number of UDP connections\n")
	fmt.Fprintf(w, "# TYPE server_probe_udp_connections gauge\n")
	for _, item := range items {
		fmt.Fprintf(w, "server_probe_udp_connections{agent_id=\"%d\",hostname=\"%s\"} %d\n",
			item.AgentID, escapeLabelValue(item.Hostname), item.UDPConns)
	}

	// --- 进程数 ---
	fmt.Fprintf(w, "# HELP server_probe_process_count Number of running processes\n")
	fmt.Fprintf(w, "# TYPE server_probe_process_count gauge\n")
	for _, item := range items {
		fmt.Fprintf(w, "server_probe_process_count{agent_id=\"%d\",hostname=\"%s\"} %d\n",
			item.AgentID, escapeLabelValue(item.Hostname), item.ProcessCount)
	}

	// --- 1 分钟负载 ---
	fmt.Fprintf(w, "# HELP server_probe_load_1 1-minute load average\n")
	fmt.Fprintf(w, "# TYPE server_probe_load_1 gauge\n")
	for _, item := range items {
		fmt.Fprintf(w, "server_probe_load_1{agent_id=\"%d\",hostname=\"%s\"} %v\n",
			item.AgentID, escapeLabelValue(item.Hostname), item.Load1)
	}

	// --- 5 分钟负载 ---
	fmt.Fprintf(w, "# HELP server_probe_load_5 5-minute load average\n")
	fmt.Fprintf(w, "# TYPE server_probe_load_5 gauge\n")
	for _, item := range items {
		fmt.Fprintf(w, "server_probe_load_5{agent_id=\"%d\",hostname=\"%s\"} %v\n",
			item.AgentID, escapeLabelValue(item.Hostname), item.Load5)
	}

	// --- 15 分钟负载 ---
	fmt.Fprintf(w, "# HELP server_probe_load_15 15-minute load average\n")
	fmt.Fprintf(w, "# TYPE server_probe_load_15 gauge\n")
	for _, item := range items {
		fmt.Fprintf(w, "server_probe_load_15{agent_id=\"%d\",hostname=\"%s\"} %v\n",
			item.AgentID, escapeLabelValue(item.Hostname), item.Load15)
	}
}
