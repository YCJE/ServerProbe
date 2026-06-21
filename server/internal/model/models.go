package model

import (
	"time"
)

// Agent 表示已注册的 Agent 元数据（GORM 模型）
type Agent struct {
	ID              int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	Token           string    `gorm:"uniqueIndex;not null" json:"-"`
	Hostname        string    `gorm:"not null" json:"hostname"`
	OS              string    `json:"os"`
	Arch            string    `json:"arch"`
	AgentVersion    string    `json:"agent_version"`
	HostFingerprint string    `gorm:"uniqueIndex" json:"-"`
	LastSeen        time.Time `json:"last_seen"`
	Online          bool      `gorm:"default:false" json:"online"`
	CreatedAt       time.Time `gorm:"autoCreateTime" json:"created_at"`
}

// TableName 指定表名
func (Agent) TableName() string { return "agents" }

// RegisterCode 注册码（GORM 模型）
type RegisterCode struct {
	Code          string    `gorm:"primaryKey" json:"code"`
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
	MetricCPUUsage     = "cpu_usage"
	MetricMemUsage     = "mem_usage"
	MetricDiskUsage    = "disk_usage"
	MetricAgentOffline = "agent_offline"
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
	Target    string    `gorm:"not null" json:"target"`
	Name      string    `gorm:"not null" json:"name"`
	Enabled   bool      `gorm:"default:true" json:"enabled"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
}

// TableName 指定表名
func (PingTarget) TableName() string { return "ping_targets" }

// MetricRecord 历史聚合数据（每5分钟一个点）
type MetricRecord struct {
	ID        int64   `gorm:"primaryKey;autoIncrement" json:"id"`
	AgentID   int64   `gorm:"index:idx_metric_records_agent_time,priority:1;not null" json:"agent_id"`
	Timestamp int64   `gorm:"index:idx_metric_records_agent_time,priority:2;not null" json:"timestamp"`
	CPUUsage  float64 `json:"cpu_usage"`
	MemUsage  float64 `json:"mem_usage"`
	DiskUsage string  `json:"disk_usage"`
	NetRx     int64   `json:"net_rx"`
	NetTx     int64   `json:"net_tx"`
	PingData  string  `json:"ping_data"`
}

// TableName 指定表名
func (MetricRecord) TableName() string { return "metric_records" }

// Admin 管理员账户（GORM 模型）
type Admin struct {
	ID           int64     `gorm:"primaryKey" json:"id"`
	Username     string    `gorm:"uniqueIndex;not null" json:"username"`
	PasswordHash string    `gorm:"not null" json:"-"`
	TOTPSecret   string    `json:"-"`
	TOTPEnabled  bool      `gorm:"default:false" json:"totp_enabled"`
	CreatedAt    time.Time `gorm:"autoCreateTime" json:"created_at"`
}

// TableName 指定表名
func (Admin) TableName() string { return "admin" }

// SharePage 公开分享页配置（GORM 模型）
type SharePage struct {
	ID        int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	ShareID   string    `gorm:"uniqueIndex;not null" json:"share_id"`
	Title     string    `json:"title"`
	AgentIDs  string    `json:"agent_ids"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
}

// TableName 指定表名
func (SharePage) TableName() string { return "share_pages" }
