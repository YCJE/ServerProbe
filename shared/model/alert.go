package model

import "time"

// AlertRule 告警规则
type AlertRule struct {
	ID              int64     `json:"id"`
	Name            string    `json:"name"`
	Metric          string    `json:"metric"`      // cpu_usage / mem_usage / disk_usage / agent_offline
	Operator        string    `json:"operator"`    // > / < / =
	Threshold       float64   `json:"threshold"`
	Duration        int       `json:"duration"`    // 持续时间（秒）
	Enabled         bool      `json:"enabled"`
	NotifyChannelID int64     `json:"notify_channel_id"`
	CreatedAt       time.Time `json:"created_at"`
}

// 告警支持的指标
const (
	MetricCPUUsage    = "cpu_usage"
	MetricMemUsage    = "mem_usage"
	MetricDiskUsage   = "disk_usage"
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

// AlertEvent 告警事件记录
type AlertEvent struct {
	ID        int64     `json:"id"`
	RuleID    int64     `json:"rule_id"`
	AgentID   int64     `json:"agent_id"`
	State     AlertState `json:"state"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}
