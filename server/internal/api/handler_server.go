package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/server-probe/server/internal/repository"
	"github.com/server-probe/server/internal/service"
	sharedmodel "github.com/server-probe/shared/model"
)

// ServerHandler 服务器信息处理器
type ServerHandler struct {
	agentRepo *repository.AgentRepository
	monitor   *service.MonitorService
	recordRepo *repository.RecordRepository
}

// NewServerHandler 创建服务器处理器
func NewServerHandler(agentRepo *repository.AgentRepository, monitor *service.MonitorService, recordRepo *repository.RecordRepository) *ServerHandler {
	return &ServerHandler{
		agentRepo:  agentRepo,
		monitor:    monitor,
		recordRepo: recordRepo,
	}
}

// HandleListServers 获取服务器列表
// 路由: GET /api/v1/servers
func (h *ServerHandler) HandleListServers(c *gin.Context) {
	agents, err := h.agentRepo.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取服务器列表失败"})
		return
	}

	type ServerListItem struct {
		ID           int64       `json:"id"`
		Hostname     string      `json:"hostname"`
		DisplayName  string      `json:"display_name"`
		OS           string      `json:"os"`
		Arch         string      `json:"arch"`
		AgentVersion string      `json:"agent_version"`
		Online       bool        `json:"online"`
		LastSeen     string      `json:"last_seen"`
		CPU          float64     `json:"cpu"`
		Mem          float64     `json:"mem"`
		MemTotal     uint64      `json:"mem_total"`
		MemUsed      uint64      `json:"mem_used"`
		NetRx        uint64      `json:"net_rx"`
		NetTx        uint64      `json:"net_tx"`
		Uptime       uint64      `json:"uptime"`
		Load1        float64     `json:"load_1"`
		DiskUsage    float64     `json:"disk_usage"`
		PingData     interface{} `json:"ping_data"`
	}

	items := make([]ServerListItem, 0, len(agents))
	for _, agent := range agents {
		item := ServerListItem{
			ID:           agent.ID,
			Hostname:     agent.Hostname,
			DisplayName:  agent.DisplayName,
			OS:           agent.OS,
			Arch:         agent.Arch,
			AgentVersion: agent.AgentVersion,
			Online:       h.monitor.IsOnline(agent.ID),
			LastSeen:     agent.LastSeen.Format(time.RFC3339),
		}

		// 获取实时数据
		if rb := h.monitor.GetRingBuffer(agent.ID); rb != nil {
			points := rb.Latest(1)
			if len(points) > 0 {
				p := points[0]
				item.CPU = p.CPU
				item.Mem = p.Mem
				item.MemTotal = p.MemTotal
				item.MemUsed = p.MemUsed
				item.NetRx = p.NetRx
				item.NetTx = p.NetTx
				item.Uptime = p.Uptime
				item.Load1 = p.Load1
				item.DiskUsage = calcDiskUsage(p.Disks)
				item.PingData = p.PingData
			}
		}

		items = append(items, item)
	}

	c.JSON(http.StatusOK, gin.H{"servers": items})
}

// calcDiskUsage 从磁盘分区列表计算使用率
// 优先取根分区（/），否则取最大分区的使用率
func calcDiskUsage(disks []sharedmodel.DiskInfo) float64 {
	if len(disks) == 0 {
		return 0
	}

	// 优先查找根分区
	for _, d := range disks {
		if d.Device == "/" && d.Total > 0 {
			return float64(d.Used) / float64(d.Total) * 100
		}
	}

	// 否则取最大分区
	var maxDisk *sharedmodel.DiskInfo
	for i := range disks {
		if disks[i].Total == 0 {
			continue
		}
		if maxDisk == nil || disks[i].Total > maxDisk.Total {
			maxDisk = &disks[i]
		}
	}

	if maxDisk != nil && maxDisk.Total > 0 {
		return float64(maxDisk.Used) / float64(maxDisk.Total) * 100
	}

	return 0
}

// HandleGetServer 获取单台服务器详情
// 路由: GET /api/v1/servers/:id
func (h *ServerHandler) HandleGetServer(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 ID"})
		return
	}

	agent, err := h.agentRepo.GetByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	// 获取实时数据
	var latestData *repository.MetricPoint
	if rb := h.monitor.GetRingBuffer(id); rb != nil {
		points := rb.Latest(1)
		if len(points) > 0 {
			latestData = &points[0]
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"agent":       agent,
		"online":      h.monitor.IsOnline(id),
		"latest_data": latestData,
	})
}

// HandleGetServerHistory 获取历史数据
// 路由: GET /api/v1/servers/:id/history?range=1h|6h|12h|1d|2d
func (h *ServerHandler) HandleGetServerHistory(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 ID"})
		return
	}

	rangeStr := c.DefaultQuery("range", "1h")

	var startTime int64
	now := time.Now().Unix()

	switch rangeStr {
	case "1h":
		startTime = now - 3600
	case "6h":
		startTime = now - 6*3600
	case "12h":
		startTime = now - 12*3600
	case "1d":
		startTime = now - 24*3600
	case "2d":
		startTime = now - 2*24*3600
	default:
		startTime = now - 3600
	}

	// 1h 和 6h 从环形缓冲读取
	if rangeStr == "1h" || rangeStr == "6h" {
		if rb := h.monitor.GetRingBuffer(id); rb != nil {
			points := rb.GetByTimeRange(startTime, now)
			c.JSON(http.StatusOK, gin.H{
				"source":  "ringbuffer",
				"records": points,
			})
			return
		}
	}

	// 12h+ 从 SQLite 读取
	records, err := h.recordRepo.GetByAgentAndTimeRange(id, startTime, now)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取历史数据失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"source":  "sqlite",
		"records": records,
	})
}

// HandleDashboard 获取仪表盘数据
// 路由: GET /api/v1/dashboard
func (h *ServerHandler) HandleDashboard(c *gin.Context) {
	items := h.monitor.GetDashboardData()
	c.JSON(http.StatusOK, gin.H{"servers": items})
}
