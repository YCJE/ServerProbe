package api

import (
	"encoding/json"
	"log"
	"net/http"
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
}

// NewAgentHandler 创建 Agent 处理器
func NewAgentHandler(
	registry *service.AgentRegistryService,
	monitor *service.MonitorService,
	configSync *service.ConfigSyncService,
	validator *service.DataValidator,
) *AgentHandler {
	return &AgentHandler{
		registry:   registry,
		monitor:    monitor,
		configSync: configSync,
		validator:  validator,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // 允许所有来源（生产环境应限制）
			},
		},
	}
}

// HandleWebSocket Agent WebSocket 接入端点
// 路由: WS /api/v1/agent/report
func (h *AgentHandler) HandleWebSocket(c *gin.Context) {
	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("WebSocket 升级失败: %v", err)
		return
	}

	var agentID int64
	var registered bool

	defer func() {
		if registered && agentID > 0 {
			h.monitor.UnregisterConnection(agentID)
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
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
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
			h.handleRegister(conn, &msg, &agentID, &registered)

		case sharedmodel.MsgTypeReport:
			h.handleReport(conn, &msg, agentID, registered)

		case sharedmodel.MsgTypePingResult:
			h.handlePingResult(conn, &msg, agentID, registered)

		case sharedmodel.MsgTypeHeartbeat:
			h.handleHeartbeat(conn, &msg, agentID, registered)

		default:
			log.Printf("未知消息类型: %s", msg.Type)
		}
	}
}

// handleRegister 处理注册消息
func (h *AgentHandler) handleRegister(conn *websocket.Conn, msg *sharedmodel.WSMessage, agentID *int64, registered *bool) {
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
		_ = conn.WriteJSON(response)
		return
	}

	*agentID = result.AgentID
	*registered = true

	// 注册连接
	h.monitor.RegisterConnection(result.AgentID, conn)

	// 发送注册成功响应
	response := sharedmodel.WSMessage{
		Type:  sharedmodel.MsgTypeRegisterOK,
		Token: result.Token,
	}
	_ = conn.WriteJSON(response)

	// 发送初始配置
	h.sendConfigUpdate(conn, result.AgentID)
}

// handleReport 处理数据上报
func (h *AgentHandler) handleReport(conn *websocket.Conn, msg *sharedmodel.WSMessage, agentID int64, registered bool) {
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
func (h *AgentHandler) handlePingResult(conn *websocket.Conn, msg *sharedmodel.WSMessage, agentID int64, registered bool) {
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
func (h *AgentHandler) handleHeartbeat(conn *websocket.Conn, msg *sharedmodel.WSMessage, agentID int64, registered bool) {
	if !registered || agentID == 0 {
		return
	}

	h.monitor.UpdateHeartbeat(agentID)

	// 发送心跳确认
	response := sharedmodel.WSMessage{
		Type: sharedmodel.MsgTypeHeartbeatAck,
	}
	_ = conn.WriteJSON(response)
}

// sendConfigUpdate 发送配置更新
func (h *AgentHandler) sendConfigUpdate(conn *websocket.Conn, agentID int64) {
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
	_ = conn.WriteJSON(response)
}

// HandleGetAgentConfig 处理 Agent 配置拉取
// 路由: GET /api/v1/agent/config
func (h *AgentHandler) HandleGetAgentConfig(c *gin.Context) {
	token := c.Query("token")
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
