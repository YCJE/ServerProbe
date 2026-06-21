package model

// WSMessage 是 WebSocket 通信的顶层消息封装
type WSMessage struct {
	Type      string      `json:"type"`      // 消息类型
	Token     string      `json:"token"`     // Agent Token（register 时为空）
	Timestamp int64       `json:"timestamp"` // Unix 时间戳
	Hostname  string      `json:"hostname"`  // 主机名
	OS        string      `json:"os"`        // 操作系统
	Code      string      `json:"code"`      // 注册码（仅 register 消息）
	Data      *MetricData `json:"data"`      // 监控数据（仅 report 消息）
	PingData  []PingResult `json:"ping_data"` // Ping 结果（仅 ping_result 消息）
	Reason    string      `json:"reason"`    // 失败原因（仅 register_fail 消息）

	// config_update 消息字段
	PingTargets  []PingTarget `json:"ping_targets"`
	PingInterval int          `json:"ping_interval"`
}

// 消息类型常量 - Agent → Server
const (
	MsgTypeRegister   = "register"    // Agent 注册
	MsgTypeReport     = "report"      // 数据上报
	MsgTypePingResult = "ping_result" // Ping 探测结果
	MsgTypeHeartbeat  = "heartbeat"   // 心跳
)

// 消息类型常量 - Server → Agent（仅这 4 种，无控制指令）
const (
	MsgTypeRegisterOK    = "register_ok"    // 注册成功
	MsgTypeRegisterFail  = "register_fail"  // 注册失败
	MsgTypeConfigUpdate  = "config_update"  // 配置更新
	MsgTypeHeartbeatAck  = "heartbeat_ack"  // 心跳确认
)
