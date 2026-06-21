package api

import (
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/server-probe/server/internal/pkg"
	"github.com/server-probe/server/internal/repository"
	"github.com/server-probe/server/internal/service"
	"net/http"
)

// Router API 路由
type Router struct {
	router      *gin.Engine
	middleware  *Middleware
	authHandler *AuthHandler
	serverHandler *ServerHandler
	agentHandler *AgentHandler
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

			// 注册码
			protected.GET("/agents/register-codes", h.handleListRegisterCodes)
			protected.POST("/agents/register-codes", h.handleGenerateRegisterCode)
			protected.DELETE("/agents/register-codes/:code", h.handleDeleteRegisterCode)
		}
	}

	return &Router{
		router:        r,
		middleware:    middleware,
		authHandler:   authHandler,
		serverHandler: serverHandler,
		agentHandler:  agentHandler,
	}
}

// GetRouter 返回 gin 引擎
func (r *Router) GetRouter() *gin.Engine {
	return r.router
}

// 临时占位，后面移到单独的 handler
var h = &placeholderHandler{}

type placeholderHandler struct{}

func (p *placeholderHandler) handleListRegisterCodes(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"codes": []interface{}{}})
}

func (p *placeholderHandler) handleGenerateRegisterCode(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"code": ""})
}

func (p *placeholderHandler) handleDeleteRegisterCode(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// 确保 websocket 包被使用
var _ = websocket.ErrBadHandshake
