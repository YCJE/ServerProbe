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
	w.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return w.conn.WriteMessage(messageType, data)
}

func (w *agentWSConn) writeJSON(v interface{}) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
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

	// 使用 done channel 通知 ping 协程退出，避免 goroutine 泄漏
	done := make(chan struct{})

	defer func() {
		close(done) // 通知 ping 协程退出
		if registered && agentID > 0 {
			// 条件注销: 仅当注册的连接仍是自己时才注销
			// 防止旧连接的 defer 关闭新连接
			h.monitor.UnregisterConnectionIfMatch(agentID, conn)
			h.wsConnsMu.Lock()
			if existing, ok := h.wsConns[agentID]; ok && existing == ws {
				delete(h.wsConns, agentID)
			}
			h.wsConnsMu.Unlock()
		}
		conn.Close()
	}()

	// 设置读超时和写超时
	conn.SetReadLimit(1024 * 1024) // 1MB 读取限制，防止 OOM
	conn.SetReadDeadline(time.Now().Add(120 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(120 * time.Second))
		return nil
	})

	pingTicker := time.NewTicker(30 * time.Second)
	defer pingTicker.Stop()

	// 启动 ping 协程 (使用 select + done channel 避免泄漏)
	go func() {
		for {
			select {
			case <-pingTicker.C:
				if err := ws.writeMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			case <-done:
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
			h.handleReport(ws, &msg, &agentID, &registered)

		case sharedmodel.MsgTypePingResult:
			h.handlePingResult(ws, &msg, &agentID, &registered)

		case sharedmodel.MsgTypeHeartbeat:
			h.handleHeartbeat(ws, &msg, &agentID, &registered)

		default:
			log.Printf("未知消息类型: %s", msg.Type)
		}
	}
}

// handleRegister 处理注册消息
// 两种场景:
//   1. 新 Agent 注册: 消息携带 Code (注册码)，无 Token
//   2. 已有 Agent 会话恢复: 消息携带 Token，无 Code (Server 重启后 Agent 重连)
func (h *AgentHandler) handleRegister(ws *agentWSConn, msg *sharedmodel.WSMessage, agentID *int64, registered *bool) {
	// 场景 2: Token-based 会话恢复（Agent 重连）
	if msg.Token != "" {
		agent, err := h.registry.ValidateToken(msg.Token)
		if err != nil {
			log.Printf("Agent 会话恢复失败，Token 无效: %v", err)
			response := sharedmodel.WSMessage{
				Type:   sharedmodel.MsgTypeRegisterFail,
				Reason: "Token 无效，请重新注册",
			}
			_ = ws.writeJSON(response)
			return
		}

		// 校验主机指纹
		if agent.HostFingerprint != "" {
			if msg.HostFingerprint == "" || agent.HostFingerprint != msg.HostFingerprint {
				log.Printf("Agent %d 会话恢复指纹不匹配", agent.ID)
				response := sharedmodel.WSMessage{
					Type:   sharedmodel.MsgTypeRegisterFail,
					Reason: "主机指纹不匹配",
				}
				_ = ws.writeJSON(response)
				return
			}
		}

		*agentID = agent.ID
		*registered = true

		// 注册连接
		h.monitor.RegisterConnection(agent.ID, ws.conn)

		// 保存 wsConn 引用用于配置推送
		h.wsConnsMu.Lock()
		h.wsConns[agent.ID] = ws
		h.wsConnsMu.Unlock()

		// 发送注册成功响应（回显 Token）
		response := sharedmodel.WSMessage{
			Type:  sharedmodel.MsgTypeRegisterOK,
			Token: msg.Token,
		}
		_ = ws.writeJSON(response)

		// 发送初始配置
		h.sendConfigUpdate(ws, agent.ID)

		log.Printf("Agent %d (%s) 会话恢复成功", agent.ID, agent.Hostname)
		return
	}

	// 场景 1: 注册码注册新 Agent
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

	log.Printf("Agent %d (%s) 注册成功", result.AgentID, req.Hostname)
}

