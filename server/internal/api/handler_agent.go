package api

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/server-probe/server/internal/service"
	sharedmodel "github.com/server-probe/shared/model"
)

// AgentHandler Agent WebSocket 处理器
type AgentHandler struct {
	registry   *service.AgentRegistryService
	monitor    *service.MonitorService
	configSync *service.ConfigSyncService
	validator  *service.DataValidator
	upgrader   websocket.Upgrader
	wsConns    map[int64]*agentWSConn // Agent ID → WebSocket 连接
	wsConnsMu  sync.RWMutex
}

// NewAgentHandler 创建 Agent 处理器
func NewAgentHandler(
	registry *service.AgentRegistryService,
	monitor *service.MonitorService,
	configSync *service.ConfigSyncService,
	validator *service.DataValidator,
) *AgentHandler {
	h := &AgentHandler{
		registry:   registry,
		monitor:    monitor,
		configSync: configSync,
		validator:  validator,
		wsConns:    make(map[int64]*agentWSConn),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				// Agent 连接不需要 Origin 检查（非浏览器客户端）
				return true
			},
		},
	}

	// 注册配置推送回调，使用 agentWSConn.mu 锁保护写入
	monitor.SetConfigPushCallback(func(agentID int64, config *sharedmodel.AgentConfig) {
		h.wsConnsMu.RLock()
		ws, ok := h.wsConns[agentID]
		h.wsConnsMu.RUnlock()
		if !ok {
			return
		}

		msg := sharedmodel.WSMessage{
			Type:         sharedmodel.MsgTypeConfigUpdate,
			PingTargets:  config.PingTargets,
			PingInterval: config.PingInterval,
		}

		if err := ws.writeJSON(msg); err != nil {
			log.Printf("推送配置更新到 Agent %d 失败: %v", agentID, err)
		} else {
			log.Printf("已推送配置更新到 Agent %d (探测目标 %d 个, 间隔 %ds)",
				agentID, len(config.PingTargets), config.PingInterval)
		}
	})

	return h
}

// agentWSConn 封装 Agent WebSocket 连接，添加写锁
type agentWSConn struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (w *agentWSConn) writeMessage(messageType int, data []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.conn.WriteMessage(messageType, data)
}

func (w *agentWSConn) writeJSON(v interface{}) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.conn.WriteJSON(v)
}

// HandleWebSocket Agent WebSocket 接入端点
// 路由: WS /api/v1/agent/report
func (h *AgentHandler) HandleWebSocket(c *gin.Context) {
	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("WebSocket 升级失败: %v", err)
		return
	}

	ws := &agentWSConn{conn: conn}

	var agentID int64
	var registered bool

	defer func() {
		if registered && agentID > 0 {
			h.monitor.UnregisterConnection(agentID)
			h.wsConnsMu.Lock()
			delete(h.wsConns, agentID)
			h.wsConnsMu.Unlock()
		}
		conn.Close()
	}()

	// 设置读超时和写超时
	conn.SetReadDeadline(time.Now().Add(120 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(120 * time.Second))
		return nil
	})

	pingTicker := time.NewTicker(30 * time.Second)
	defer pingTicker.Stop()

	// 启动 ping 协程
	go func() {
		for range pingTicker.C {
			if err := ws.writeMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}()

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("WebSocket 读取错误: %v", err)
			}
			break
		}

		var msg sharedmodel.WSMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			log.Printf("消息解析失败: %v", err)
			continue
		}

		// 根据消息类型处理
		switch msg.Type {
		case sharedmodel.MsgTypeRegister:
			h.handleRegister(ws, &msg, &agentID, &registered)

		case sharedmodel.MsgTypeReport:
			h.handleReport(ws, &msg, agentID, registered)

		case sharedmodel.MsgTypePingResult:
			h.handlePingResult(ws, &msg, agentID, registered)

		case sharedmodel.MsgTypeHeartbeat:
			h.handleHeartbeat(ws, &msg, agentID, registered)

		default:
			log.Printf("未知消息类型: %s", msg.Type)
		}
	}
}

