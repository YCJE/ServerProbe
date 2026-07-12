package api

import (
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/server-probe/server/internal/repository"
)

// TrafficHandler 流量统计处理器
type TrafficHandler struct {
	trafficRepo *repository.TrafficRepository
}

// NewTrafficHandler 创建流量统计处理器
func NewTrafficHandler(trafficRepo *repository.TrafficRepository) *TrafficHandler {
	return &TrafficHandler{trafficRepo: trafficRepo}
}

// HandleGetTraffic 获取指定 Agent 的流量统计
// 路由: GET /api/v1/traffic/:agentId
// Query param range: "today" | "month" | "custom"
func (h *TrafficHandler) HandleGetTraffic(c *gin.Context) {
	agentIDStr := c.Param("agentId")
	agentID, err := strconv.ParseInt(agentIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 Agent ID"})
		return
	}

	rangeType := c.DefaultQuery("range", "today")

	switch rangeType {
	case "today":
		h.handleTodayTraffic(c, agentID)
	case "month":
		h.handleMonthTraffic(c, agentID)
	case "custom":
		h.handleCustomRangeTraffic(c, agentID)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 range 参数，支持: today, month, custom"})
	}
}

// handleTodayTraffic 返回当日流量
func (h *TrafficHandler) handleTodayTraffic(c *gin.Context, agentID int64) {
	today := time.Now().Format("2006-01-02")
	record, err := h.trafficRepo.GetDailyTraffic(agentID, today)
	if err != nil {
		log.Printf("[API] 获取 Agent %d 当日流量失败: %v", agentID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取流量数据失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"agent_id": agentID,
		"date":     today,
		"traffic":  record,
	})
}

// handleMonthTraffic 返回当月每日流量明细及汇总
func (h *TrafficHandler) handleMonthTraffic(c *gin.Context, agentID int64) {
	now := time.Now()
	year := now.Year()
	month := int(now.Month())

	records, err := h.trafficRepo.GetMonthlyTraffic(agentID, year, month)
	if err != nil {
		log.Printf("[API] 获取 Agent %d 当月流量失败: %v", agentID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取流量数据失败"})
		return
	}

	var totalRX, totalTX uint64
	for _, r := range records {
		totalRX += r.RXBytes
		totalTX += r.TXBytes
	}

	c.JSON(http.StatusOK, gin.H{
		"agent_id": agentID,
		"year":     year,
		"month":    month,
		"records":  records,
		"total": gin.H{
			"rx_bytes": totalRX,
			"tx_bytes": totalTX,
		},
	})
}

// handleCustomRangeTraffic 返回自定义日期范围内的流量明细及汇总
func (h *TrafficHandler) handleCustomRangeTraffic(c *gin.Context, agentID int64) {
	startDate := c.Query("start")
	endDate := c.Query("end")

	if startDate == "" || endDate == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "自定义范围需要 start 和 end 参数 (格式: 2006-01-02)"})
		return
	}

	// 校验日期格式
	if _, err := time.Parse("2006-01-02", startDate); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "start 日期格式无效，应为 2006-01-02"})
		return
	}
	if _, err := time.Parse("2006-01-02", endDate); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "end 日期格式无效，应为 2006-01-02"})
		return
	}
	if startDate > endDate {
		c.JSON(http.StatusBadRequest, gin.H{"error": "start 日期不能晚于 end 日期"})
		return
	}

	// 限制日期范围最大 366 天，防止查询过大范围导致内存消耗
	parsedStart, _ := time.Parse("2006-01-02", startDate)
	parsedEnd, _ := time.Parse("2006-01-02", endDate)
	if parsedEnd.Sub(parsedStart).Hours()/24 > 366 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "日期范围不能超过 366 天"})
		return
	}

	records, err := h.trafficRepo.GetTrafficByDateRange(agentID, startDate, endDate)
	if err != nil {
		log.Printf("[API] 获取 Agent %d 日期范围流量失败: %v", agentID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取流量数据失败"})
		return
	}

	var totalRX, totalTX uint64
	for _, r := range records {
		totalRX += r.RXBytes
		totalTX += r.TXBytes
	}

	c.JSON(http.StatusOK, gin.H{
		"agent_id": agentID,
		"start":    startDate,
		"end":      endDate,
		"records":  records,
		"total": gin.H{
			"rx_bytes": totalRX,
			"tx_bytes": totalTX,
		},
	})
}

// HandleGetAllTraffic 获取所有 Agent 当日流量
// 路由: GET /api/v1/traffic
func (h *TrafficHandler) HandleGetAllTraffic(c *gin.Context) {
	today := time.Now().Format("2006-01-02")
	records, err := h.trafficRepo.GetAllTrafficForDate(today)
	if err != nil {
		log.Printf("[API] 获取所有 Agent 当日流量失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取流量数据失败"})
		return
	}

	var totalRX, totalTX uint64
	for _, r := range records {
		totalRX += r.RXBytes
		totalTX += r.TXBytes
	}

	c.JSON(http.StatusOK, gin.H{
		"date":    today,
		"traffic": records,
		"total": gin.H{
			"rx_bytes": totalRX,
			"tx_bytes": totalTX,
		},
	})
}
