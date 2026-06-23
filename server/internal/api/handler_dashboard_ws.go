package api

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/server-probe/server/internal/pkg"
	"github.com/server-probe/server/internal/service"
	sharedmodel "github.com/server-probe/shared/model"
)

// DashboardWSHandler 仪表盘 WebSocket 处理器
type DashboardWSHandler struct {
	monitor    *service.MonitorService
	jwtManager *pkg.JWTManager
	upgrader   websocket.Upgrader
}

// NewDashboardWSHandler 创建仪表盘 WebSocket 处理器
func NewDashboardWSHandler(monitor *service.MonitorService, jwtManager *pkg.JWTManager) *DashboardWSHandler {
	return &DashboardWSHandler{
		monitor:    monitor,
		jwtManager: jwtManager,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				// 仅允许同源请求，防止跨站 WebSocket 劫持
				origin := r.Header.Get("Origin")
				if origin == "" {
					return true // 非浏览器客户端
				}
				host := r.Host
				return origin == "https://"+host || origin == "http://"+host
			},
		},
	}
}

// wsConn 封装 WebSocket 连接，添加写锁
type wsConn struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (w *wsConn) writeMessage(messageType int, data []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.conn.WriteMessage(messageType, data)
}

func (w *wsConn) writeJSON(v interface{}) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.conn.WriteJSON(v)
}

// HandleDashboardWS 仪表盘 WebSocket 端点
// 路由: GET /ws/dashboard?token=JWT_TOKEN
func (h *DashboardWSHandler) HandleDashboardWS(c *gin.Context) {
	// 从 query 参数获取 JWT token
	token := c.Query("token")
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "缺少 token"})
		return
	}

	// 验证 JWT token
	claims, err := h.jwtManager.ValidateToken(token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Token 无效或已过期"})
		return
	}
	_ = claims

	// 升级为 WebSocket 连接
	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("Dashboard WebSocket 升级失败: %v", err)
		return
	}
	defer conn.Close()

	h.monitor.IncDashboardWS()
	defer h.monitor.DecDashboardWS()

	ws := &wsConn{conn: conn}

	// 设置读超时和 pong 处理器
	conn.SetReadDeadline(time.Now().Add(120 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(120 * time.Second))
		return nil
	})

	// 启动 ping 协程，保持连接活跃
	pingTicker := time.NewTicker(30 * time.Second)
	defer pingTicker.Stop()

	go func() {
		for range pingTicker.C {
			if err := ws.writeMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}()

	// 启动一个协程读取客户端消息（主要用于检测连接关闭）
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	// 每 3 秒推送一次仪表盘数据
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	// 立即推送一次数据
	if !h.pushDashboardData(ws) {
		return
	}

	for {
		select {
		case <-done:
			// 客户端已断开
			return
		case <-ticker.C:
			if !h.pushDashboardData(ws) {
				return
			}
		}
	}
}

// pushDashboardData 推送仪表盘数据，返回是否成功
func (h *DashboardWSHandler) pushDashboardData(ws *wsConn) bool {
	items := h.monitor.GetDashboardData()

	message := gin.H{
		"type":    "dashboard_update",
		"servers": items,
	}

	data, err := json.Marshal(message)
	if err != nil {
		log.Printf("Dashboard 数据序列化失败: %v", err)
		return true // 序列化失败不影响连接
	}

	// 加锁写入，避免与 ping 协程竞争
	if err := ws.writeMessage(websocket.TextMessage, data); err != nil {
		log.Printf("Dashboard WebSocket 写入失败: %v", err)
		return false
	}

	return true
}

// HandlePublicDashboardWS 公开仪表盘 WebSocket 端点 (无需登录)
// 路由: GET /ws/public/dashboard
func (h *DashboardWSHandler) HandlePublicDashboardWS(c *gin.Context) {
	// 升级为 WebSocket 连接
	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("Public Dashboard WebSocket 升级失败: %v", err)
		return
	}
	defer conn.Close()

	ws := &wsConn{conn: conn}

	// 设置读超时和 pong 处理器
	conn.SetReadDeadline(time.Now().Add(120 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(120 * time.Second))
		return nil
	})

	// 启动 ping 协程
	pingTicker := time.NewTicker(30 * time.Second)
	defer pingTicker.Stop()

	go func() {
		for range pingTicker.C {
			if err := ws.writeMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}()

	// 启动读协程检测连接关闭
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	// 每 3 秒推送一次公开仪表盘数据
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	// 立即推送一次
	if !h.pushPublicDashboardData(ws) {
		return
	}

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			if !h.pushPublicDashboardData(ws) {
				return
			}
		}
	}
}

// pushPublicDashboardData 推送公开仪表盘数据 (过滤敏感字段)
func (h *DashboardWSHandler) pushPublicDashboardData(ws *wsConn) bool {
	items := h.monitor.GetDashboardData()

	// 过滤敏感字段
	type PublicItem struct {
		AgentID     int64                    `json:"agent_id"`
		Hostname    string                   `json:"hostname"`
		DisplayName string                   `json:"display_name"`
		Online      bool                     `json:"online"`
		CPU         float64                  `json:"cpu"`
		Mem         float64                  `json:"mem"`
		MemTotal    uint64                   `json:"mem_total"`
		MemUsed     uint64                   `json:"mem_used"`
		NetRx       uint64                   `json:"net_rx"`
		NetTx       uint64                   `json:"net_tx"`
		Load1       float64                  `json:"load_1"`
		Uptime      uint64                   `json:"uptime"`
		PingData    []sharedmodel.PingResult `json:"ping_data"`
		Timestamp   int64                    `json:"timestamp"`
	}

	publicItems := make([]PublicItem, 0, len(items))
	for _, item := range items {
		// 过滤 PingData 中的 Target 字段 (敏感信息)
		publicPing := make([]sharedmodel.PingResult, 0, len(item.PingData))
		for _, p := range item.PingData {
			publicPing = append(publicPing, sharedmodel.PingResult{
				Name:        p.Name,
				Method:      p.Method,
				AvgLatency:  p.AvgLatency,
				MinLatency:  p.MinLatency,
				MaxLatency:  p.MaxLatency,
				Jitter:      p.Jitter,
				Loss:        p.Loss,
				PacketsSent: p.PacketsSent,
				PacketsRecv: p.PacketsRecv,
				// Target 字段不包含，防止泄露探测目标地址
			})
		}

		publicItems = append(publicItems, PublicItem{
			AgentID:     item.AgentID,
			Hostname:    item.Hostname,
			DisplayName: item.DisplayName,
			Online:      item.Online,
			CPU:         item.CPU,
			Mem:         item.Mem,
			MemTotal:    item.MemTotal,
			MemUsed:     item.MemUsed,
			NetRx:       item.NetRx,
			NetTx:       item.NetTx,
			Load1:       item.Load1,
			Uptime:      item.Uptime,
			PingData:    publicPing,
			Timestamp:   item.Timestamp,
		})
	}

	message := gin.H{
		"type":    "dashboard_update",
		"servers": publicItems,
	}

	data, err := json.Marshal(message)
	if err != nil {
		return true
	}

	if err := ws.writeMessage(websocket.TextMessage, data); err != nil {
		return false
	}

	return true
}
