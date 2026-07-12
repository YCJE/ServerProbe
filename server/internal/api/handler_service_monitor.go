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

// ServiceMonitorHandler 服务监控处理器
type ServiceMonitorHandler struct {
	repo   *repository.ServiceMonitorRepository
	engine *service.ServiceMonitorEngine
}

// NewServiceMonitorHandler 创建服务监控处理器
func NewServiceMonitorHandler(repo *repository.ServiceMonitorRepository, engine *service.ServiceMonitorEngine) *ServiceMonitorHandler {
	return &ServiceMonitorHandler{repo: repo, engine: engine}
}

// HandleListServiceMonitors 获取服务监控列表
// 路由: GET /api/v1/service-monitors
func (h *ServiceMonitorHandler) HandleListServiceMonitors(c *gin.Context) {
	monitors, err := h.repo.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取服务监控列表失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"monitors": monitors})
}

// HandleCreateServiceMonitor 创建服务监控
// 路由: POST /api/v1/service-monitors
func (h *ServiceMonitorHandler) HandleCreateServiceMonitor(c *gin.Context) {
	var req struct {
		Name           string `json:"name"`
		Type           string `json:"type"`
		Target         string `json:"target"`
		ExpectedStatus int    `json:"expected_status"`
		Timeout        int    `json:"timeout"`
		Interval       int    `json:"interval"`
		Enabled        bool   `json:"enabled"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求体"})
		return
	}

	if req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "名称不能为空"})
		return
	}
	if req.Type == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "类型不能为空"})
		return
	}
	if req.Target == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "目标不能为空"})
		return
	}

	validTypes := map[string]bool{"http": true, "tcp": true}
	if !validTypes[req.Type] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的监控类型，支持: http, tcp"})
		return
	}

	if req.ExpectedStatus == 0 {
		req.ExpectedStatus = 200
	}
	if req.Timeout == 0 {
		req.Timeout = 10
	}
	if req.Interval == 0 {
		req.Interval = 60
	}

	monitor := &model.ServiceMonitor{
		Name:           req.Name,
		Type:           req.Type,
		Target:         req.Target,
		ExpectedStatus: req.ExpectedStatus,
		Timeout:        req.Timeout,
		Interval:       req.Interval,
		Enabled:        req.Enabled,
	}

	if err := h.repo.Create(monitor); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建服务监控失败"})
		return
	}

	// GORM v2 对有 default tag 且字段值为零值(false)的字段会在 INSERT 中省略，
	// 让数据库使用 DEFAULT 值(true)。用户显式指定 enabled=false 时，需 Create 后用 Select 强制覆盖。
	if !req.Enabled {
		if err := h.repo.UpdateEnabled(monitor, false); err != nil {
			log.Printf("[API] Failed to update service monitor enabled field: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "创建服务监控成功但禁用状态更新失败"})
			return
		}
		monitor.Enabled = false
	}

	c.JSON(http.StatusOK, gin.H{"monitor": monitor})
}

// HandleUpdateServiceMonitor 更新服务监控
// 路由: PUT /api/v1/service-monitors/:id
func (h *ServiceMonitorHandler) HandleUpdateServiceMonitor(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的监控 ID"})
		return
	}

	monitor, err := h.repo.GetByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务监控不存在"})
		return
	}

	var req struct {
		Name           *string `json:"name"`
		Type           *string `json:"type"`
		Target         *string `json:"target"`
		ExpectedStatus *int    `json:"expected_status"`
		Timeout        *int    `json:"timeout"`
		Interval       *int    `json:"interval"`
		Enabled        *bool   `json:"enabled"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求体"})
		return
	}

	if req.Name != nil {
		if *req.Name == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "名称不能为空"})
			return
		}
		monitor.Name = *req.Name
	}
	if req.Type != nil {
		validTypes := map[string]bool{"http": true, "tcp": true}
		if !validTypes[*req.Type] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "无效的监控类型"})
			return
		}
		monitor.Type = *req.Type
	}
	if req.Target != nil {
		if *req.Target == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "目标不能为空"})
			return
		}
		monitor.Target = *req.Target
	}
	if req.ExpectedStatus != nil {
		monitor.ExpectedStatus = *req.ExpectedStatus
	}
	if req.Timeout != nil {
		monitor.Timeout = *req.Timeout
	}
	if req.Interval != nil {
		monitor.Interval = *req.Interval
	}
	if req.Enabled != nil {
		monitor.Enabled = *req.Enabled
	}

	if err := h.repo.Update(monitor); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新服务监控失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"monitor": monitor})
}

// HandleDeleteServiceMonitor 删除服务监控
// 路由: DELETE /api/v1/service-monitors/:id
func (h *ServiceMonitorHandler) HandleDeleteServiceMonitor(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的监控 ID"})
		return
	}

	if err := h.repo.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除服务监控失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// HandleTestServiceMonitor 测试服务监控（立即执行一次探测）
// 路由: POST /api/v1/service-monitors/:id/test
func (h *ServiceMonitorHandler) HandleTestServiceMonitor(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的监控 ID"})
		return
	}

	monitor, err := h.repo.GetByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务监控不存在"})
		return
	}

	if h.engine == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "服务监控引擎不可用"})
		return
	}

	status, latency := h.engine.ProbeService(monitor)

	c.JSON(http.StatusOK, gin.H{
		"status":  status,
		"latency": latency,
	})
}

// HandleServiceMonitorStatuses 获取所有服务监控的当前状态
// 路由: GET /api/v1/service-monitors/statuses
func (h *ServiceMonitorHandler) HandleServiceMonitorStatuses(c *gin.Context) {
	if h.engine == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "服务监控引擎不可用"})
		return
	}

	statuses := h.engine.GetAllStatuses()
	c.JSON(http.StatusOK, gin.H{"statuses": statuses})
}
