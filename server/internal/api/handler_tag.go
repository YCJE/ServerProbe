package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/server-probe/server/internal/model"
	"github.com/server-probe/server/internal/repository"
)

// TagHandler 标签管理处理器
type TagHandler struct {
	repo *repository.TagRepository
}

// NewTagHandler 创建标签管理处理器
func NewTagHandler(repo *repository.TagRepository) *TagHandler {
	return &TagHandler{repo: repo}
}

// isValidTagColor 校验标签颜色为 #RRGGBB 格式
func isValidTagColor(color string) bool {
	if len(color) != 7 || color[0] != '#' {
		return false
	}
	for _, ch := range color[1:] {
		switch {
		case ch >= '0' && ch <= '9':
		case ch >= 'a' && ch <= 'f':
		case ch >= 'A' && ch <= 'F':
		default:
			return false
		}
	}
	return true
}

// HandleListTags 获取标签列表
// 路由: GET /api/v1/tags
func (h *TagHandler) HandleListTags(c *gin.Context) {
	tags, err := h.repo.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取标签失败"})
		return
	}
	if tags == nil {
		tags = []model.Tag{}
	}
	c.JSON(http.StatusOK, gin.H{"tags": tags})
}

// HandleCreateTag 创建标签
// 路由: POST /api/v1/tags
func (h *TagHandler) HandleCreateTag(c *gin.Context) {
	var req struct {
		Name  string `json:"name"`
		Color string `json:"color"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求体"})
		return
	}

	if req.Name == "" || len(req.Name) > 32 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "标签名称不能为空且不超过 32 个字符"})
		return
	}
	if req.Color == "" {
		req.Color = "#3b82f6"
	}
	if !isValidTagColor(req.Color) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "颜色必须是 #RRGGBB 格式"})
		return
	}

	tag := &model.Tag{Name: req.Name, Color: req.Color}
	if err := h.repo.Create(tag); err != nil {
		if errors.Is(err, repository.ErrTagAlreadyExists) {
			c.JSON(http.StatusConflict, gin.H{"error": "同名标签已存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建标签失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"tag": tag})
}

// HandleUpdateTag 更新标签
// 路由: PUT /api/v1/tags/:id
func (h *TagHandler) HandleUpdateTag(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的标签 ID"})
		return
	}

	tag, err := h.repo.GetByID(id)
	if err != nil {
		if errors.Is(err, repository.ErrTagNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "标签不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询标签失败"})
		return
	}

	var req struct {
		Name  *string `json:"name"`
		Color *string `json:"color"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求体"})
		return
	}

	if req.Name != nil {
		if *req.Name == "" || len(*req.Name) > 32 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "标签名称不能为空且不超过 32 个字符"})
			return
		}
		tag.Name = *req.Name
	}
	if req.Color != nil {
		if !isValidTagColor(*req.Color) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "颜色必须是 #RRGGBB 格式"})
			return
		}
		tag.Color = *req.Color
	}

	if err := h.repo.Update(tag); err != nil {
		if errors.Is(err, repository.ErrTagAlreadyExists) {
			c.JSON(http.StatusConflict, gin.H{"error": "同名标签已存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新标签失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"tag": tag})
}

// HandleDeleteTag 删除标签
// 路由: DELETE /api/v1/tags/:id
func (h *TagHandler) HandleDeleteTag(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的标签 ID"})
		return
	}

	if _, err := h.repo.GetByID(id); err != nil {
		if errors.Is(err, repository.ErrTagNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "标签不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询标签失败"})
		return
	}

	if err := h.repo.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除标签失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}
