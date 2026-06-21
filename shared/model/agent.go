package model

import "time"

// Agent 表示已注册的 Agent 元数据
type Agent struct {
	ID               int64     `json:"id"`
	Token            string    `json:"-"`              // 不对外暴露
	Hostname         string    `json:"hostname"`
	OS               string    `json:"os"`
	Arch             string    `json:"arch"`
	AgentVersion     string    `json:"agent_version"`
	HostFingerprint  string    `json:"-"`              // 不对外暴露
	LastSeen         time.Time `json:"last_seen"`
	Online           bool      `json:"online"`
	CreatedAt        time.Time `json:"created_at"`
}

// RegisterCode 注册码
type RegisterCode struct {
	Code          string    `json:"code"`
	CreatedAt     time.Time `json:"created_at"`
	ExpiresAt     time.Time `json:"expires_at"`
	Used          bool      `json:"used"`
	UsedByAgentID int64     `json:"used_by_agent_id"`
}

// RegisterRequest Agent 注册请求
type RegisterRequest struct {
	Code             string `json:"code"`
	Hostname         string `json:"hostname"`
	OS               string `json:"os"`
	Arch             string `json:"arch"`
	AgentVersion     string `json:"agent_version"`
	HostFingerprint  string `json:"host_fingerprint"`
}

// RegisterResponse Server 注册响应
type RegisterResponse struct {
	Success bool   `json:"success"`
	Token   string `json:"token"`
	Reason  string `json:"reason"`
}
