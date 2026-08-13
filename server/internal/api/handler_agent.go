package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/server-probe/server/internal/service"
	sharedmodel "github.com/server-probe/shared/model"
)

// 全局 Agent WebSocket 连接计数器，防止连接数过多导致 DoS
var agentWSConnCount atomic.Int32

const maxAgentWSConnections = 500

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
			Type:            sharedmodel.MsgTypeConfigUpdate,
			PingTargets:     config.PingTargets,
			PingInterval:    config.PingInterval,
			ReportInterval:  config.ReportInterval,
		}

		if err := ws.writeJSON(msg); err != nil {
			log.Printf("推送配置更新到 Agent %d 失败: %v", agentID, err)
		} else {
			log.Printf("已推送配置更新到 Agent %d (探测目标 %d 个, 间隔 %ds, 上报间隔 %ds)",
				agentID, len(config.PingTargets), config.PingInterval, config.ReportInterval)
		}
	})

	return h
}

// agentWSConn 封装 Agent WebSocket 连接，添加写锁
type agentWSConn struct {
	conn             *websocket.Conn
	mu               sync.Mutex
	lastHeartbeat    time.Time
	lastLazyRegister time.Time // 限制 lazyRegister 调用频率，防止未注册连接 DoS
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
	// 连接数限制，防止 DoS（先递增再检查，避免 TOCTOU 竞态）
	if agentWSConnCount.Add(1) > maxAgentWSConnections {
		agentWSConnCount.Add(-1)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "服务器连接数已满"})
		return
	}

	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		agentWSConnCount.Add(-1) // Upgrade 失败，回退计数
		log.Printf("WebSocket 升级失败: %v", err)
		return
	}

	ws := &agentWSConn{conn: conn}

	// 使用 atomic 类型防止 ping 协程与主读循环之间的数据竞争
	var agentID atomic.Int64
	var registered atomic.Bool

	// 使用 done channel 通知 ping 协程退出，避免 goroutine 泄漏
	done := make(chan struct{})

	defer func() {
		agentWSConnCount.Add(-1) // 递减连接计数
		close(done)              // 通知 ping 协程退出
		if registered.Load() && agentID.Load() > 0 {
			// 条件注销: 仅当注册的连接仍是自己时才注销
			// 防止旧连接的 defer 关闭新连接
			h.monitor.UnregisterConnectionIfMatch(agentID.Load(), conn)
			h.wsConnsMu.Lock()
			if existing, ok := h.wsConns[agentID.Load()]; ok && existing == ws {
				delete(h.wsConns, agentID.Load())
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
	// 未认证连接不发送 ping，避免向未注册的连接发送心跳
	go func() {
		for {
			select {
			case <-pingTicker.C:
				if !registered.Load() {
					continue // 未注册不发 ping
				}
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
//  1. 新 Agent 注册: 消息携带 Code (注册码)，无 Token
//  2. 已有 Agent 会话恢复: 消息携带 Token，无 Code (Server 重启后 Agent 重连)
func (h *AgentHandler) handleRegister(ws *agentWSConn, msg *sharedmodel.WSMessage, agentID *atomic.Int64, registered *atomic.Bool) {
	// 忽略重复注册消息: Agent 已注册后再次发送 Register 时，
	// 直接忽略，避免 RegisterConnection 检测到旧连接(自身)并关闭
	if registered.Load() {
		log.Printf("Agent %d 重复发送注册消息，已忽略", agentID.Load())
		return
	}

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

		agentID.Store(agent.ID)
		registered.Store(true)

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
			Reason: "注册失败，请检查注册码是否正确",
		}
		_ = ws.writeJSON(response)
		return
	}

	agentID.Store(result.AgentID)
	registered.Store(true)

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
func (h *AgentHandler) handleReport(ws *agentWSConn, msg *sharedmodel.WSMessage, agentID *atomic.Int64, registered *atomic.Bool) {
	// Token 逐帧验证优化: 已注册连接使用缓存的 agentID，不再每帧查询数据库验证 Token
	// Token 和主机指纹已在注册（handleRegister/lazyRegister）时校验
	if !registered.Load() || agentID.Load() == 0 {
		// 速率限制：距上次 lazyRegister 不足 5 秒则忽略，防止未注册连接 DoS
		if time.Since(ws.lastLazyRegister) < 5*time.Second {
			return
		}
		ws.lastLazyRegister = time.Now()
		// 向后兼容: 旧版 Agent 重连后不发送 register，直接上报数据
		// lazyRegister 内部会验证 Token 和主机指纹
		if msg.Token == "" || !h.lazyRegister(ws, msg, agentID, registered) {
			return
		}
	}

	id := agentID.Load()

	// 校验数据
	if msg.Data == nil {
		return
	}

	// 先做频率检查（廉价操作），再执行昂贵的序列化与数据校验，
	// 防止攻击者用大体积消息反复触发 json.Marshal 造成 CPU DoS
	if err := h.validator.CheckReportFrequency(id); err != nil {
		log.Printf("Agent %d 上报频率异常: %v", id, err)
		return
	}

	// 校验数据大小 (≤10KB)
	if rawData, err := json.Marshal(msg.Data); err == nil {
		if err := h.validator.CheckDataSize(rawData); err != nil {
			log.Printf("Agent %d 数据大小超限: %v", id, err)
			return
		}
	} else {
		log.Printf("Agent %d 数据序列化失败，拒绝上报", id)
		return
	}

	if err := h.validator.ValidateMetricData(id, msg.Data); err != nil {
		log.Printf("Agent %d 数据校验失败: %v", id, err)
		return
	}

	// 写入实时数据（含静态/动态数据分离 + 哈希去重）
	if err := h.monitor.HandleAgentReport(id, msg.Data); err != nil {
		log.Printf("Agent %d 写入数据失败: %v", id, err)
		return
	}

	// 更新心跳
	h.monitor.UpdateHeartbeat(id)
}

// handlePingResult 处理 Ping 结果
func (h *AgentHandler) handlePingResult(ws *agentWSConn, msg *sharedmodel.WSMessage, agentID *atomic.Int64, registered *atomic.Bool) {
	if !registered.Load() || agentID.Load() == 0 {
		// 速率限制：距上次 lazyRegister 不足 5 秒则忽略
		if time.Since(ws.lastLazyRegister) < 5*time.Second {
			return
		}
		ws.lastLazyRegister = time.Now()
		if msg.Token == "" || !h.lazyRegister(ws, msg, agentID, registered) {
			return
		}
	}

	id := agentID.Load()

	// Token 和主机指纹已在注册时校验，不再每帧查询数据库（与 handleReport 保持一致）

	// 限制 PingData 数量，防止攻击者发送大量 PingResult 导致资源耗尽
	const maxPingResults = 100
	if len(msg.PingData) > maxPingResults {
		log.Printf("Agent %d PingData 数量超限: %d (最大 %d)", id, len(msg.PingData), maxPingResults)
		return
	}

	// 校验 Ping 数据
	for i := range msg.PingData {
		if err := h.validator.ValidatePingResult(&msg.PingData[i]); err != nil {
			log.Printf("Agent %d Ping 数据校验失败: %v", id, err)
			return
		}
	}

	// 写入 Ping 数据
	if err := h.monitor.WritePingData(id, msg.PingData); err != nil {
		log.Printf("Agent %d 写入 Ping 数据失败: %v", id, err)
		return
	}
}

// handleHeartbeat 处理心跳
func (h *AgentHandler) handleHeartbeat(ws *agentWSConn, msg *sharedmodel.WSMessage, agentID *atomic.Int64, registered *atomic.Bool) {
	// 速率限制：距上次 heartbeat 不足 5 秒则忽略，防止高频心跳导致资源耗尽
	// lastHeartbeat 仅在连接的读循环 goroutine 中访问，无需额外同步
	now := time.Now()
	if now.Sub(ws.lastHeartbeat) < 5*time.Second {
		return
	}
	ws.lastHeartbeat = now

	if !registered.Load() || agentID.Load() == 0 {
		// 向后兼容: 旧版 Agent 重连后不发送 register，直接发心跳
		if msg.Token == "" || !h.lazyRegister(ws, msg, agentID, registered) {
			return
		}
	}

	// Token 和主机指纹已在注册时校验，不再每帧查询数据库（与 handleReport 保持一致）

	h.monitor.UpdateHeartbeat(agentID.Load())

	// 发送心跳确认
	response := sharedmodel.WSMessage{
		Type: sharedmodel.MsgTypeHeartbeatAck,
	}
	_ = ws.writeJSON(response)
}

// lazyRegister 懒注册会话（向后兼容旧版 Agent）
// 当 Agent 重连后未发送 register 消息而直接上报数据时，
// 通过 Token 验证身份并建立会话
func (h *AgentHandler) lazyRegister(ws *agentWSConn, msg *sharedmodel.WSMessage, agentID *atomic.Int64, registered *atomic.Bool) bool {
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

	agentID.Store(agent.ID)
	registered.Store(true)

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
		Type:           sharedmodel.MsgTypeConfigUpdate,
		PingTargets:    config.PingTargets,
		PingInterval:   config.PingInterval,
		ReportInterval: config.ReportInterval,
	}
	_ = ws.writeJSON(response)
}

// HandleGetAgentConfig 处理 Agent 配置拉取
// 路由: GET /api/v1/agent/config
func (h *AgentHandler) HandleGetAgentConfig(c *gin.Context) {
	// 从 Authorization header 获取 Token（不再支持 query 参数，防止日志泄露）
	token := ""
	authHeader := c.GetHeader("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		token = strings.TrimPrefix(authHeader, "Bearer ")
	}
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "缺少 Token，请使用 Authorization: Bearer <token>"})
		return
	}

	_, err := h.registry.ValidateToken(token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Token 无效"})
		return
	}

	config, err := h.configSync.GetAgentConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取配置失败"})
		return
	}

	c.JSON(http.StatusOK, config)
}

