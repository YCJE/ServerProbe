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
	fingerprint  string // 主机指纹 (缓存)

	conn      *websocket.Conn
	mu        sync.Mutex    // 连接状态锁（保护 conn/connected/token 等状态字段）
	writeMu   sync.Mutex    // 写入锁，仅保护 WriteJSON，避免写超时阻塞 Stop()
	connected bool
	stopCh    chan struct{} // 通知 Run() 退出
	stopOnce  sync.Once     // 保证 stopCh 只被关闭一次，避免并发 Stop panic

	// 回调函数
	onRegisterOK   func(token string)
	onConfigUpdate func(config *sharedmodel.AgentConfig)
	onMessage      func(msg *sharedmodel.WSMessage)

	// 重连参数
	reconnectAttempts    int
	maxReconnectInterval time.Duration
}

// NewWSClient 创建 WebSocket 客户端
func NewWSClient(serverURL, token, registerCode string, insecureTLS bool) *WSClient {
	return &WSClient{
		// 规范化 serverURL，去除尾斜杠以避免拼接出双斜杠 URL（如 https://host//api/...）
		serverURL:            strings.TrimRight(serverURL, "/"),
		token:                token,
		registerCode:         registerCode,
		insecureTLS:          insecureTLS,
		fingerprint:          getHostFingerprint(),
		maxReconnectInterval: 60 * time.Second,
		stopCh:               make(chan struct{}),
	}
}

// Stop 停止 WebSocket 客户端，优雅关闭连接
func (c *WSClient) Stop() {
	// 使用 sync.Once 保证 stopCh 只被关闭一次，避免 check-then-close 反模式导致的并发 panic
	c.stopOnce.Do(func() { close(c.stopCh) })
	// 仅持有连接状态锁来关闭 conn，不持有 writeMu：
	// 这样正在进行的 WriteJSON 会被 conn.Close() 中断，而不会阻塞 Stop() 最长 10s
	c.mu.Lock()
	c.connected = false
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
	c.mu.Unlock()
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
	// 检查是否已存在连接
	c.mu.Lock()
	if c.conn != nil {
		c.mu.Unlock()
		return fmt.Errorf("已存在连接")
	}
	c.mu.Unlock()

	// 强制 TLS：拒绝明文连接（大小写不敏感）
	lowerURL := strings.ToLower(c.serverURL)
	if !strings.HasPrefix(lowerURL, "https://") {
		return fmt.Errorf("安全错误：Server 地址必须使用 https://，拒绝明文连接")
	}

	// 转换为 WebSocket URL（大小写不敏感地替换 scheme 前缀）
	wsURL := "wss://" + c.serverURL[len("https://"):] + "/api/v1/agent/report"

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

	// 设置读取大小限制，防止恶意 Server 发送超大消息导致内存耗尽
	conn.SetReadLimit(1024 * 1024) // 1MB

	// dial 成功后、设置 c.conn 之前检查 stopCh：拨号最长 10s，期间若 Stop() 已被调用，
	// 立即关闭新连接并返回错误，避免连接泄漏（连接被建立却永不关闭）
	select {
	case <-c.stopCh:
		conn.Close()
		return fmt.Errorf("客户端已停止，丢弃新建立的连接")
	default:
	}

	c.mu.Lock()
	c.conn = conn
	c.reconnectAttempts = 0
	c.mu.Unlock()

	// 连接建立后，必须进行会话初始化（认证完成前不标记 connected，
	// 避免心跳/上报 goroutine 在会话恢复期间写入数据）
	if c.token != "" {
		// 已有 Token，发送会话恢复请求
		if err := c.resumeSession(); err != nil {
			c.mu.Lock()
			c.conn = nil
			c.mu.Unlock()
			conn.Close()
			return fmt.Errorf("会话恢复失败: %w", err)
		}
	} else if c.registerCode != "" {
		// 有注册码，发送注册请求
		if err := c.register(); err != nil {
			c.mu.Lock()
			c.conn = nil
			c.mu.Unlock()
			conn.Close()
			return fmt.Errorf("注册失败: %w", err)
		}
	} else {
		// Token 和注册码都为空，无法建立认证连接
		c.mu.Lock()
		c.conn = nil
		c.mu.Unlock()
		conn.Close()
		return fmt.Errorf("安全错误：缺少 Token 和注册码，无法建立认证连接")
	}

	// 认证（resumeSession/register）成功后、标记 connected=true 之前再次检查 stopCh：
	// 认证最长 30s，期间若 Stop() 被调用，需关闭已认证连接避免泄漏
	select {
	case <-c.stopCh:
		c.mu.Lock()
		c.conn = nil
		c.mu.Unlock()
		conn.Close()
		return fmt.Errorf("客户端已停止，丢弃已完成认证的连接")
	default:
	}

	// 会话初始化成功后才标记为已连接
	c.mu.Lock()
	c.connected = true
	c.mu.Unlock()

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
		HostFingerprint: c.fingerprint,
	}

	c.mu.Lock()
	if c.conn == nil {
		c.mu.Unlock()
		return fmt.Errorf("连接已关闭")
	}
	conn := c.conn
	c.mu.Unlock()

	// 写入操作使用独立的 writeMu，与连接状态锁分离，
	// 避免 WriteJSON 写超时（最长 10s）期间阻塞 Stop() 和其他发送方
	c.writeMu.Lock()
	conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	err := conn.WriteJSON(msg)
	c.writeMu.Unlock()

	if err != nil {
		return fmt.Errorf("发送注册消息失败: %w", err)
	}

	// 等待注册响应（gorilla/websocket 允许并发读/写，读取无需 writeMu）
	conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	_, message, err := conn.ReadMessage()
	if err != nil {
		return fmt.Errorf("读取注册响应失败: %w", err)
	}

	var response sharedmodel.WSMessage
	if err := json.Unmarshal(message, &response); err != nil {
		return fmt.Errorf("解析注册响应失败: %w", err)
	}

	switch response.Type {
	case sharedmodel.MsgTypeRegisterOK:
		c.mu.Lock()
		c.token = response.Token
		c.mu.Unlock()
		if c.onRegisterOK != nil {
			c.onRegisterOK(response.Token)
		}
		log.Printf("注册成功，已获取 Token")
		return nil

	case sharedmodel.MsgTypeRegisterFail:
		c.mu.Lock()
		c.registerCode = "" // 仅在收到 register_fail 时清除注册码，避免无效重试
		c.mu.Unlock()
		return fmt.Errorf("注册被拒绝: %s", response.Reason)

	default:
		return fmt.Errorf("未预期的响应类型: %s", response.Type)
	}
}

