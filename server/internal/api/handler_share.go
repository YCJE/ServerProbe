package api

import (
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/server-probe/server/internal/model"
	"github.com/server-probe/server/internal/repository"
)

// ShareHandler 分享页处理器
type ShareHandler struct {
	repo *repository.SharePageRepository
}

// NewShareHandler 创建分享页处理器
func NewShareHandler(repo *repository.SharePageRepository) *ShareHandler {
	return &ShareHandler{repo: repo}
}

// HandleListSharePages 获取分享页列表
// 路由: GET /api/v1/share-pages
func (h *ShareHandler) HandleListSharePages(c *gin.Context) {
	pages, err := h.repo.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取分享页列表失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"pages": pages})
}

// HandleCreateSharePage 创建分享页
// 路由: POST /api/v1/share-pages
func (h *ShareHandler) HandleCreateSharePage(c *gin.Context) {
	var req struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		AgentIDs    string `json:"agent_ids"`
		Enabled     bool   `json:"enabled"`
		SortOrder   int    `json:"sort_order"`
		ShareID     string `json:"share_id"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求体"})
		return
	}

	if req.Title == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "标题不能为空"})
		return
	}
	if len(req.Title) > 200 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "标题过长"})
		return
	}

	// 如果未提供 share_id，自动生成；如果提供了，校验长度和字符集
	shareID := req.ShareID
	if shareID == "" {
		shareID = repository.GenerateShareID()
	} else {
		// 校验用户提供的 share_id: 长度 4-32，仅允许字母数字
		if len(shareID) < 4 || len(shareID) > 32 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "share_id 长度必须在 4-32 个字符之间"})
			return
		}
		for _, ch := range shareID {
			if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9')) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "share_id 仅允许字母和数字"})
				return
			}
		}
	}

	page := &model.SharePage{
		ShareID:     shareID,
		Title:       req.Title,
		Description: req.Description,
		AgentIDs:    req.AgentIDs,
		Enabled:     req.Enabled,
		SortOrder:   req.SortOrder,
	}

	if err := h.repo.Create(page); err != nil {
		log.Printf("[API] 创建分享页失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建分享页失败"})
		return
	}

	// GORM v2 对有 default tag 且字段值为零值(false)的字段会在 INSERT 中省略，
	// 让数据库使用 DEFAULT 值(true)。用户显式指定 enabled=false 时，需 Create 后用 Select 强制覆盖。
	if !req.Enabled {
		if err := h.repo.UpdateEnabled(page, false); err != nil {
			log.Printf("[API] 更新分享页 enabled 字段失败: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "创建分享页成功但禁用状态更新失败"})
			return
		}
		page.Enabled = false
	}

	c.JSON(http.StatusOK, gin.H{"page": page})
}

// HandlePublicSharePage 公开端点：验证 shareId 是否有效并返回分享页基本信息
// 路由: GET /api/v1/public/share/:shareId
func (h *ShareHandler) HandlePublicSharePage(c *gin.Context) {
	shareID := c.Param("shareId")
	if shareID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 shareId"})
		return
	}

	page, err := h.repo.GetByShareID(shareID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "分享页不存在"})
		return
	}

	if !page.Enabled {
		c.JSON(http.StatusNotFound, gin.H{"error": "分享页已禁用"})
		return
	}

	// 仅返回公开安全字段，不返回管理信息
	c.JSON(http.StatusOK, gin.H{
		"share_id":     page.ShareID,
		"title":        page.Title,
		"description":  page.Description,
		"agent_ids":    page.AgentIDs,
	})
}

// HandleUpdateSharePage 更新分享页
// 路由: PUT /api/v1/share-pages/:id
func (h *ShareHandler) HandleUpdateSharePage(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的分享页 ID"})
		return
	}

	page, err := h.repo.GetByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "分享页不存在"})
		return
	}

	var req struct {
		Title       *string `json:"title"`
		Description *string `json:"description"`
		AgentIDs    *string `json:"agent_ids"`
		Enabled     *bool   `json:"enabled"`
		SortOrder   *int    `json:"sort_order"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求体"})
		return
	}

	if req.Title != nil {
		if *req.Title == "" || len(*req.Title) > 200 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "标题无效"})
			return
		}
		page.Title = *req.Title
	}
	if req.Description != nil {
		page.Description = *req.Description
	}
	if req.AgentIDs != nil {
		page.AgentIDs = *req.AgentIDs
	}
	if req.Enabled != nil {
		page.Enabled = *req.Enabled
	}
	if req.SortOrder != nil {
		page.SortOrder = *req.SortOrder
	}

	if err := h.repo.Update(page); err != nil {
		log.Printf("[API] 更新分享页失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新分享页失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"page": page})
}

// HandleDeleteSharePage 删除分享页
// 路由: DELETE /api/v1/share-pages/:id
func (h *ShareHandler) HandleDeleteSharePage(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的分享页 ID"})
		return
	}

	if err := h.repo.Delete(id); err != nil {
		log.Printf("[API] 删除分享页失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除分享页失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// HandleGetSharePage 获取单个分享页
// 路由: GET /api/v1/share-pages/:id
func (h *ShareHandler) HandleGetSharePage(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的分享页 ID"})
		return
	}

	page, err := h.repo.GetByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "分享页不存在"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"page": page})
}
