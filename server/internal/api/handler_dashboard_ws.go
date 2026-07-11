package api

import (
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/server-probe/server/internal/pkg"
	"github.com/server-probe/server/internal/service"
)

// 管理员与公开仪表盘 WebSocket 各自独立的连接上限，防止公开用户阻断管理员
const (
	maxAdminDashboardWSConnections  = 50  // 管理员独立配额
	maxPublicDashboardWSConnections = 200 // 公开独立配额
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
	w.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return w.conn.WriteMessage(messageType, data)
}

func (w *wsConn) writeJSON(v interface{}) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return w.conn.WriteJSON(v)
}

// HandleDashboardWS 仪表盘 WebSocket 端点
// 路由: GET /ws/dashboard
// 认证方式: HttpOnly Cookie（浏览器自动携带，无需通过 URL 参数传递，防止日志泄露）
func (h *DashboardWSHandler) HandleDashboardWS(c *gin.Context) {
	// 从 Cookie 中获取 JWT token（HttpOnly，JS 无法读取，不会出现在日志/Referer 中）
	token, err := c.Cookie("token")
	if err != nil || token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "缺少 token"})
		return
	}

	// 验证 JWT token
	claims, err := h.jwtManager.ValidateToken(token)
	if err != nil {
		// Token 过期或无效，清除 Cookie 防止浏览器持续发送过期凭证
		c.SetSameSite(http.SameSiteStrictMode)
		c.SetCookie("token", "", -1, "/", "", cookieSecure(), true)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Token 无效或已过期"})
		return
	}
	_ = claims

	// 连接数限制：管理员独立配额（先递增再检查，消除 TOCTOU 竞态）
	newCount := h.monitor.IncDashboardWS()
	if newCount > maxAdminDashboardWSConnections {
		h.monitor.DecDashboardWS()
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "连接数已满"})
		return
	}

	// 升级为 WebSocket 连接
	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.monitor.DecDashboardWS()
		log.Printf("Dashboard WebSocket 升级失败: %v", err)
		return
	}
	defer h.monitor.DecDashboardWS()
	defer conn.Close()

	ws := &wsConn{conn: conn}

	// 设置读超时和 pong 处理器
	conn.SetReadLimit(1024 * 1024) // 1MB 读取限制
	conn.SetReadDeadline(time.Now().Add(120 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(120 * time.Second))
		return nil
	})

	// 启动 ping 协程，保持连接活跃 (使用 done channel 避免泄漏)
	pingTicker := time.NewTicker(30 * time.Second)
	defer pingTicker.Stop()

	// doneRead 由读取协程控制，donePing 由 ping 协程使用
	donePing := make(chan struct{})

	go func() {
		for {
			select {
			case <-pingTicker.C:
				if err := ws.writeMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			case <-donePing:
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
		close(donePing)
		return
	}

	for {
		select {
		case <-done:
			// 客户端已断开
			close(donePing)
			return
		case <-ticker.C:
			if !h.pushDashboardData(ws) {
				close(donePing)
				return
			}
		}
	}
}

// pushDashboardData 推送仪表盘数据（P0-1: 使用预序列化缓存，所有客户端共享同一份 JSON）
func (h *DashboardWSHandler) pushDashboardData(ws *wsConn) bool {
	data := h.monitor.GetDashboardJSON()
	if data == nil {
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
	// 连接数限制：公开独立配额（先递增再检查，消除 TOCTOU 竞态）
	newCount := h.monitor.IncPublicDashboardWS()
	if newCount > maxPublicDashboardWSConnections {
		h.monitor.DecPublicDashboardWS()
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "连接数已满"})
		return
	}

	// 升级为 WebSocket 连接
	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.monitor.DecPublicDashboardWS()
		log.Printf("Public Dashboard WebSocket 升级失败: %v", err)
		return
	}
	defer h.monitor.DecPublicDashboardWS()
	defer conn.Close()

	ws := &wsConn{conn: conn}

	// 设置读超时和 pong 处理器
	conn.SetReadLimit(1024 * 1024) // 1MB 读取限制
	conn.SetReadDeadline(time.Now().Add(120 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(120 * time.Second))
		return nil
	})

	// 启动 ping 协程 (使用 donePing channel 避免泄漏)
	pingTicker := time.NewTicker(30 * time.Second)
	defer pingTicker.Stop()

	donePing := make(chan struct{})
	go func() {
		for {
			select {
			case <-pingTicker.C:
				if err := ws.writeMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			case <-donePing:
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
		close(donePing)
		return
	}

	for {
		select {
		case <-done:
			close(donePing)
			return
		case <-ticker.C:
			if !h.pushPublicDashboardData(ws) {
				close(donePing)
				return
			}
		}
	}
}

// pushPublicDashboardData 推送公开仪表盘数据（P0-1: 使用预序列化缓存，过滤敏感字段 + 摘要）
func (h *DashboardWSHandler) pushPublicDashboardData(ws *wsConn) bool {
	data := h.monitor.GetPublicDashboardJSON()
	if data == nil {
		return true
	}

	if err := ws.writeMessage(websocket.TextMessage, data); err != nil {
		return false
	}

	return true
}
