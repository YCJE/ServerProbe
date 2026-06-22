package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/server-probe/server/internal/model"
	"github.com/server-probe/server/internal/repository"
)

// PingTargetHandler 探测目标处理器
type PingTargetHandler struct {
	repo *repository.PingTargetRepository
}

// NewPingTargetHandler 创建探测目标处理器
func NewPingTargetHandler(repo *repository.PingTargetRepository) *PingTargetHandler {
	return &PingTargetHandler{repo: repo}
}

// HandleListPingTargets 列出所有探测目标
// 路由: GET /api/v1/ping-targets
func (h *PingTargetHandler) HandleListPingTargets(c *gin.Context) {
	targets, err := h.repo.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取探测目标列表失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"targets": targets})
}

// HandleCreatePingTarget 创建探测目标
// 路由: POST /api/v1/ping-targets
func (h *PingTargetHandler) HandleCreatePingTarget(c *gin.Context) {
	var req struct {
		Name      string `json:"name"`
		Target    string `json:"target"`
		Method    string `json:"method"`
		Enabled   *bool  `json:"enabled"`
		SortOrder *int   `json:"sort_order"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求体"})
		return
	}

	if req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "名称不能为空"})
		return
	}
	if req.Target == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "目标地址不能为空"})
		return
	}
	if len(req.Name) > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "名称过长"})
		return
	}
	if len(req.Target) > 255 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "目标地址过长"})
		return
	}

	// 验证探测方式
	validMethods := map[string]bool{"icmp": true, "tcp": true, "http": true, "": true}
	if !validMethods[req.Method] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "探测方式无效，支持: icmp, tcp, http"})
		return
	}

	target := &model.PingTarget{
		Name:      req.Name,
		Target:    req.Target,
		Method:    req.Method,
		Enabled:   true,
		SortOrder: 0,
	}

	// 设置默认值
	if target.Method == "" {
		target.Method = "icmp"
	}
	if req.Enabled != nil {
		target.Enabled = *req.Enabled
	}
	if req.SortOrder != nil {
		target.SortOrder = *req.SortOrder
	}

	if err := h.repo.Create(target); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建探测目标失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"target": target})
}

// HandleUpdatePingTarget 更新探测目标
// 路由: PUT /api/v1/ping-targets/:id
func (h *PingTargetHandler) HandleUpdatePingTarget(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 ID"})
		return
	}

	// 先获取现有记录
	existing, err := h.repo.GetByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "探测目标不存在"})
		return
	}

	var req struct {
		Name      *string `json:"name"`
		Target    *string `json:"target"`
		Method    *string `json:"method"`
		Enabled   *bool   `json:"enabled"`
		SortOrder *int    `json:"sort_order"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求体"})
		return
	}

	// 按需更新字段
	if req.Name != nil {
		if *req.Name == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "名称不能为空"})
			return
		}
		existing.Name = *req.Name
	}
	if req.Target != nil {
		if *req.Target == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "目标地址不能为空"})
			return
		}
		existing.Target = *req.Target
	}
	if req.Method != nil {
		existing.Method = *req.Method
	}
	if req.Enabled != nil {
		existing.Enabled = *req.Enabled
	}
	if req.SortOrder != nil {
		existing.SortOrder = *req.SortOrder
	}

	if err := h.repo.Update(existing); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新探测目标失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"target": existing})
}

// HandleDeletePingTarget 删除探测目标
// 路由: DELETE /api/v1/ping-targets/:id
func (h *PingTargetHandler) HandleDeletePingTarget(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 ID"})
		return
	}

	if err := h.repo.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除探测目标失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}