// resumeSession 恢复会话（使用已有 Token 重新建立 WebSocket 会话）
// Server 重启后 Agent 重连时调用，避免数据被静默丢弃
func (c *WSClient) resumeSession() error {
	hostname, _ := getHostname()

	msg := sharedmodel.WSMessage{
		Type:            sharedmodel.MsgTypeRegister,
		Token:           c.token, // 携带 Token 表示会话恢复
		Hostname:        hostname,
		OS:              runtime.GOOS,
		Arch:            runtime.GOARCH,
		AgentVersion:    "1.0.0",
		HostFingerprint: c.fingerprint,
	}

	c.mu.Lock()
	if c.conn == nil {
		c.mu.Unlock()
		return fmt.Errorf("连接已关闭")
	}
	conn := c.conn
	c.mu.Unlock()

	// 写入操作使用独立的 writeMu，与连接状态锁分离，
	// 避免 WriteJSON 写超时（最长 10s）期间阻塞 Stop() 和其他发送方
	c.writeMu.Lock()
	conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	err := conn.WriteJSON(msg)
	c.writeMu.Unlock()

	if err != nil {
		return fmt.Errorf("发送会话恢复消息失败: %w", err)
	}

	// 等待响应（gorilla/websocket 允许并发读/写，读取无需 writeMu）
	conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	_, message, err := conn.ReadMessage()
	if err != nil {
		return fmt.Errorf("读取会话恢复响应失败: %w", err)
	}

	var response sharedmodel.WSMessage
	if err := json.Unmarshal(message, &response); err != nil {
		return fmt.Errorf("解析会话恢复响应失败: %w", err)
	}

	switch response.Type {
	case sharedmodel.MsgTypeRegisterOK:
		log.Printf("会话恢复成功")
		return nil

	case sharedmodel.MsgTypeRegisterFail:
		// 保留 Token，仅记录失败，返回 error 让 Run() 重试，避免永久无法重连
		log.Printf("会话恢复被拒绝: %s", response.Reason)
		return fmt.Errorf("会话恢复被拒绝: %s", response.Reason)

	default:
		return fmt.Errorf("未预期的响应类型: %s", response.Type)
	}
}

