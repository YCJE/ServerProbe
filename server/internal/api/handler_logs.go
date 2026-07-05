package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/server-probe/server/internal/service"
)

// LogHandler 日志查看处理器
type LogHandler struct {
	logCapture *service.LogCapture
}

// NewLogHandler 创建日志处理器
func NewLogHandler(logCapture *service.LogCapture) *LogHandler {
	return &LogHandler{logCapture: logCapture}
}

// HandleGetLogs 获取日志列表
// 路由: GET /api/v1/logs?level=all&limit=200&search=keyword
func (h *LogHandler) HandleGetLogs(c *gin.Context) {
	level := c.DefaultQuery("level", "ALL")
	limitStr := c.DefaultQuery("limit", "200")
	search := c.Query("search")

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}

	logs := h.logCapture.GetLogs(level, limit, search)

	c.JSON(http.StatusOK, gin.H{
		"logs":  logs,
		"total": h.logCapture.GetLogCount(),
	})
}
