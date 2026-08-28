package model

import (
	"time"
)

// Agent 表示已注册的 Agent 元数据（GORM 模型）
type Agent struct {
	ID              int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	Token           string    `gorm:"uniqueIndex;not null" json:"-"`
	Hostname        string    `gorm:"not null" json:"hostname"`
	DisplayName     string    `json:"display_name"` // 用户自定义名称
	OS              string    `json:"os"`
	Arch            string    `json:"arch"`
	Kernel          string    `json:"kernel"`           // 内核版本
	Virtualization  string    `json:"virtualization"`   // 虚拟化类型（KVM/LXC/Docker/VMware/空）
	Distro          string    `json:"distro"`           // Linux 发行版名称
	AgentVersion    string    `json:"agent_version"`
	HostFingerprint string    `gorm:"uniqueIndex" json:"-"`
	Tags            string    `json:"tags"` // P1-10: 逗号分隔的标签（如 "web,production"）
	// NodeGet 风格元数据（全部由管理员设置，Agent 上报不覆盖）
	Region            string     `json:"region"`               // 位置："上海"/"Tokyo"/"US-LAX"
	CountryCode       string     `json:"country_code"`         // 国家代码："CN"/"JP"/"US"（旗帜+地图）
	ISP               string     `json:"isp"`                  // 供应商备注："Bandwagon"/"Oracle"
	ExpiresAt         *time.Time `json:"expires_at"`           // 到期时间（nil=永不过期）
	PriceAmount       float64    `json:"price_amount"`         // 周期费用数值
	PriceCurrency     string     `json:"price_currency"`       // 币种: CNY/USD/EUR/JPY
	PriceCycle        string     `json:"price_cycle"`          // 周期: monthly/yearly
	TrafficQuotaBytes int64      `json:"traffic_quota_bytes"`  // 月流量配额字节数（0=不限）
	IPv4              string     `json:"ipv4"`                 // 出口 IPv4（从 WS 连接 RemoteAddr 获取）
	IPv6              string     `json:"ipv6"`                 // 出口 IPv6
	LastSeen          time.Time  `json:"last_seen"`
	Online            bool       `gorm:"default:false" json:"online"`
	CreatedAt         time.Time  `gorm:"autoCreateTime" json:"created_at"`
}

// TableName 指定表名
func (Agent) TableName() string { return "agents" }

// RegisterCode 注册码（GORM 模型）
type RegisterCode struct {
	Code          string    `gorm:"primaryKey" json:"code"`
	DisplayName   string    `json:"display_name"` // 用户自定义服务器名称
	Remark        string    `json:"remark"`       // 备注
	CreatedAt     time.Time `gorm:"autoCreateTime" json:"created_at"`
	ExpiresAt     time.Time `gorm:"not null" json:"expires_at"`
	Used          bool      `gorm:"default:false" json:"used"`
	UsedByAgentID int64     `gorm:"index" json:"used_by_agent_id"`
}

// TableName 指定表名
func (RegisterCode) TableName() string { return "register_codes" }

// AlertRule 告警规则（GORM 模型）
type AlertRule struct {
	ID              int64   `gorm:"primaryKey;autoIncrement" json:"id"`
	Name            string  `gorm:"not null" json:"name"`
	Metric          string  `gorm:"not null" json:"metric"`
	Operator        string  `gorm:"not null" json:"operator"`
	Threshold       float64 `gorm:"not null" json:"threshold"`
	Duration        int     `gorm:"not null" json:"duration"`
	Enabled         bool    `gorm:"default:true" json:"enabled"`
	NotifyChannelID int64   `gorm:"index" json:"notify_channel_id"`
	CreatedAt       time.Time `gorm:"autoCreateTime" json:"created_at"`
}

// TableName 指定表名
func (AlertRule) TableName() string { return "alert_rules" }

// 告警支持的指标
const (
	MetricCPUUsage      = "cpu_usage"
	MetricMemUsage      = "mem_usage"
	MetricDiskUsage     = "disk_usage"
	MetricAgentOffline  = "agent_offline"
	MetricServiceStatus = "service_status"   // P0-3: 服务监控（1=down, 0=up）
	MetricSSLCertExpiry = "ssl_cert_expiry"  // P0-4: SSL 证书剩余天数
	MetricTrafficQuota  = "traffic_quota"    // v1.1: 月流量使用率（百分比）
	MetricExpireDays    = "expire_days"      // v1.1: VPS 剩余到期天数
)

// 告警支持的操作符
const (
	OpGreaterThan = ">"
	OpLessThan    = "<"
	OpEqual       = "="
)

// AlertState 告警状态
type AlertState string

