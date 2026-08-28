package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/server-probe/server/internal/repository"
)

// AuditHandler 审计日志查询处理器
type AuditHandler struct {
	repo *repository.AuditLogRepository
}

// NewAuditHandler 创建审计日志处理器
func NewAuditHandler(repo *repository.AuditLogRepository) *AuditHandler {
	return &AuditHandler{repo: repo}
}

// HandleListAuditLogs 分页查询审计日志
// 路由: GET /api/v1/audit-logs?username=&action=&success=&page=&page_size=
func (h *AuditHandler) HandleListAuditLogs(c *gin.Context) {
	q := repository.AuditLogQuery{
		Username: c.Query("username"),
		Action:   c.Query("action"),
	}

	if s := c.Query("success"); s != "" {
		v := s == "true" || s == "1"
		q.Success = &v
	}

	if p, err := strconv.Atoi(c.DefaultQuery("page", "1")); err == nil {
		q.Page = p
	}
	if p, err := strconv.Atoi(c.DefaultQuery("page_size", "50")); err == nil {
		q.PageSize = p
	}

	logs, total, err := h.repo.List(q)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询审计日志失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"logs":      logs,
		"total":     total,
		"page":      q.Page,
		"page_size": q.PageSize,
	})
}
