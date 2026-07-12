package api

import (
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/server-probe/server/internal/model"
	"github.com/server-probe/server/internal/repository"
	"github.com/server-probe/server/internal/service"
)

// SSLMonitorHandler SSL 证书监控处理器
type SSLMonitorHandler struct {
	repo   *repository.SSLCertMonitorRepository
	engine *service.SSLMonitorEngine
}

// NewSSLMonitorHandler 创建 SSL 证书监控处理器
func NewSSLMonitorHandler(repo *repository.SSLCertMonitorRepository, engine *service.SSLMonitorEngine) *SSLMonitorHandler {
	return &SSLMonitorHandler{repo: repo, engine: engine}
}

// HandleListSSLMonitors 获取 SSL 证书监控列表
// 路由: GET /api/v1/ssl-monitors
func (h *SSLMonitorHandler) HandleListSSLMonitors(c *gin.Context) {
	monitors, err := h.repo.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取 SSL 证书监控列表失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"monitors": monitors})
}

// HandleCreateSSLMonitor 创建 SSL 证书监控
// 路由: POST /api/v1/ssl-monitors
func (h *SSLMonitorHandler) HandleCreateSSLMonitor(c *gin.Context) {
	var req struct {
		Domain    string `json:"domain"`
		Port      int    `json:"port"`
		AlertDays int    `json:"alert_days"`
		Enabled   bool   `json:"enabled"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求体"})
		return
	}

	if req.Domain == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "域名不能为空"})
		return
	}
	if len(req.Domain) > 253 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "域名过长"})
		return
	}

	if req.Port == 0 {
		req.Port = 443
	}
	if req.AlertDays == 0 {
		req.AlertDays = 30
	}

	// 范围校验（与前端一致）
	if req.Port < 1 || req.Port > 65535 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "端口必须在 1-65535 之间"})
		return
	}
	if req.AlertDays < 1 || req.AlertDays > 365 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "告警天数必须在 1-365 之间"})
		return
	}

	monitor := &model.SSLCertMonitor{
		Domain:    req.Domain,
		Port:      req.Port,
		AlertDays: req.AlertDays,
		Enabled:   req.Enabled,
	}

	if err := h.repo.Create(monitor); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建 SSL 证书监控失败"})
		return
	}

	// GORM v2 对有 default tag 且字段值为零值(false)的字段会在 INSERT 中省略，
	// 让数据库使用 DEFAULT 值(true)。用户显式指定 enabled=false 时，需 Create 后用 Select 强制覆盖。
	if !req.Enabled {
		if err := h.repo.UpdateEnabled(monitor, false); err != nil {
			log.Printf("[API] Failed to update SSL monitor enabled field: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "创建 SSL 证书监控成功但禁用状态更新失败"})
			return
		}
		monitor.Enabled = false
	}

	c.JSON(http.StatusOK, gin.H{"monitor": monitor})
}

// HandleUpdateSSLMonitor 更新 SSL 证书监控
// 路由: PUT /api/v1/ssl-monitors/:id
func (h *SSLMonitorHandler) HandleUpdateSSLMonitor(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的监控 ID"})
		return
	}

	monitor, err := h.repo.GetByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "SSL 证书监控不存在"})
		return
	}

	var req struct {
		Domain    *string `json:"domain"`
		Port      *int    `json:"port"`
		AlertDays *int    `json:"alert_days"`
		Enabled   *bool   `json:"enabled"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求体"})
		return
	}

	if req.Domain != nil {
		if *req.Domain == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "域名不能为空"})
			return
		}
		if len(*req.Domain) > 253 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "域名过长"})
			return
		}
		monitor.Domain = *req.Domain
	}
	if req.Port != nil {
		if *req.Port < 1 || *req.Port > 65535 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "端口必须在 1-65535 之间"})
			return
		}
		monitor.Port = *req.Port
	}
	if req.AlertDays != nil {
		if *req.AlertDays < 1 || *req.AlertDays > 365 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "告警天数必须在 1-365 之间"})
			return
		}
		monitor.AlertDays = *req.AlertDays
	}
	if req.Enabled != nil {
		monitor.Enabled = *req.Enabled
	}

	if err := h.repo.Update(monitor); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新 SSL 证书监控失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"monitor": monitor})
}

// HandleDeleteSSLMonitor 删除 SSL 证书监控
// 路由: DELETE /api/v1/ssl-monitors/:id
func (h *SSLMonitorHandler) HandleDeleteSSLMonitor(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的监控 ID"})
		return
	}

	if err := h.repo.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除 SSL 证书监控失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// HandleTestSSLMonitor 测试 SSL 证书监控（立即执行一次检查）
// 路由: POST /api/v1/ssl-monitors/:id/test
func (h *SSLMonitorHandler) HandleTestSSLMonitor(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的监控 ID"})
		return
	}

	monitor, err := h.repo.GetByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "SSL 证书监控不存在"})
		return
	}

	if h.engine == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "SSL 证书监控引擎不可用"})
		return
	}

	remainingDays, expiryDate, checkErr := h.engine.CheckCert(monitor)
	if checkErr != nil {
		log.Printf("SSL 证书检查失败 (ID=%d): %v", id, checkErr)
		c.JSON(http.StatusOK, gin.H{
			"status": "error",
			"error":  "SSL 证书检查失败，请检查域名和端口是否正确",
			"domain": monitor.Domain,
		})
		return
	}

	status := "ok"
	if remainingDays < monitor.AlertDays {
		status = "warning"
	}

	c.JSON(http.StatusOK, gin.H{
		"status":         status,
		"domain":         monitor.Domain,
		"remaining_days": remainingDays,
		"expiry_date":    expiryDate,
	})
}

// HandleSSLMonitorStatuses 获取所有 SSL 证书监控的当前状态
// 路由: GET /api/v1/ssl-monitors/statuses
func (h *SSLMonitorHandler) HandleSSLMonitorStatuses(c *gin.Context) {
	if h.engine == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "SSL 证书监控引擎不可用"})
		return
	}

	statuses := h.engine.GetAllStatuses()
	c.JSON(http.StatusOK, gin.H{"statuses": statuses})
}