const (
	AlertStateOK       AlertState = "OK"
	AlertStatePending  AlertState = "PENDING"
	AlertStateFiring   AlertState = "FIRING"
	AlertStateResolved AlertState = "RESOLVED"
)

// AlertHistory 告警历史记录（FIRING 触发与 RESOLVED 恢复的持久化时间线）
type AlertHistory struct {
	ID            int64      `gorm:"primaryKey;autoIncrement" json:"id"`
	RuleID        int64      `gorm:"index" json:"rule_id"`
	RuleName      string     `json:"rule_name"`
	AgentID       int64      `gorm:"index" json:"agent_id"`
	ServerName    string     `json:"server_name"`
	Metric        string     `json:"metric"`
	State         string     `gorm:"index" json:"state"` // firing / resolved
	Value         float64    `json:"value"`              // 触发时的指标值
	ResolvedValue float64    `json:"resolved_value"`     // 恢复时的指标值
	Message       string     `json:"message"`
	TriggeredAt   time.Time  `gorm:"index" json:"triggered_at"`
	ResolvedAt    *time.Time `json:"resolved_at"`
}

// TableName 指定表名
func (AlertHistory) TableName() string { return "alert_history" }

// Tag 标签（NodeGet 风格：彩色标签徽章，管理员创建，Agent 通过名称引用）
type Tag struct {
	ID        int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	Name      string    `gorm:"uniqueIndex;not null" json:"name"`
	Color     string    `json:"color"` // 十六进制色值，如 "#3b82f6"
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
}

// TableName 指定表名
func (Tag) TableName() string { return "tags" }

