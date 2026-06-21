package reporter

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	sharedmodel "github.com/server-probe/shared/model"
)

// WSClient WebSocket 客户端
type WSClient struct {
	serverURL    string
	token        string
	registerCode string
	insecureTLS  bool

	conn        *websocket.Conn
	mu          sync.Mutex
	connected   bool
	reconnectCh chan struct{}

	// 回调函数
	onRegisterOK    func(token string)
	onConfigUpdate  func(config *sharedmodel.AgentConfig)
	onMessage       func(msg *sharedmodel.WSMessage)

	// 重连参数
	maxReconnectInterval time.Duration
}

// NewWSClient 创建 WebSocket 客户端
func NewWSClient(serverURL, token, registerCode string, insecureTLS bool) *WSClient {
	return &WSClient{
		serverURL:            serverURL,
		token:                token,
		registerCode:         registerCode,
		insecureTLS:          insecureTLS,
		reconnectCh:          make(chan struct{}, 1),
		maxReconnectInterval: 60 * time.Second,
	}
}

// SetCallbacks 设置回调函数
func (c *WSClient) SetCallbacks(
	onRegisterOK func(token string),
	onConfigUpdate func(config *sharedmodel.AgentConfig),
	onMessage func(msg *sharedmodel.WSMessage),
) {
	c.onRegisterOK = onRegisterOK
	c.onConfigUpdate = onConfigUpdate
	c.onMessage = onMessage
}

// Connect 连接 Server
func (c *WSClient) Connect() error {
	// 强制 TLS：拒绝明文连接
	if !strings.HasPrefix(c.serverURL, "https://") {
		return fmt.Errorf("安全错误：Server 地址必须使用 https://，拒绝明文连接")
	}

	// 转换为 WebSocket URL
	wsURL := strings.Replace(c.serverURL, "https://", "wss://", 1)
	wsURL += "/api/v1/agent/report"

	// 验证 URL
	parsed, err := url.Parse(wsURL)
	if err != nil {
		return fmt.Errorf("URL 解析失败: %w", err)
	}
	if parsed.Scheme != "wss" {
		return fmt.Errorf("安全错误：必须使用 wss:// 协议")
	}

	// 创建 WebSocket 拨号器
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}
	if c.insecureTLS {
		tlsConfig.InsecureSkipVerify = true
	}

	dialer := websocket.Dialer{
		TLSClientConfig:  tlsConfig,
		HandshakeTimeout: 10 * time.Second,
	}

	header := http.Header{}
	header.Set("User-Agent", "ServerProbe-Agent/1.0")

	conn, _, err := dialer.Dial(wsURL, header)
	if err != nil {
		return fmt.Errorf("WebSocket 连接失败: %w", err)
	}

	c.mu.Lock()
	c.conn = conn
	c.connected = true
	c.mu.Unlock()

	// 如果有注册码，先注册
	if c.registerCode != "" && c.token == "" {
		if err := c.register(); err != nil {
			c.mu.Lock()
			c.conn = nil
			c.connected = false
			c.mu.Unlock()
			conn.Close()
			return fmt.Errorf("注册失败: %w", err)
		}
	}

	return nil
}

// register 发送注册消息
func (c *WSClient) register() error {
	hostname, _ := getHostname()

	msg := sharedmodel.WSMessage{
		Type:            sharedmodel.MsgTypeRegister,
		Code:            c.registerCode,
		Hostname:        hostname,
		OS:              runtime.GOOS,
		Arch:            runtime.GOARCH,
		AgentVersion:    "1.0.0",
		HostFingerprint: getHostFingerprint(),
	}

	c.mu.Lock()
	err := c.conn.WriteJSON(msg)
	c.mu.Unlock()

	if err != nil {
		return fmt.Errorf("发送注册消息失败: %w", err)
	}

	// 等待注册响应
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()

	conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	_, message, err := conn.ReadMessage()
	if err != nil {
		return fmt.Errorf("读取注册响应失败: %w", err)
	}
	conn.SetReadDeadline(time.Time{})

	var response sharedmodel.WSMessage
	if err := json.Unmarshal(message, &response); err != nil {
		return fmt.Errorf("解析注册响应失败: %w", err)
	}

	switch response.Type {
	case sharedmodel.MsgTypeRegisterOK:
		c.token = response.Token
		if c.onRegisterOK != nil {
			c.onRegisterOK(response.Token)
		}
		log.Printf("注册成功，已获取 Token")
		return nil

	case sharedmodel.MsgTypeRegisterFail:
		return fmt.Errorf("注册被拒绝: %s", response.Reason)

	default:
		return fmt.Errorf("未预期的响应类型: %s", response.Type)
	}
}