// HandleGetReportInterval 获取 Agent 上报间隔
// 路由: GET /api/v1/agent/config/interval (管理员)
func (h *AgentHandler) HandleGetReportInterval(c *gin.Context) {
	interval := 3
	if h.configSync != nil {
		interval = h.configSync.GetReportInterval()
	}
	c.JSON(http.StatusOK, gin.H{"interval": interval})
}

// HandleSetReportInterval 设置 Agent 上报间隔
// 路由: PUT /api/v1/agent/config/interval (管理员)
// 修改后自动推送配置更新到所有在线 Agent
func (h *AgentHandler) HandleSetReportInterval(c *gin.Context) {
	var req struct {
		Interval int `json:"interval"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求体"})
		return
	}

	if req.Interval < 1 || req.Interval > 3600 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "上报间隔必须在 1-3600 秒之间"})
		return
	}

	if h.configSync == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "配置服务不可用"})
		return
	}

	if err := h.configSync.SetReportInterval(req.Interval); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "设置上报间隔失败"})
		return
	}

	// 推送配置更新到所有在线 Agent（异步执行，避免广播阻塞 HTTP 响应）
	if h.monitor != nil {
		config, err := h.configSync.GetAgentConfig()
		if err == nil {
			go func() {
				h.monitor.BroadcastConfigUpdate(config)
			}()
		}
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "interval": req.Interval})
}