// NotifyChannel 通知渠道（GORM 模型）
type NotifyChannel struct {
	ID        int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	Name      string    `gorm:"not null" json:"name"`
	Type      string    `gorm:"not null" json:"type"`
	Config    string    `gorm:"not null" json:"config"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
}

// TableName 指定表名
func (NotifyChannel) TableName() string { return "notify_channels" }

// 通知渠道类型
const (
	NotifyTypeWebhook  = "webhook"
	NotifyTypeTelegram = "telegram"
	NotifyTypeEmail    = "email"
)

// WebhookConfig Webhook 通知配置
type WebhookConfig struct {
	URL    string `json:"url"`
	Secret string `json:"secret"`
}

// TelegramConfig Telegram 通知配置
type TelegramConfig struct {
	BotToken string `json:"bot_token"`
	ChatID   string `json:"chat_id"`
}

// EmailConfig 邮件通知配置
type EmailConfig struct {
	SMTPHost string `json:"smtp_host"`
	SMTPPort int    `json:"smtp_port"`
	Username string `json:"username"`
	Password string `json:"password"`
	From     string `json:"from"`
	To       string `json:"to"`
	UseTLS   bool   `json:"use_tls"`
}

// PingTarget 探测目标（GORM 模型）
type PingTarget struct {
    ID        int64     `gorm:"primaryKey;autoIncrement" json:"id"`
    Name      string    `gorm:"not null" json:"name"`
    Target    string    `gorm:"not null" json:"target"`
    Method    string    `gorm:"default:icmp" json:"method"`
    Enabled   bool      `gorm:"default:true" json:"enabled"`
    SortOrder int       `gorm:"default:0" json:"sort_order"`
    IPVersion int       `gorm:"default:0" json:"ip_version"` // 0=自动识别，4=IPv4，6=IPv6
    CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
}

// TableName 指定表名
func (PingTarget) TableName() string { return "ping_targets" }

// MetricRecord 历史聚合数据（每5分钟一个点）
//
// P3 缩放整数存储规则：
// 为减少 SQLite 存储空间并提升查询效率，CPUUsage、Load1、Load5、Load15
// 均以"实际值 × 10"的整数形式存储。例如 CPU 53.4% 存为 534，Load 1.23 存为 12。
//   - 写入时（见 service/aggregation.go）：int(math.Round(value * 10))
//   - 读取时（见 api/handler_server.go）：value / 10.0 还原为浮点数
type MetricRecord struct {
	ID            int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	AgentID       int64  `gorm:"index:idx_metric_records_agent_time,priority:1;not null" json:"agent_id"`
	Timestamp     int64  `gorm:"index:idx_metric_records_agent_time,priority:2;not null" json:"timestamp"`
	CPUUsage      int    `gorm:"type:integer" json:"cpu_usage"`  // 存储 ×10 的值（53.4% → 534）
	MemUsage      float64 `json:"mem_usage"`
	MemTotal      uint64  `json:"mem_total"`
	MemUsed       uint64  `json:"mem_used"`
	SwapTotal     uint64  `json:"swap_total"`
	SwapUsed      uint64  `json:"swap_used"`
	DiskUsage     string  `json:"disk_usage"`
	NetRx         int64   `json:"net_rx"`
	NetTx         int64   `json:"net_tx"`
	TCPConns      int     `gorm:"column:tcp_connections" json:"tcp_connections"`
	UDPConns      int     `gorm:"column:udp_connections" json:"udp_connections"`
	Load1         int    `gorm:"column:load_1;type:integer" json:"load_1"`     // 存储 ×10 的值
	Load5         int    `gorm:"column:load_5;type:integer" json:"load_5"`     // 存储 ×10 的值
	Load15        int    `gorm:"column:load_15;type:integer" json:"load_15"`    // 存储 ×10 的值
	Uptime        uint64  `json:"uptime"`
	ProcessCount  int     `json:"process_count"`
	PingData      string  `json:"ping_data"`
	// Offline 离线标记（0=在线, 1=离线占位记录）
	// 采用反转语义使旧行（AutoMigrate 加列默认 0）自然表示"在线"，
	// 离线时段由聚合服务写入占位记录（Offline=1），用于在线率时间线
	Offline int `gorm:"default:0" json:"offline"`
}

// TableName 指定表名
func (MetricRecord) TableName() string { return "metric_records" }

// MetricRecordHourly 小时级聚合记录
// 由聚合服务从 metric_records（5 分钟层）滚动聚合生成，用于 7d/30d/90d/1y 长范围查询。
// timestamp 对齐整点；upsert 冲突键 (agent_id, timestamp)，整小时由 12 条 5 分钟行
// 全量重算后覆盖写入，任意时刻重跑幂等。
// ×10 整数缩放规则与 metric_records 一致（写入 ×10，读取 /10.0）。
type MetricRecordHourly struct {
	ID        int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	AgentID   int64  `gorm:"uniqueIndex:idx_hourly_agent_time,priority:1;not null" json:"agent_id"`
	Timestamp int64  `gorm:"uniqueIndex:idx_hourly_agent_time,priority:2;not null" json:"timestamp"`
	// 均值列
	CPUUsage int     `gorm:"type:integer" json:"cpu_usage"` // ×10
	MemUsage float64 `json:"mem_usage"`
	Load1    int     `gorm:"column:load_1;type:integer" json:"load_1"` // ×10
	Load5    int     `gorm:"column:load_5;type:integer" json:"load_5"` // ×10
	Load15   int     `gorm:"column:load_15;type:integer" json:"load_15"` // ×10
	NetRx    int64   `json:"net_rx"`
	NetTx    int64   `json:"net_tx"`
	// 极值列（小时内 5 分钟记录的最小/最大值，捕获被均值抹平的尖峰）
	CPUMin   int     `gorm:"column:cpu_min;type:integer" json:"cpu_min"` // ×10
	CPUMax   int     `gorm:"column:cpu_max;type:integer" json:"cpu_max"` // ×10
	MemMin   float64 `gorm:"column:mem_min" json:"mem_min"`
	MemMax   float64 `gorm:"column:mem_max" json:"mem_max"`
	Load1Max int     `gorm:"column:load_1_max;type:integer" json:"load_1_max"` // ×10
	NetRxMax int64   `gorm:"column:net_rx_max" json:"net_rx_max"`
	NetTxMax int64   `gorm:"column:net_tx_max" json:"net_tx_max"`
	// 固定值列（取小时内最后一条在线记录，与 5 分钟层口径一致）
	MemTotal     uint64 `json:"mem_total"`
	MemUsed      uint64 `json:"mem_used"`
	SwapTotal    uint64 `json:"swap_total"`
	SwapUsed     uint64 `json:"swap_used"`
	DiskUsage    string `json:"disk_usage"`
	Uptime       uint64 `json:"uptime"`
	ProcessCount int    `json:"process_count"`
	TCPConns     int    `gorm:"column:tcp_connections" json:"tcp_connections"`
	UDPConns     int    `gorm:"column:udp_connections" json:"udp_connections"`
	PingData     string `json:"ping_data"`
	// 在线状态与样本统计
	Offline        int `gorm:"default:0" json:"offline"` // 多数规则：offline_samples >= 总样本/2 → 1
	SampleCount    int `json:"sample_count"`             // 该小时实际存在的 5 分钟记录总数
	OfflineSamples int `json:"offline_samples"`         // 其中离线占位行数
}

// TableName 指定表名
func (MetricRecordHourly) TableName() string { return "metric_records_hourly" }

// Admin 管理员账户（GORM 模型）
type Admin struct {
	ID           int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	Username     string    `gorm:"uniqueIndex;not null" json:"username"`
	PasswordHash string    `gorm:"not null" json:"-"`
	TOTPSecret   string    `json:"-"`
	TOTPEnabled  bool      `gorm:"default:false" json:"totp_enabled"`
	CreatedAt    time.Time `gorm:"autoCreateTime" json:"created_at"`
}

// TableName 指定表名
func (Admin) TableName() string { return "admin" }

// AuditLog 管理员操作审计日志（P1：安全闭环）
// 记录登录事件与管理端的全部变更操作（POST/PUT/DELETE），
// 不记录请求体（可能含密码/TOTP 密钥等敏感字段），仅记录路由与目标标识
type AuditLog struct {
	ID        int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	AdminID   int64     `gorm:"index" json:"admin_id"`
	Username  string    `json:"username"`           // 冗余存储用户名，管理员删除后日志仍可读
	Action    string    `gorm:"index;not null" json:"action"` // 如 "DELETE /api/v1/agents/:id"、"auth.login"
	Target    string    `json:"target"`             // 实际请求路径（含具体 ID），如 "/api/v1/agents/42"
	Success   bool      `json:"success"`            // HTTP 状态码 < 400 视为成功
	IP        string    `json:"ip"`
	UserAgent string    `json:"user_agent"`
	CreatedAt time.Time `gorm:"autoCreateTime;index" json:"created_at"`
}

// TableName 指定表名
func (AuditLog) TableName() string { return "audit_logs" }

// SharePage 公开分享页配置（GORM 模型）
type SharePage struct {
	ID          int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	ShareID     string    `gorm:"uniqueIndex;not null" json:"share_id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	AgentIDs   string    `json:"agent_ids"`  // 逗号分隔的 Agent ID（空=全部）
	Enabled     bool      `gorm:"default:true" json:"enabled"`
	SortOrder   int       `gorm:"default:0" json:"sort_order"`
	CreatedAt   time.Time `gorm:"autoCreateTime" json:"created_at"`
}

