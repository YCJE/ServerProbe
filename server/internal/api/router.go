package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/server-probe/server/internal/pkg"
	"github.com/server-probe/server/internal/repository"
	"github.com/server-probe/server/internal/service"
)

// Router API 路由
type Router struct {
	router             *gin.Engine
	middleware         *Middleware
	authHandler        *AuthHandler
	serverHandler      *ServerHandler
	agentHandler       *AgentHandler
	agentAPIHandler    *AgentAPIHandler
	dashboardWSHandler *DashboardWSHandler
	pingTargetHandler  *PingTargetHandler
	alertHandler       *AlertHandler
	notifyHandler      *NotifyHandler
}

// NewRouter 创建路由
func NewRouter(
	jwtManager *pkg.JWTManager,
	adminRepo *repository.AdminRepository,
	agentRepo *repository.AgentRepository,
	recordRepo *repository.RecordRepository,
	monitor *service.MonitorService,
	registry *service.AgentRegistryService,
	configSync *service.ConfigSyncService,
	validator *service.DataValidator,
	pingTargetRepo *repository.PingTargetRepository,
	alertRepo *repository.AlertRepository,
	notifyRepo *repository.NotifyRepository,
	alertEngine *service.AlertEngine,
	notifySvc *service.NotifyService,
) *Router {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	middleware := NewMiddleware(jwtManager)
	r.Use(middleware.SecurityHeaders())
	r.Use(middleware.CORS())
	r.Use(gin.Recovery())

	// 创建处理器
	authHandler := NewAuthHandler(adminRepo, jwtManager)
	serverHandler := NewServerHandler(agentRepo, monitor, recordRepo)
	agentHandler := NewAgentHandler(registry, monitor, configSync, validator)
	agentAPIHandler := NewAgentAPIHandler(registry, agentRepo, monitor)
	dashboardWSHandler := NewDashboardWSHandler(monitor, jwtManager)
	pingTargetHandler := NewPingTargetHandler(pingTargetRepo, configSync, monitor)
	alertHandler := NewAlertHandler(alertRepo, notifyRepo, alertEngine)
	notifyHandler := NewNotifyHandler(notifyRepo, notifySvc)

	// 健康检查
	r.GET("/api/v1/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true, "service": "server-probe"})
	})

	// API v1
	api := r.Group("/api/v1")
	{
		// 认证相关（无需登录）
		auth := api.Group("/auth")
		{
			auth.GET("/setup-status", authHandler.HandleCheckSetup)
			auth.POST("/setup", authHandler.HandleSetup)
			auth.POST("/login", middleware.LoginRateLimit(), authHandler.HandleLogin)
			auth.POST("/logout", authHandler.HandleLogout)
		}

		// 公开 API（无需登录，仅返回非敏感信息）
		public := api.Group("/public")
		{
			public.GET("/servers", serverHandler.HandlePublicServers)
			public.GET("/dashboard", serverHandler.HandlePublicDashboard)
			public.GET("/servers/:id/history", serverHandler.HandleGetServerHistory)
		}

		// 公开仪表盘 WebSocket（无需登录）
		r.GET("/ws/public/dashboard", dashboardWSHandler.HandlePublicDashboardWS)

		// 管理员仪表盘 WebSocket（需要 token）
		r.GET("/ws/dashboard", dashboardWSHandler.HandleDashboardWS)

		// Agent 相关
		agent := api.Group("/agent")
		{
			agent.GET("/config", agentHandler.HandleGetAgentConfig)
			agent.GET("/report", agentHandler.HandleWebSocket)
		}

		// 需要认证的 API
		protected := api.Group("")
		protected.Use(middleware.AuthRequired())
		{
			// 服务器
			protected.GET("/servers", serverHandler.HandleListServers)
			protected.GET("/servers/:id", serverHandler.HandleGetServer)
			protected.GET("/servers/:id/history", serverHandler.HandleGetServerHistory)
			protected.GET("/dashboard", serverHandler.HandleDashboard)

			// 注册码管理
			protected.GET("/agents/register-codes", agentAPIHandler.HandleListRegisterCodes)
			protected.POST("/agents/register-codes", agentAPIHandler.HandleGenerateRegisterCode)
			protected.DELETE("/agents/register-codes/:code", agentAPIHandler.HandleDeleteRegisterCode)

			// Agent 管理
			protected.GET("/agents", agentAPIHandler.HandleListAgents)
			protected.PUT("/agents/:id", agentAPIHandler.HandleUpdateAgent)
			protected.DELETE("/agents/:id", agentAPIHandler.HandleDeleteAgent)

			// 探测目标管理
			protected.GET("/ping-targets", pingTargetHandler.HandleListPingTargets)
			protected.POST("/ping-targets", pingTargetHandler.HandleCreatePingTarget)
			protected.PUT("/ping-targets/:id", pingTargetHandler.HandleUpdatePingTarget)
			protected.DELETE("/ping-targets/:id", pingTargetHandler.HandleDeletePingTarget)
			protected.GET("/ping-targets/interval", pingTargetHandler.HandleGetPingInterval)
			protected.PUT("/ping-targets/interval", pingTargetHandler.HandleSetPingInterval)

			// 系统状态
			protected.GET("/system/status", serverHandler.HandleSystemStatus)

			// 告警规则管理
			protected.GET("/alerts", alertHandler.HandleListAlerts)
			protected.POST("/alerts", alertHandler.HandleCreateAlert)
			protected.PUT("/alerts/:id", alertHandler.HandleUpdateAlert)
			protected.DELETE("/alerts/:id", alertHandler.HandleDeleteAlert)
			protected.POST("/alerts/:id/test", alertHandler.HandleTestAlert)

			// 通知渠道管理
			protected.GET("/notify/channels", notifyHandler.HandleListChannels)
			protected.POST("/notify/channels", notifyHandler.HandleCreateChannel)
			protected.PUT("/notify/channels/:id", notifyHandler.HandleUpdateChannel)
			protected.DELETE("/notify/channels/:id", notifyHandler.HandleDeleteChannel)
			protected.POST("/notify/channels/:id/test", notifyHandler.HandleTestChannel)
		}
	}

	return &Router{
		router:             r,
		middleware:         middleware,
		authHandler:        authHandler,
		serverHandler:      serverHandler,
		agentHandler:       agentHandler,
		agentAPIHandler:    agentAPIHandler,
		dashboardWSHandler: dashboardWSHandler,
		pingTargetHandler:  pingTargetHandler,
		alertHandler:       alertHandler,
		notifyHandler:      notifyHandler,
	}
}

// GetRouter 返回 gin 引擎
func (r *Router) GetRouter() *gin.Engine {
	return r.router
}

// 确保 websocket 包被使用
var _ = websocket.ErrBadHandshake