// Run 运行消息循环
func (c *WSClient) Run() {
	for {
		select {
		case <-c.stopCh:
			log.Printf("WebSocket 客户端已停止")
			return
		default:
		}

		c.mu.Lock()
		conn := c.conn
		c.mu.Unlock()

		if conn == nil {
			// 重连前再次检查 stopCh，避免 Stop() 后仍触发重连拨号
			select {
			case <-c.stopCh:
				log.Printf("WebSocket 客户端已停止")
				return
			default:
			}
			// 重连
			if err := c.reconnect(); err != nil {
				log.Printf("重连失败: %v", err)
				select {
				case <-c.stopCh:
					return
				case <-time.After(c.getReconnectInterval()):
				}
				continue
			}
			continue
		}

		// 设置读取超时和 pong handler
		conn.SetReadDeadline(time.Now().Add(90 * time.Second))
		conn.SetPongHandler(func(string) error {
			conn.SetReadDeadline(time.Now().Add(90 * time.Second))
			return nil
		})

		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Printf("WebSocket 读取错误: %v", err)
			// 仅当 c.conn 仍为当前连接（未被 Stop 关闭）时才关闭，避免 double close
			c.mu.Lock()
			c.connected = false
			needClose := c.conn == conn
			if needClose {
				c.conn = nil
			}
			c.mu.Unlock()
			if needClose {
				conn.Close()
			}
			continue
		}
		// 读到消息后重置读取超时，避免消息处理时间影响下一次读取
		conn.SetReadDeadline(time.Time{})

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
	// 仅短暂持有连接状态锁获取 conn 引用与状态，避免 WriteJSON 期间长时间阻塞 Stop()
	c.mu.Lock()
	conn := c.conn
	connected := c.connected
	token := c.token
	c.mu.Unlock()

	if conn == nil || !connected {
		return fmt.Errorf("未连接")
	}

	msg.Token = token

	// 写入操作由 writeMu 保护，与连接状态锁分离；
	// Stop() 可同时关闭 conn 中断阻塞中的写操作，从而不会等待写超时
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	defer conn.SetWriteDeadline(time.Time{})
	return conn.WriteJSON(msg)
}

// SendHeartbeat 发送心跳
func (c *WSClient) SendHeartbeat() error {
	msg := &sharedmodel.WSMessage{
		Type:            sharedmodel.MsgTypeHeartbeat,
		Timestamp:       time.Now().Unix(),
		HostFingerprint: c.fingerprint,
	}
	return c.SendMessage(msg)
}

// SendReport 发送监控数据
func (c *WSClient) SendReport(data *sharedmodel.MetricData) error {
	msg := &sharedmodel.WSMessage{
		Type:            sharedmodel.MsgTypeReport,
		Timestamp:       time.Now().Unix(),
		Data:            data,
		HostFingerprint: c.fingerprint,
	}
	return c.SendMessage(msg)
}

// SendPingResult 发送 Ping 结果
func (c *WSClient) SendPingResult(results []sharedmodel.PingResult) error {
	msg := &sharedmodel.WSMessage{
		Type:            sharedmodel.MsgTypePingResult,
		Timestamp:       time.Now().Unix(),
		PingData:        results,
		HostFingerprint: c.fingerprint,
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
	return c.Connect()
}

// getReconnectInterval 获取重连间隔（指数退避）
func (c *WSClient) getReconnectInterval() time.Duration {
	c.mu.Lock()
	c.reconnectAttempts++
	attempts := c.reconnectAttempts
	c.mu.Unlock()

	// 限制移位次数，防止整数溢出导致 time.Sleep 收到负值而 CPU 空转
	if attempts > 10 {
		attempts = 10
	}

	interval := time.Duration(5*(1<<(attempts-1))) * time.Second
	if interval > c.maxReconnectInterval {
		interval = c.maxReconnectInterval
	}
	return interval
}

// getHostname 获取主机名
func getHostname() (string, error) {
	return executeHostname()
}
