package service

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/smtp"
	"strings"
	"time"

	"github.com/server-probe/server/internal/model"
	"github.com/server-probe/server/internal/pkg"
	"github.com/server-probe/server/internal/repository"
)

// NotifyService 通知发送服务
type NotifyService struct {
	notifyRepo *repository.NotifyRepository
	ssrf       *pkg.SSRFProtector
}

// NewNotifyService 创建通知服务
func NewNotifyService(notifyRepo *repository.NotifyRepository, ssrf *pkg.SSRFProtector) *NotifyService {
	return &NotifyService{
		notifyRepo: notifyRepo,
		ssrf:       ssrf,
	}
}

// SendNotification 发送通知
func (s *NotifyService) SendNotification(channelID int64, title, content string) error {
	channel, err := s.notifyRepo.GetByID(channelID)
	if err != nil {
		return fmt.Errorf("获取通知渠道失败: %w", err)
	}

	switch channel.Type {
	case model.NotifyTypeWebhook:
		return s.sendWebhook(channel, title, content)
	case model.NotifyTypeTelegram:
		return s.sendTelegram(channel, title, content)
	case model.NotifyTypeEmail:
		return s.sendEmail(channel, title, content)
	default:
		return fmt.Errorf("未知通知渠道类型: %s", channel.Type)
	}
}

// sendWebhook 发送 Webhook 通知（带 SSRF 防护）
func (s *NotifyService) sendWebhook(channel *model.NotifyChannel, title, content string) error {
	var config model.WebhookConfig
	if err := json.Unmarshal([]byte(channel.Config), &config); err != nil {
		return fmt.Errorf("解析 Webhook 配置失败: %w", err)
	}

	payload := map[string]string{
		"title":   title,
		"content": content,
		"level":   "warning",
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("序列化通知内容失败: %w", err)
	}

	// 通过 SSRF 防护器发送
	statusCode, respBody, err := s.ssrf.SendWebhook(config.URL, body)
	if err != nil {
		return fmt.Errorf("Webhook 发送失败: %w", err)
	}

	if statusCode >= 400 {
		return fmt.Errorf("Webhook 返回错误状态码: %d, 响应: %s", statusCode, string(respBody))
	}

	log.Printf("Webhook 通知发送成功: %s", config.URL)
	return nil
}

// sendTelegram 发送 Telegram 通知
func (s *NotifyService) sendTelegram(channel *model.NotifyChannel, title, content string) error {
	var config model.TelegramConfig
	if err := json.Unmarshal([]byte(channel.Config), &config); err != nil {
		return fmt.Errorf("解析 Telegram 配置失败: %w", err)
	}

	// Telegram Bot API URL
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", config.BotToken)

	payload := map[string]interface{}{
		"chat_id": config.ChatID,
		"text":    fmt.Sprintf("*%s*\n%s", title, content),
		"parse_mode": "Markdown",
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("序列化通知内容失败: %w", err)
	}

	// Telegram API 是公网地址，通过 SSRF 防护器发送
	statusCode, respBody, err := s.ssrf.SendWebhook(apiURL, body)
	if err != nil {
		return fmt.Errorf("Telegram 发送失败: %w", err)
	}

	if statusCode >= 400 {
		return fmt.Errorf("Telegram API 返回错误状态码: %d, 响应: %s", statusCode, string(respBody))
	}

	log.Printf("Telegram 通知发送成功")
	return nil
}

// sendEmail 发送邮件通知
func (s *NotifyService) sendEmail(channel *model.NotifyChannel, title, content string) error {
	var config model.EmailConfig
	if err := json.Unmarshal([]byte(channel.Config), &config); err != nil {
		return fmt.Errorf("解析邮件配置失败: %w", err)
	}

	// 构建邮件内容
	subject := fmt.Sprintf("Subject: %s\r\n", title)
	contentType := "Content-Type: text/plain; charset=UTF-8\r\n"
	body := fmt.Sprintf("%s%s\r\n\r\n%s", subject, contentType, content)

	// SMTP 认证
	auth := smtp.PlainAuth("", config.Username, config.Password, config.SMTPHost)

	addr := fmt.Sprintf("%s:%d", config.SMTPHost, config.SMTPPort)

	// UseTLS 为 true 时使用隐式 TLS (SMTPS, 通常端口 465)
	// UseTLS 为 false 时使用标准 SMTP (支持 STARTTLS, 通常端口 25/587)
	if config.UseTLS {
		return s.sendEmailWithTLS(addr, auth, config, body)
	}

	// 标准模式: smtp.SendMail 内部会尝试 STARTTLS
	err := smtp.SendMail(addr, auth, config.From, []string{config.To}, []byte(body))
	if err != nil {
		return fmt.Errorf("邮件发送失败: %w", err)
	}

	log.Printf("邮件通知发送成功: %s -> %s", config.From, config.To)
	return nil
}

