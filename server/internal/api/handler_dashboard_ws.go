package api

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/server-probe/server/internal/pkg"
	"github.com/server-probe/server/internal/service"
)

// DashboardWSHandler 仪表盘 WebSocket 处理器
type DashboardWSHandler struct {
	monitor   *service.MonitorService
	jwtManager *pkg.JWTManager
	upgrader  websocket.Upgrader
}

// NewDashboardWSHandler 创建仪表盘 WebSocket 处理器
func NewDashboardWSHandler(monitor *service.MonitorService, jwtManager *pkg.JWTManager) *DashboardWSHandler {
	return &DashboardWSHandler{
		monitor:    monitor,
		jwtManager: jwtManager,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // 允许所有来源（生产环境应限制）
			},
		},
	}
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
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
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
	if !h.pushDashboardData(conn) {
		return
	}

	for {
		select {
		case <-done:
			// 客户端已断开
			return
		case <-ticker.C:
			if !h.pushDashboardData(conn) {
				return
			}
		}
	}
}

// pushDashboardData 推送仪表盘数据，返回是否成功
func (h *DashboardWSHandler) pushDashboardData(conn *websocket.Conn) bool {
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
	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		log.Printf("Dashboard WebSocket 写入失败: %v", err)
		return false
	}

	return true
}
