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
	agentAPIHandler := NewAgentAPIHandler(registry, agentRepo)
	dashboardWSHandler := NewDashboardWSHandler(monitor, jwtManager)

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
			protected.DELETE("/agents/:id", agentAPIHandler.HandleDeleteAgent)
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
	}
}

// GetRouter 返回 gin 引擎
func (r *Router) GetRouter() *gin.Engine {
	return r.router
}

// 确保 websocket 包被使用
var _ = websocket.ErrBadHandshake