// sendEmailWithTLS 使用隐式 TLS (SMTPS) 发送邮件
func (s *NotifyService) sendEmailWithTLS(addr string, auth smtp.Auth, config model.EmailConfig, body string) error {
	// 建立 TLS 连接
	conn, err := tls.DialWithDialer(
		&net.Dialer{Timeout: 10 * time.Second},
		"tcp",
		addr,
		&tls.Config{ServerName: config.SMTPHost},
	)
	if err != nil {
		return fmt.Errorf("TLS 连接 SMTP 服务器失败: %w", err)
	}
	defer conn.Close()

	// 创建 SMTP 客户端
	client, err := smtp.NewClient(conn, config.SMTPHost)
	if err != nil {
		return fmt.Errorf("创建 SMTP 客户端失败: %w", err)
	}
	defer client.Close()

	// 认证
	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("SMTP 认证失败: %w", err)
	}

	// 设置发件人
	if err := client.Mail(config.From); err != nil {
		return fmt.Errorf("设置发件人失败: %w", err)
	}

	// 设置收件人
	if err := client.Rcpt(config.To); err != nil {
		return fmt.Errorf("设置收件人失败: %w", err)
	}

	// 发送邮件内容
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("打开数据通道失败: %w", err)
	}
	if _, err := w.Write([]byte(body)); err != nil {
		return fmt.Errorf("写入邮件内容失败: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("关闭数据通道失败: %w", err)
	}

	// 退出
	client.Quit()

	log.Printf("邮件通知发送成功 (TLS): %s -> %s", config.From, config.To)
	return nil
}

// TestChannel 测试通知渠道
func (s *NotifyService) TestChannel(channelID int64) error {
	return s.SendNotification(channelID, "测试通知", "这是一条来自服务器探针的测试通知")
}

// ValidateChannelConfig 验证渠道配置
func (s *NotifyService) ValidateChannelConfig(channelType, config string) error {
	switch channelType {
	case model.NotifyTypeWebhook:
		var cfg model.WebhookConfig
		if err := json.Unmarshal([]byte(config), &cfg); err != nil {
			return fmt.Errorf("配置格式错误: %w", err)
		}
		if cfg.URL == "" {
			return fmt.Errorf("URL 不能为空")
		}
		if !strings.HasPrefix(cfg.URL, "https://") && !strings.HasPrefix(cfg.URL, "http://") {
			return fmt.Errorf("URL 必须以 http:// 或 https:// 开头")
		}
		// SSRF 检查
		if err := pkg.CheckURL(cfg.URL); err != nil {
			return fmt.Errorf("URL 安全检查失败: %w", err)
		}

	case model.NotifyTypeTelegram:
		var cfg model.TelegramConfig
		if err := json.Unmarshal([]byte(config), &cfg); err != nil {
			return fmt.Errorf("配置格式错误: %w", err)
		}
		if cfg.BotToken == "" || cfg.ChatID == "" {
			return fmt.Errorf("BotToken 和 ChatID 不能为空")
		}

	case model.NotifyTypeEmail:
		var cfg model.EmailConfig
		if err := json.Unmarshal([]byte(config), &cfg); err != nil {
			return fmt.Errorf("配置格式错误: %w", err)
		}
		if cfg.SMTPHost == "" || cfg.From == "" || cfg.To == "" {
			return fmt.Errorf("SMTPHost、From、To 不能为空")
		}

	default:
		return fmt.Errorf("未知渠道类型: %s", channelType)
	}

	return nil
}
