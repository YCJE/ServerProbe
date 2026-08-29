package api

import (
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/server-probe/server/internal/pkg"
	"github.com/server-probe/server/internal/repository"
	"github.com/server-probe/server/internal/service"
	"gorm.io/gorm"
)

// Router API 路由
type Router struct {
	router                *gin.Engine
	middleware            *Middleware
	authHandler           *AuthHandler
	serverHandler         *ServerHandler
	agentHandler          *AgentHandler
	agentAPIHandler       *AgentAPIHandler
	dashboardWSHandler    *DashboardWSHandler
	pingTargetHandler     *PingTargetHandler
	alertHandler          *AlertHandler
	notifyHandler         *NotifyHandler
	logHandler            *LogHandler
	serviceMonitorHandler *ServiceMonitorHandler
	sslMonitorHandler     *SSLMonitorHandler
	trafficHandler        *TrafficHandler
	prometheusHandler     *PrometheusHandler
	shareHandler         *ShareHandler
	tagHandler           *TagHandler
	settingsHandler      *SettingsHandler
	auditHandler         *AuditHandler
}

// NewRouter 创建路由
func NewRouter(
	jwtManager *pkg.JWTManager,
	adminRepo *repository.AdminRepository,
	sessionRepo *repository.SessionRepository,
	agentRepo *repository.AgentRepository,
	recordRepo *repository.RecordRepository,
	hourlyRepo *repository.HourlyRepository,
	monitor *service.MonitorService,
	registry *service.AgentRegistryService,
	configSync *service.ConfigSyncService,
	validator *service.DataValidator,
	pingTargetRepo *repository.PingTargetRepository,
	alertRepo *repository.AlertRepository,
	notifyRepo *repository.NotifyRepository,
	alertEngine *service.AlertEngine,
	notifySvc *service.NotifyService,
	logCapture *service.LogCapture,
	serviceMonitorRepo *repository.ServiceMonitorRepository,
	sslMonitorRepo *repository.SSLCertMonitorRepository,
	trafficRepo *repository.TrafficRepository,
	sharePageRepo *repository.SharePageRepository,
	serviceMonitorEngine *service.ServiceMonitorEngine,
	sslMonitorEngine *service.SSLMonitorEngine,
	tagRepo *repository.TagRepository,
	settingRepo *repository.SettingRepository,
	settingsSvc *service.SettingsService,
	auditRepo *repository.AuditLogRepository,
	auditSvc *service.AuditService,
	db *gorm.DB,
	dataDir string,
) *Router {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	// 信任反向代理：通过 TRUSTED_PROXIES 环境变量配置（如 "127.0.0.1,::1"）
	// 确保反代部署时 c.ClientIP() 返回真实客户端 IP，限速才能按 IP 生效
	// 未配置时不信任任何代理，ClientIP() 返回直接连接方 IP
	trustedProxies := os.Getenv("TRUSTED_PROXIES")
	if trustedProxies != "" {
		proxies := strings.Split(trustedProxies, ",")
		for i := range proxies {
			proxies[i] = strings.TrimSpace(proxies[i])
		}
		if err := r.SetTrustedProxies(proxies); err != nil {
			log.Printf("警告: 设置信任代理失败: %v", err)
		}
	} else {
		if err := r.SetTrustedProxies(nil); err != nil {
			log.Printf("警告: 设置信任代理失败: %v", err)
		}
	}

	middleware := NewMiddleware(jwtManager, sessionRepo)
	r.Use(middleware.SecurityHeaders())
	r.Use(middleware.CORS())
	r.Use(gin.Recovery())
	// 全局请求体大小限制 (1MB)，防止超大请求导致 OOM
	r.Use(func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
		c.Next()
	})

	// 创建处理器
	authHandler := NewAuthHandler(adminRepo, sessionRepo, jwtManager, auditSvc)
	serverHandler := NewServerHandler(agentRepo, monitor, recordRepo, hourlyRepo)
	serverHandler.SetSettings(settingsSvc)
	agentHandler := NewAgentHandler(registry, monitor, configSync, validator)
	agentAPIHandler := NewAgentAPIHandler(registry, agentRepo, recordRepo, monitor, alertEngine)
	dashboardWSHandler := NewDashboardWSHandler(monitor, jwtManager, sessionRepo)
	pingTargetHandler := NewPingTargetHandler(pingTargetRepo, configSync, monitor)
	alertHandler := NewAlertHandler(alertRepo, notifyRepo, alertEngine)
	notifyHandler := NewNotifyHandler(notifyRepo, notifySvc, alertRepo)
	logHandler := NewLogHandler(logCapture)
	serviceMonitorHandler := NewServiceMonitorHandler(serviceMonitorRepo, serviceMonitorEngine)
	sslMonitorHandler := NewSSLMonitorHandler(sslMonitorRepo, sslMonitorEngine)
	trafficHandler := NewTrafficHandler(trafficRepo)
	prometheusHandler := NewPrometheusHandler(monitor)
	shareHandler := NewShareHandler(sharePageRepo)
	tagHandler := NewTagHandler(tagRepo)
	settingsHandler := NewSettingsHandler(settingsSvc, recordRepo, hourlyRepo, alertRepo, tagRepo, agentRepo, db, dataDir)
	auditHandler := NewAuditHandler(auditRepo)

	// 健康检查
	r.GET("/api/v1/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true, "service": "server-probe"})
	})

	// Prometheus 指标端点（公开，限速防 DoS）
	r.GET("/metrics", middleware.PublicRateLimit(), prometheusHandler.HandleMetrics)

	// 公开分享页验证端点（无需认证，限速）
	r.GET("/api/v1/public/share/:shareId", middleware.PublicRateLimit(), shareHandler.HandlePublicSharePage)

	// API v1
	api := r.Group("/api/v1")
	{
		// 认证相关（无需登录，限速防止 DoS）
		auth := api.Group("/auth")
		auth.Use(middleware.PublicRateLimit())
		{
			auth.GET("/setup-status", authHandler.HandleCheckSetup)
			auth.GET("/me", authHandler.HandleCheckAuth)
			auth.POST("/setup", middleware.LoginRateLimit(), authHandler.HandleSetup)
			auth.POST("/login", middleware.LoginRateLimit(), authHandler.HandleLogin)
			auth.POST("/logout", authHandler.HandleLogout)
		}

		// 公开 API（无需登录，仅返回非敏感信息，限速防止 DoS）
		public := api.Group("/public")
		public.Use(middleware.PublicRateLimit())
		{
			public.GET("/servers", serverHandler.HandlePublicServers)
			public.GET("/dashboard", serverHandler.HandlePublicDashboard)
			public.GET("/servers/:id/history", serverHandler.HandlePublicServerHistory)
			// 标签列表（只读，标签名已随公开服务器数据暴露，颜色非敏感；供公开页卡片徽章取色）
			public.GET("/tags", tagHandler.HandleListTags)
			// 公开站点设置（标题/描述/公告/页脚，非敏感）
			public.GET("/settings", settingsHandler.HandlePublicSettings)
		}

		// 公开仪表盘 WebSocket（无需登录，限速防 DoS）
		r.GET("/ws/public/dashboard", middleware.PublicRateLimit(), dashboardWSHandler.HandlePublicDashboardWS)

		// 管理员仪表盘 WebSocket（需要 token，限速防 DoS）
		r.GET("/ws/dashboard", middleware.PublicRateLimit(), dashboardWSHandler.HandleDashboardWS)

		// Agent 相关（限速防止 DoS）
		agent := api.Group("/agent")
		agent.Use(middleware.PublicRateLimit())
		{
			agent.GET("/config", agentHandler.HandleGetAgentConfig)
			agent.GET("/report", agentHandler.HandleWebSocket)
		}

		// 需要认证的 API
		protected := api.Group("")
		protected.Use(middleware.AuthRequired())
		// 变更操作审计（POST/PUT/DELETE 及敏感 GET 自动落库）
		protected.Use(middleware.AuditMutations(auditSvc, adminRepo))
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
			protected.POST("/agents", agentAPIHandler.HandleCreateAgent)
			protected.PUT("/agents/:id", agentAPIHandler.HandleUpdateAgent)
			protected.PUT("/agents/:id/meta", agentAPIHandler.HandleUpdateAgentMeta)
			// Token 是 Agent 接入的唯一凭证，查看属敏感操作（2FA 再验证 + 审计）
			protected.GET("/agents/:id/token", middleware.RequireSensitive2FA(adminRepo), agentAPIHandler.HandleGetAgentToken)
			// 删除 Agent 会连带删除全部历史数据，属敏感操作（2FA 再验证 + 审计）
			protected.DELETE("/agents/:id", middleware.RequireSensitive2FA(adminRepo), agentAPIHandler.HandleDeleteAgent)

			// 探测目标管理
			protected.GET("/ping-targets", pingTargetHandler.HandleListPingTargets)
			protected.POST("/ping-targets", pingTargetHandler.HandleCreatePingTarget)
			protected.PUT("/ping-targets/:id", pingTargetHandler.HandleUpdatePingTarget)
			protected.DELETE("/ping-targets/:id", pingTargetHandler.HandleDeletePingTarget)
			protected.GET("/ping-targets/interval", pingTargetHandler.HandleGetPingInterval)
			protected.PUT("/ping-targets/interval", pingTargetHandler.HandleSetPingInterval)

			// Agent 上报间隔管理
			protected.GET("/agent/config/interval", agentHandler.HandleGetReportInterval)
			protected.PUT("/agent/config/interval", agentHandler.HandleSetReportInterval)

			// 系统状态
			protected.GET("/system/status", serverHandler.HandleSystemStatus)

			// 系统日志
			protected.GET("/logs", logHandler.HandleGetLogs)

			// 告警规则管理
			protected.GET("/alerts", alertHandler.HandleListAlerts)
			protected.POST("/alerts", alertHandler.HandleCreateAlert)
			protected.PUT("/alerts/:id", alertHandler.HandleUpdateAlert)
			protected.DELETE("/alerts/:id", alertHandler.HandleDeleteAlert)
			protected.POST("/alerts/:id/test", middleware.TestActionRateLimit(), alertHandler.HandleTestAlert)

			// 告警历史（独立前缀，避免与 /alerts/:id 参数路由冲突）
			protected.GET("/alert-history", alertHandler.HandleListAlertHistory)

			// 标签管理
			protected.GET("/tags", tagHandler.HandleListTags)
			protected.POST("/tags", tagHandler.HandleCreateTag)
			protected.PUT("/tags/:id", tagHandler.HandleUpdateTag)
			protected.DELETE("/tags/:id", tagHandler.HandleDeleteTag)

			// TOTP 两步验证（需登录后操作）
			protected.GET("/auth/totp/status", authHandler.HandleTOTPStatus)
			protected.POST("/auth/totp/setup", authHandler.HandleTOTPSetup)
			protected.POST("/auth/totp/enable", authHandler.HandleTOTPEnable)
			protected.POST("/auth/totp/disable", authHandler.HandleTOTPDisable)

			// 会话管理（P2：会话列表 + 远程注销）
			protected.GET("/auth/sessions", authHandler.HandleListSessions)
			protected.DELETE("/auth/sessions/:sessionId", authHandler.HandleRevokeSession)
			protected.POST("/auth/sessions/revoke-others", authHandler.HandleRevokeOtherSessions)

			// 通知渠道管理
			protected.GET("/notify/channels", notifyHandler.HandleListChannels)
			protected.POST("/notify/channels", notifyHandler.HandleCreateChannel)
			protected.PUT("/notify/channels/:id", notifyHandler.HandleUpdateChannel)
			protected.DELETE("/notify/channels/:id", notifyHandler.HandleDeleteChannel)
			protected.POST("/notify/channels/:id/test", middleware.TestActionRateLimit(), notifyHandler.HandleTestChannel)

			// 服务监控管理 (P0-3)
			protected.GET("/service-monitors", serviceMonitorHandler.HandleListServiceMonitors)
			protected.POST("/service-monitors", serviceMonitorHandler.HandleCreateServiceMonitor)
			protected.PUT("/service-monitors/:id", serviceMonitorHandler.HandleUpdateServiceMonitor)
			protected.DELETE("/service-monitors/:id", serviceMonitorHandler.HandleDeleteServiceMonitor)
			protected.POST("/service-monitors/:id/test", middleware.TestActionRateLimit(), serviceMonitorHandler.HandleTestServiceMonitor)
			protected.GET("/service-monitors/statuses", serviceMonitorHandler.HandleServiceMonitorStatuses)

			// SSL 证书监控管理 (P0-4)
			protected.GET("/ssl-monitors", sslMonitorHandler.HandleListSSLMonitors)
			protected.POST("/ssl-monitors", sslMonitorHandler.HandleCreateSSLMonitor)
			protected.PUT("/ssl-monitors/:id", sslMonitorHandler.HandleUpdateSSLMonitor)
			protected.DELETE("/ssl-monitors/:id", sslMonitorHandler.HandleDeleteSSLMonitor)
			protected.POST("/ssl-monitors/:id/test", middleware.TestActionRateLimit(), sslMonitorHandler.HandleTestSSLMonitor)
			protected.GET("/ssl-monitors/statuses", sslMonitorHandler.HandleSSLMonitorStatuses)

			// 流量统计 (P0-1)
			protected.GET("/traffic", trafficHandler.HandleGetAllTraffic)
			protected.GET("/traffic/:agentId", trafficHandler.HandleGetTraffic)

			// 分享页管理 (P1-8)
			protected.GET("/share-pages", shareHandler.HandleListSharePages)
			protected.POST("/share-pages", shareHandler.HandleCreateSharePage)
			protected.GET("/share-pages/:id", shareHandler.HandleGetSharePage)
			protected.PUT("/share-pages/:id", shareHandler.HandleUpdateSharePage)
			protected.DELETE("/share-pages/:id", shareHandler.HandleDeleteSharePage)

			// 系统设置（站点信息 + 数据加载参数）
			protected.GET("/settings", settingsHandler.HandleGetSettings)
			protected.PUT("/settings", settingsHandler.HandleUpdateSettings)
			// 设置导出/导入涉及站点全部配置迁移，属敏感操作（2FA 再验证 + 审计）
			protected.GET("/settings/export", middleware.RequireSensitive2FA(adminRepo), settingsHandler.HandleExportSettings)
			protected.POST("/settings/import", middleware.RequireSensitive2FA(adminRepo), settingsHandler.HandleImportSettings)

			// 数据库管理（统计/备份/清理/压缩）
			protected.GET("/db/stats", settingsHandler.HandleDBStats)
			protected.GET("/db/backup", settingsHandler.HandleDBBackup)
			// 按天清理历史数据不可恢复，属敏感操作（2FA 再验证 + 审计）
			protected.POST("/db/cleanup", middleware.RequireSensitive2FA(adminRepo), settingsHandler.HandleDBCleanup)
			protected.POST("/db/compact", settingsHandler.HandleDBCompact)

			// 审计日志查询（P1：安全闭环）
			protected.GET("/audit-logs", auditHandler.HandleListAuditLogs)
		}
	}

	return &Router{
		router:                r,
		middleware:            middleware,
		authHandler:           authHandler,
		serverHandler:         serverHandler,
		agentHandler:          agentHandler,
		agentAPIHandler:       agentAPIHandler,
		dashboardWSHandler:    dashboardWSHandler,
		pingTargetHandler:     pingTargetHandler,
		alertHandler:          alertHandler,
		notifyHandler:         notifyHandler,
		logHandler:            logHandler,
		serviceMonitorHandler: serviceMonitorHandler,
		sslMonitorHandler:     sslMonitorHandler,
		trafficHandler:        trafficHandler,
		prometheusHandler:     prometheusHandler,
		shareHandler:         shareHandler,
		tagHandler:           tagHandler,
		settingsHandler:      settingsHandler,
		auditHandler:         auditHandler,
	}
}

// GetRouter 返回 gin 引擎
func (r *Router) GetRouter() *gin.Engine {
	return r.router
}

// 确保 websocket 包被使用
var _ = websocket.ErrBadHandshake