// TableName 指定表名
func (SharePage) TableName() string { return "share_pages" }

// SystemSetting 系统配置 (键值对存储)
type SystemSetting struct {
	Key   string `gorm:"primaryKey" json:"key"`
	Value string `gorm:"not null" json:"value"`
}

// TableName 指定表名
func (SystemSetting) TableName() string { return "system_settings" }

// TrafficRecord 每日流量统计（P0-1）
// 由聚合服务每 5 分钟计算增量并 upsert 到当日记录
type TrafficRecord struct {
	ID        int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	AgentID   int64     `gorm:"uniqueIndex:idx_traffic_agent_date,priority:1;not null" json:"agent_id"`
	Date      string    `gorm:"uniqueIndex:idx_traffic_agent_date,priority:2;not null" json:"date"` // "2006-01-02"
	RXBytes   uint64    `json:"rx_bytes"`
	TXBytes   uint64    `json:"tx_bytes"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (TrafficRecord) TableName() string { return "traffic_records" }

// ServiceMonitor 服务监控配置（P0-3）
// 服务端主动探测 HTTP/TCP 端点可用性
type ServiceMonitor struct {
	ID             int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	Name           string    `gorm:"not null" json:"name"`
	Type           string    `gorm:"not null" json:"type"`            // "http" | "tcp"
	Target         string    `gorm:"not null" json:"target"`          // HTTP: 完整 URL; TCP: host:port
	ExpectedStatus int       `gorm:"default:200" json:"expected_status"` // HTTP 期望状态码
	Timeout        int       `gorm:"default:10" json:"timeout"`       // 超时秒数
	Interval       int       `gorm:"default:60" json:"interval"`      // 探测间隔秒数
	Enabled        bool      `gorm:"default:true" json:"enabled"`
	LastStatus     string    `json:"last_status"`   // "up" | "down"
	LastLatency    float64   `json:"last_latency"`  // 毫秒
	LastChecked    time.Time `json:"last_checked"`
	CreatedAt      time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (ServiceMonitor) TableName() string { return "service_monitors" }

// SSLCertMonitor SSL 证书到期监控配置（P0-4）
// 服务端定时检查目标域名 TLS 证书有效期
type SSLCertMonitor struct {
	ID               int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	Domain           string    `gorm:"not null" json:"domain"`
	Port             int       `gorm:"default:443" json:"port"`
	AlertDays        int       `gorm:"default:30" json:"alert_days"` // 剩余天数 < 此值时告警
	Enabled          bool      `gorm:"default:true" json:"enabled"`
	LastExpiryDate   time.Time `json:"last_expiry_date"`
	LastRemainingDays int      `json:"last_remaining_days"`
	LastChecked      time.Time `json:"last_checked"`
	CreatedAt        time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (SSLCertMonitor) TableName() string { return "ssl_cert_monitors" }
