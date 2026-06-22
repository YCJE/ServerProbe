package model

// MetricData 是 Agent 上报的完整监控数据
type MetricData struct {
	CPU          CPUInfo      `json:"cpu"`
	Memory       MemoryInfo   `json:"memory"`
	Disks        []DiskInfo   `json:"disk"`
	Network      NetworkInfo  `json:"network"`
	Uptime       uint64       `json:"uptime"`
	ProcessCount int          `json:"process_count"`
	System       SystemInfo   `json:"system"`
}

// CPUInfo CPU 监控数据
type CPUInfo struct {
	Usage  float64 `json:"usage"`   // 使用率 0-100
	Cores  int     `json:"cores"`   // 核心数
	Model  string  `json:"model"`   // CPU 型号
	Load1  float64 `json:"load_1"`  // 1 分钟负载
	Load5  float64 `json:"load_5"`  // 5 分钟负载
	Load15 float64 `json:"load_15"` // 15 分钟负载
}

// MemoryInfo 内存监控数据
type MemoryInfo struct {
	Total     uint64 `json:"total"`      // 总内存（字节）
	Used      uint64 `json:"used"`       // 已用内存（字节）
	SwapTotal uint64 `json:"swap_total"` // Swap 总量
	SwapUsed  uint64 `json:"swap_used"`  // Swap 已用
}

// DiskInfo 磁盘分区数据
type DiskInfo struct {
	Device string `json:"device"` // 挂载点，如 /
	Total  uint64 `json:"total"`  // 总容量（字节）
	Used   uint64 `json:"used"`   // 已用容量（字节）
}

// NetworkInfo 网络监控数据
type NetworkInfo struct {
	RxSpeed        uint64 `json:"rx_speed"`         // 下行速率（字节/秒）
	TxSpeed        uint64 `json:"tx_speed"`         // 上行速率（字节/秒）
	TCPConnections int    `json:"tcp_connections"`  // TCP 连接数
	UDPConnections int    `json:"udp_connections"`  // UDP 连接数
}

// SystemInfo 系统信息
type SystemInfo struct {
	OS           string `json:"os"`            // 操作系统
	Arch         string `json:"arch"`          // 架构
	Kernel       string `json:"kernel"`        // 内核版本
	Hostname     string `json:"hostname"`      // 主机名
	AgentVersion string `json:"agent_version"` // Agent 版本
}

// PingResult Ping 探测结果
type PingResult struct {
	Target      string  `json:"target"`       // 探测目标 IP/域名
	Name        string  `json:"name"`         // 显示名称（如"电信"）
	Method      string  `json:"method"`       // icmp / icmp_unprivileged / tcp / http
	AvgLatency  float64 `json:"avg_latency"`  // 平均延迟（毫秒）
	MinLatency  float64 `json:"min_latency"`  // 最小延迟（毫秒）
	MaxLatency  float64 `json:"max_latency"`  // 最大延迟（毫秒）
	Jitter      float64 `json:"jitter"`       // 抖动（毫秒）
	Loss        float64 `json:"loss"`         // 丢包率 0-100
	PacketsSent int     `json:"packets_sent"` // 发送包数
	PacketsRecv int     `json:"packets_recv"` // 接收包数
}

// PingTarget 探测目标配置
type PingTarget struct {
	ID      int64  `json:"id"`
	Target  string `json:"target"` // IP 或域名
	Name    string `json:"name"`   // 显示名称
	Method  string `json:"method"` // 探测方式: icmp, tcp, http
	Enabled bool   `json:"enabled"`
}

// AgentConfig Agent 从 Server 拉取的配置
type AgentConfig struct {
	PingTargets  []PingTarget `json:"ping_targets"`
	PingInterval int          `json:"ping_interval"` // 探测间隔（秒）
}