// handleReport 处理数据上报
func (h *AgentHandler) handleReport(ws *agentWSConn, msg *sharedmodel.WSMessage, agentID *int64, registered *bool) {
	if !*registered || *agentID == 0 {
		// 向后兼容: 旧版 Agent 重连后不发送 register，直接上报数据
		// 如果携带有效 Token，懒注册会话
		if msg.Token == "" || !h.lazyRegister(ws, msg, agentID, registered) {
			return
		}
	}

	// 验证 Token
	if msg.Token == "" {
		return
	}
	agent, err := h.registry.ValidateToken(msg.Token)
	if err != nil || agent.ID != *agentID {
		return
	}

	// 校验主机指纹 (存储的指纹非空时，消息指纹必须匹配)
	if agent.HostFingerprint != "" {
		if msg.HostFingerprint == "" || agent.HostFingerprint != msg.HostFingerprint {
			log.Printf("Agent %d 主机指纹不匹配或缺失，拒绝上报", *agentID)
			return
		}
	}

	// 校验数据
	if msg.Data == nil {
		return
	}

	// 校验数据大小 (≤10KB)
	if rawData, err := json.Marshal(msg.Data); err == nil {
		if err := h.validator.CheckDataSize(rawData); err != nil {
			log.Printf("Agent %d 数据大小超限: %v", *agentID, err)
			return
		}
	} else {
		log.Printf("Agent %d 数据序列化失败，拒绝上报", *agentID)
		return
	}

	if err := h.validator.ValidateMetricData(*agentID, msg.Data); err != nil {
		log.Printf("Agent %d 数据校验失败: %v", *agentID, err)
		return
	}

	if err := h.validator.CheckReportFrequency(*agentID); err != nil {
		log.Printf("Agent %d 上报频率异常: %v", *agentID, err)
		return
	}

	// 写入实时数据
	if err := h.monitor.WriteMetricData(*agentID, msg.Data); err != nil {
		log.Printf("Agent %d 写入数据失败: %v", *agentID, err)
		return
	}

	// 更新心跳
	h.monitor.UpdateHeartbeat(*agentID)
}

// handlePingResult 处理 Ping 结果
func (h *AgentHandler) handlePingResult(ws *agentWSConn, msg *sharedmodel.WSMessage, agentID *int64, registered *bool) {
	if !*registered || *agentID == 0 {
		if msg.Token == "" || !h.lazyRegister(ws, msg, agentID, registered) {
			return
		}
	}

	// 验证 Token
	agent, err := h.registry.ValidateToken(msg.Token)
	if err != nil || agent.ID != *agentID {
		return
	}

	// 校验主机指纹 (与其他 handler 保持一致)
	if agent.HostFingerprint != "" {
		if msg.HostFingerprint == "" || agent.HostFingerprint != msg.HostFingerprint {
			log.Printf("Agent %d Ping 结果指纹不匹配或缺失，拒绝", *agentID)
			return
		}
	}

	// 校验 Ping 数据
	for i := range msg.PingData {
		if err := h.validator.ValidatePingResult(&msg.PingData[i]); err != nil {
			log.Printf("Agent %d Ping 数据校验失败: %v", *agentID, err)
			return
		}
	}

	// 写入 Ping 数据
	if err := h.monitor.WritePingData(*agentID, msg.PingData); err != nil {
		log.Printf("Agent %d 写入 Ping 数据失败: %v", *agentID, err)
		return
	}
}

// handleHeartbeat 处理心跳
func (h *AgentHandler) handleHeartbeat(ws *agentWSConn, msg *sharedmodel.WSMessage, agentID *int64, registered *bool) {
	if !*registered || *agentID == 0 {
		// 向后兼容: 旧版 Agent 重连后不发送 register，直接发心跳
		if msg.Token == "" || !h.lazyRegister(ws, msg, agentID, registered) {
			return
		}
	}

	// 校验 Token (心跳必须携带 Token)
	if msg.Token == "" {
		return
	}
	agent, err := h.registry.ValidateToken(msg.Token)
	if err != nil || agent.ID != *agentID {
		return
	}
	// 校验主机指纹 (存储的指纹非空时，消息指纹必须匹配)
	if agent.HostFingerprint != "" {
		if msg.HostFingerprint == "" || agent.HostFingerprint != msg.HostFingerprint {
			log.Printf("Agent %d 心跳指纹不匹配或缺失，拒绝", *agentID)
			return
		}
	}

	h.monitor.UpdateHeartbeat(*agentID)

	// 发送心跳确认
	response := sharedmodel.WSMessage{
		Type: sharedmodel.MsgTypeHeartbeatAck,
	}
	_ = ws.writeJSON(response)
}

// lazyRegister 懒注册会话（向后兼容旧版 Agent）
// 当 Agent 重连后未发送 register 消息而直接上报数据时，
// 通过 Token 验证身份并建立会话
func (h *AgentHandler) lazyRegister(ws *agentWSConn, msg *sharedmodel.WSMessage, agentID *int64, registered *bool) bool {
	agent, err := h.registry.ValidateToken(msg.Token)
	if err != nil {
		return false
	}

	// 校验主机指纹
	if agent.HostFingerprint != "" {
		if msg.HostFingerprint == "" || agent.HostFingerprint != msg.HostFingerprint {
			log.Printf("Agent %d 懒注册指纹不匹配", agent.ID)
			return false
		}
	}

	*agentID = agent.ID
	*registered = true

	// 注册连接
	h.monitor.RegisterConnection(agent.ID, ws.conn)

	// 保存 wsConn 引用用于配置推送
	h.wsConnsMu.Lock()
	h.wsConns[agent.ID] = ws
	h.wsConnsMu.Unlock()

	// 发送初始配置 (与正常注册流程一致)
	h.sendConfigUpdate(ws, agent.ID)

	log.Printf("Agent %d (%s) 懒注册成功（向后兼容模式）", agent.ID, agent.Hostname)
	return true
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