// handleRegister 处理注册消息
func (h *AgentHandler) handleRegister(ws *agentWSConn, msg *sharedmodel.WSMessage, agentID *int64, registered *bool) {
	req := service.RegisterAgentRequest{
		Code:            msg.Code,
		Hostname:        msg.Hostname,
		OS:              msg.OS,
		Arch:            msg.Arch,
		AgentVersion:    msg.AgentVersion,
		HostFingerprint: msg.HostFingerprint,
	}

	result, err := h.registry.RegisterAgent(req)
	if err != nil {
		log.Printf("Agent 注册失败: %v", err)
		response := sharedmodel.WSMessage{
			Type:   sharedmodel.MsgTypeRegisterFail,
			Reason: err.Error(),
		}
		_ = ws.writeJSON(response)
		return
	}

	*agentID = result.AgentID
	*registered = true

	// 注册连接
	h.monitor.RegisterConnection(result.AgentID, ws.conn)

	// 保存 wsConn 引用用于配置推送
	h.wsConnsMu.Lock()
	h.wsConns[result.AgentID] = ws
	h.wsConnsMu.Unlock()

	// 发送注册成功响应
	response := sharedmodel.WSMessage{
		Type:  sharedmodel.MsgTypeRegisterOK,
		Token: result.Token,
	}
	_ = ws.writeJSON(response)

	// 发送初始配置
	h.sendConfigUpdate(ws, result.AgentID)
}

// handleReport 处理数据上报
func (h *AgentHandler) handleReport(ws *agentWSConn, msg *sharedmodel.WSMessage, agentID int64, registered bool) {
	if !registered || agentID == 0 {
		return
	}

	// 验证 Token
	if msg.Token == "" {
		return
	}
	agent, err := h.registry.ValidateToken(msg.Token)
	if err != nil || agent.ID != agentID {
		return
	}

	// 校验数据
	if msg.Data == nil {
		return
	}

	if err := h.validator.ValidateMetricData(agentID, msg.Data); err != nil {
		log.Printf("Agent %d 数据校验失败: %v", agentID, err)
		return
	}

	if err := h.validator.CheckReportFrequency(agentID); err != nil {
		log.Printf("Agent %d 上报频率异常: %v", agentID, err)
		return
	}

	// 写入实时数据
	if err := h.monitor.WriteMetricData(agentID, msg.Data); err != nil {
		log.Printf("Agent %d 写入数据失败: %v", agentID, err)
		return
	}

	// 更新心跳
	h.monitor.UpdateHeartbeat(agentID)
}

// handlePingResult 处理 Ping 结果
func (h *AgentHandler) handlePingResult(ws *agentWSConn, msg *sharedmodel.WSMessage, agentID int64, registered bool) {
	if !registered || agentID == 0 {
		return
	}

	// 验证 Token
	agent, err := h.registry.ValidateToken(msg.Token)
	if err != nil || agent.ID != agentID {
		return
	}

	// 校验 Ping 数据
	for i := range msg.PingData {
		if err := h.validator.ValidatePingResult(&msg.PingData[i]); err != nil {
			log.Printf("Agent %d Ping 数据校验失败: %v", agentID, err)
			return
		}
	}

	// 写入 Ping 数据
	if err := h.monitor.WritePingData(agentID, msg.PingData); err != nil {
		log.Printf("Agent %d 写入 Ping 数据失败: %v", agentID, err)
		return
	}
}

// handleHeartbeat 处理心跳
func (h *AgentHandler) handleHeartbeat(ws *agentWSConn, msg *sharedmodel.WSMessage, agentID int64, registered bool) {
	if !registered || agentID == 0 {
		return
	}

	h.monitor.UpdateHeartbeat(agentID)

	// 发送心跳确认
	response := sharedmodel.WSMessage{
		Type: sharedmodel.MsgTypeHeartbeatAck,
	}
	_ = ws.writeJSON(response)
}

// sendConfigUpdate 发送配置更新
func (h *AgentHandler) sendConfigUpdate(ws *agentWSConn, agentID int64) {
	config, err := h.configSync.GetAgentConfig()
	if err != nil {
		log.Printf("获取 Agent %d 配置失败: %v", agentID, err)
		return
	}

	response := sharedmodel.WSMessage{
		Type:         sharedmodel.MsgTypeConfigUpdate,
		PingTargets:  config.PingTargets,
		PingInterval: config.PingInterval,
	}
	_ = ws.writeJSON(response)
}

// HandleGetAgentConfig 处理 Agent 配置拉取
// 路由: GET /api/v1/agent/config
func (h *AgentHandler) HandleGetAgentConfig(c *gin.Context) {
	// 优先从 Authorization header 获取 Token，兼容 query 参数
	token := ""
	authHeader := c.GetHeader("Authorization")
	if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
		token = authHeader[7:]
	}
	if token == "" {
		token = c.Query("token")
	}
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "缺少 Token"})
		return
	}

	agent, err := h.registry.ValidateToken(token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Token 无效"})
		return
	}

	config, err := h.configSync.GetAgentConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取配置失败"})
		return
	}

	_ = agent
	c.JSON(http.StatusOK, config)
}
