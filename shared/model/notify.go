package model

import "time"

// NotifyChannel 通知渠道
type NotifyChannel struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`   // webhook / telegram / email
	Config    string    `json:"config"` // JSON 格式的渠道配置
	CreatedAt time.Time `json:"created_at"`
}

// 通知渠道类型
const (
	NotifyTypeWebhook   = "webhook"
	NotifyTypeTelegram  = "telegram"
	NotifyTypeEmail     = "email"
)

// WebhookConfig Webhook 通知配置
type WebhookConfig struct {
	URL     string `json:"url"`
	Secret  string `json:"secret"` // 可选的签名密钥
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

// NotifyMessage 通知消息
type NotifyMessage struct {
	Title   string `json:"title"`
	Content string `json:"content"`
	Level   string `json:"level"` // info / warning / critical
}