// Run 运行消息循环
func (c *WSClient) Run() {
	for {
		c.mu.Lock()
		conn := c.conn
		c.mu.Unlock()

		if conn == nil {
			// 重连
			if err := c.reconnect(); err != nil {
				log.Printf("重连失败: %v", err)
				time.Sleep(c.getReconnectInterval())
				continue
			}
			continue
		}

		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Printf("WebSocket 读取错误: %v", err)
			c.mu.Lock()
			c.connected = false
			c.conn = nil
			c.mu.Unlock()
			conn.Close()
			continue
		}

		var msg sharedmodel.WSMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			log.Printf("消息解析失败: %v", err)
			continue
		}

		c.handleMessage(&msg)
	}
}

// handleMessage 处理收到的消息
func (c *WSClient) handleMessage(msg *sharedmodel.WSMessage) {
	switch msg.Type {
	case sharedmodel.MsgTypeConfigUpdate:
		config := &sharedmodel.AgentConfig{
			PingTargets:  msg.PingTargets,
			PingInterval: msg.PingInterval,
		}
		if c.onConfigUpdate != nil {
			c.onConfigUpdate(config)
		}

	case sharedmodel.MsgTypeHeartbeatAck:
		// 心跳确认，无需处理

	default:
		if c.onMessage != nil {
			c.onMessage(msg)
		}
	}
}

// SendMessage 发送消息
func (c *WSClient) SendMessage(msg *sharedmodel.WSMessage) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil || !c.connected {
		return fmt.Errorf("未连接")
	}

	msg.Token = c.token
	return c.conn.WriteJSON(msg)
}

// SendHeartbeat 发送心跳
func (c *WSClient) SendHeartbeat() error {
	msg := &sharedmodel.WSMessage{
		Type:      sharedmodel.MsgTypeHeartbeat,
		Timestamp: time.Now().Unix(),
	}
	return c.SendMessage(msg)
}

// SendReport 发送监控数据
func (c *WSClient) SendReport(data *sharedmodel.MetricData) error {
	msg := &sharedmodel.WSMessage{
		Type:      sharedmodel.MsgTypeReport,
		Timestamp: time.Now().Unix(),
		Data:      data,
	}
	return c.SendMessage(msg)
}

// SendPingResult 发送 Ping 结果
func (c *WSClient) SendPingResult(results []sharedmodel.PingResult) error {
	msg := &sharedmodel.WSMessage{
		Type:      sharedmodel.MsgTypePingResult,
		Timestamp: time.Now().Unix(),
		PingData:  results,
	}
	return c.SendMessage(msg)
}

// IsConnected 是否已连接
func (c *WSClient) IsConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connected
}

// reconnect 重连
func (c *WSClient) reconnect() error {
	interval := c.getReconnectInterval()
	log.Printf("等待 %v 后重连...", interval)
	time.Sleep(interval)

	return c.Connect()
}

// getReconnectInterval 获取重连间隔（指数退避）
func (c *WSClient) getReconnectInterval() time.Duration {
	// 简化版：固定 5 秒递增到 60 秒
	// 实际实现应记录重连次数
	return 5 * time.Second
}

// getHostname 获取主机名
func getHostname() (string, error) {
	return executeHostname()
}

// getOS 获取操作系统
func getOS() string {
	return executeOS()
}
