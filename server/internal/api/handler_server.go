package api

import (
	"encoding/json"
	"net/http"
	"runtime"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/server-probe/server/internal/repository"
	"github.com/server-probe/server/internal/service"
	sharedmodel "github.com/server-probe/shared/model"
)

var startTime = time.Now()

// historyPoint 历史数据点（统一响应格式，字段名与 model.MetricRecord 的 JSON tag 一致）
type historyPoint struct {
	Timestamp    int64                    `json:"timestamp"`
	CPUUsage     float64                  `json:"cpu_usage"`
	MemUsage     float64                  `json:"mem_usage"`
	MemTotal     uint64                   `json:"mem_total"`
	MemUsed      uint64                   `json:"mem_used"`
	SwapTotal    uint64                   `json:"swap_total"`
	SwapUsed     uint64                   `json:"swap_used"`
	DiskUsage    string                   `json:"disk_usage"`
	NetRx        uint64                   `json:"net_rx"`
	NetTx        uint64                   `json:"net_tx"`
	TCPConns     int                      `json:"tcp_connections"`
	UDPConns     int                      `json:"udp_connections"`
	Load1        float64                  `json:"load_1"`
	Load5        float64                  `json:"load_5"`
	Load15       float64                  `json:"load_15"`
	Uptime       uint64                   `json:"uptime"`
	ProcessCount int                      `json:"process_count"`
	PingData     []sharedmodel.PingResult `json:"ping_data"`
}

// metricPointToHistoryPoint 将 ringbuffer 的 MetricPoint 转换为统一的历史数据点响应格式
func metricPointToHistoryPoint(p repository.MetricPoint) historyPoint {
	hp := historyPoint{
		Timestamp:    p.Timestamp,
		CPUUsage:     p.CPU,
		MemUsage:     p.Mem,
		MemTotal:     p.MemTotal,
		MemUsed:      p.MemUsed,
		SwapTotal:    p.SwapTotal,
		SwapUsed:     p.SwapUsed,
		NetRx:        p.NetRx,
		NetTx:        p.NetTx,
		TCPConns:     p.TCPConns,
		UDPConns:     p.UDPConns,
		Load1:        p.Load1,
		Load5:        p.Load5,
		Load15:       p.Load15,
		Uptime:       p.Uptime,
		ProcessCount: p.ProcessCount,
		PingData:     p.PingData,
	}

	// 序列化磁盘数据为 JSON 字符串，与 MetricRecord.DiskUsage 格式保持一致
	if len(p.Disks) > 0 {
		if diskBytes, err := json.Marshal(p.Disks); err == nil {
			hp.DiskUsage = string(diskBytes)
		}
	}

	return hp
}

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
		ID            int64                    `json:"id"`
		Hostname      string                   `json:"hostname"`
		DisplayName   string                   `json:"display_name"`
		OS            string                   `json:"os"`
		Arch          string                   `json:"arch"`
		AgentVersion  string                   `json:"agent_version"`
		Online        bool                     `json:"online"`
		LastSeen      string                   `json:"last_seen"`
		CPU           float64                  `json:"cpu"`
		Mem           float64                  `json:"mem"`
		MemTotal      uint64                   `json:"mem_total"`
		MemUsed       uint64                   `json:"mem_used"`
		SwapTotal     uint64                   `json:"swap_total"`
		SwapUsed      uint64                   `json:"swap_used"`
		NetRx         uint64                   `json:"net_rx"`
		NetTx         uint64                   `json:"net_tx"`
		Uptime        uint64                   `json:"uptime"`
		Load1         float64                  `json:"load_1"`
		Load5         float64                  `json:"load_5"`
		Load15        float64                  `json:"load_15"`
		DiskUsage     float64                  `json:"disk_usage"`
		Disks         []sharedmodel.DiskInfo   `json:"disks"`
		TCPConns      int                      `json:"tcp_connections"`
		UDPConns      int                      `json:"udp_connections"`
		ProcessCount  int                      `json:"process_count"`
		PingData      []sharedmodel.PingResult `json:"ping_data"`
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
				item.SwapTotal = p.SwapTotal
				item.SwapUsed = p.SwapUsed
				item.NetRx = p.NetRx
				item.NetTx = p.NetTx
				item.Uptime = p.Uptime
				item.Load1 = p.Load1
				item.Load5 = p.Load5
				item.Load15 = p.Load15
				item.DiskUsage = calcDiskUsage(p.Disks)
				item.Disks = p.Disks
				item.TCPConns = p.TCPConns
				item.UDPConns = p.UDPConns
				item.ProcessCount = p.ProcessCount
				item.PingData = p.PingData
			}
		}

		items = append(items, item)
	}

	c.JSON(http.StatusOK, gin.H{"servers": items, "total": len(items)})
}

// calcDiskUsage 从磁盘信息计算使用率
// Agent 上报的是聚合后的总磁盘信息 (Device="total")
func calcDiskUsage(disks []sharedmodel.DiskInfo) float64 {
	if len(disks) == 0 {
		return 0
	}

	// 使用第一个磁盘条目 (Agent 聚合后的总磁盘)
	d := disks[0]
	if d.Total > 0 {
		return float64(d.Used) / float64(d.Total) * 100
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

	// 构建扁平化的响应，与前端 ServerData 类型匹配
	resp := gin.H{
		"id":             agent.ID,
		"hostname":       agent.Hostname,
		"display_name":   agent.DisplayName,
		"os":             agent.OS,
		"arch":           agent.Arch,
		"agent_version":  agent.AgentVersion,
		"online":         h.monitor.IsOnline(id),
		"last_seen":      agent.LastSeen.Unix(),
	}

	// 获取实时数据，补充监控字段
	if rb := h.monitor.GetRingBuffer(id); rb != nil {
		points := rb.Latest(1)
		if len(points) > 0 {
			p := points[0]
			resp["cpu"] = p.CPU
			resp["mem"] = p.Mem
			resp["mem_total"] = p.MemTotal
			resp["mem_used"] = p.MemUsed
			resp["swap_total"] = p.SwapTotal
			resp["swap_used"] = p.SwapUsed
			resp["net_rx"] = p.NetRx
			resp["net_tx"] = p.NetTx
			resp["load_1"] = p.Load1
			resp["load_5"] = p.Load5
			resp["load_15"] = p.Load15
			resp["uptime"] = p.Uptime
			resp["disk_usage"] = calcDiskUsage(p.Disks)
			resp["disks"] = p.Disks
			resp["tcp_connections"] = p.TCPConns
			resp["udp_connections"] = p.UDPConns
			resp["process_count"] = p.ProcessCount
			resp["ping_data"] = p.PingData
			resp["timestamp"] = p.Timestamp
		}
	}

	c.JSON(http.StatusOK, resp)
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
			// 将 MetricPoint 转换为统一的历史数据点格式
			historyPoints := make([]historyPoint, 0, len(points))
			for _, p := range points {
				historyPoints = append(historyPoints, metricPointToHistoryPoint(p))
			}
			c.JSON(http.StatusOK, gin.H{
				"source": "ringbuffer",
				"points": historyPoints,
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

	// 将 MetricRecord 转换为统一格式 (ping_data 从 string 解析为数组)
	historyPoints := make([]historyPoint, 0, len(records))
	for _, r := range records {
		hp := historyPoint{
			Timestamp:    r.Timestamp,
			CPUUsage:     r.CPUUsage,
			MemUsage:     r.MemUsage,
			MemTotal:     r.MemTotal,
			MemUsed:      r.MemUsed,
			SwapTotal:    r.SwapTotal,
			SwapUsed:     r.SwapUsed,
			DiskUsage:    r.DiskUsage,
			NetRx:        uint64(r.NetRx),
			NetTx:        uint64(r.NetTx),
			TCPConns:     r.TCPConns,
			UDPConns:     r.UDPConns,
			Load1:        r.Load1,
			Load5:        r.Load5,
			Load15:       r.Load15,
			Uptime:       r.Uptime,
			ProcessCount: r.ProcessCount,
		}
		// 解析 ping_data JSON 字符串为数组
		if r.PingData != "" {
			var pings []sharedmodel.PingResult
			if err := json.Unmarshal([]byte(r.PingData), &pings); err == nil {
				hp.PingData = pings
			}
		}
		historyPoints = append(historyPoints, hp)
	}

	c.JSON(http.StatusOK, gin.H{
		"source": "sqlite",
		"points": historyPoints,
	})
}

// HandleDashboard 获取仪表盘数据
// 路由: GET /api/v1/dashboard
func (h *ServerHandler) HandleDashboard(c *gin.Context) {
	items := h.monitor.GetDashboardData()
	c.JSON(http.StatusOK, gin.H{"servers": items})
}

// HandlePublicServers 公开服务器列表 (无需登录，仅返回非敏感信息)
// 路由: GET /api/v1/public/servers
func (h *ServerHandler) HandlePublicServers(c *gin.Context) {
	agents, err := h.agentRepo.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取服务器列表失败"})
		return
	}

	type PublicServerItem struct {
		ID          int64   `json:"id"`
		DisplayName string  `json:"display_name"`
		Hostname    string  `json:"hostname"`
		OS          string  `json:"os"`
		Online      bool    `json:"online"`
		CPU         float64 `json:"cpu"`
		Mem         float64 `json:"mem"`
		MemTotal    uint64  `json:"mem_total"`
		MemUsed     uint64  `json:"mem_used"`
		NetRx       uint64  `json:"net_rx"`
		NetTx       uint64  `json:"net_tx"`
		Uptime      uint64  `json:"uptime"`
		Load1       float64 `json:"load_1"`
		DiskUsage   float64 `json:"disk_usage"`
	}

	items := make([]PublicServerItem, 0, len(agents))
	for _, agent := range agents {
		item := PublicServerItem{
			ID:          agent.ID,
			DisplayName: agent.DisplayName,
			Hostname:    agent.Hostname,
			OS:          agent.OS,
			Online:      h.monitor.IsOnline(agent.ID),
		}

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
			}
		}

		items = append(items, item)
	}

	c.JSON(http.StatusOK, gin.H{"servers": items})
}

// HandlePublicDashboard 公开仪表盘数据 (无需登录)
// 路由: GET /api/v1/public/dashboard
func (h *ServerHandler) HandlePublicDashboard(c *gin.Context) {
	items := h.monitor.GetDashboardData()
	// 过滤敏感字段，只保留公开展示所需的数据
	type PublicDashboardItem struct {
		AgentID     int64                    `json:"agent_id"`
		Hostname    string                   `json:"hostname"`
		DisplayName string                   `json:"display_name"`
		Online      bool                     `json:"online"`
		CPU         float64                  `json:"cpu"`
		Mem         float64                  `json:"mem"`
		MemTotal    uint64                   `json:"mem_total"`
		MemUsed     uint64                   `json:"mem_used"`
		NetRx       uint64                   `json:"net_rx"`
		NetTx       uint64                   `json:"net_tx"`
		Load1       float64                  `json:"load_1"`
		Uptime      uint64                   `json:"uptime"`
		PingData    []sharedmodel.PingResult `json:"ping_data"`
		Timestamp   int64                    `json:"timestamp"`
	}

	publicItems := make([]PublicDashboardItem, 0, len(items))
	for _, item := range items {
		publicItems = append(publicItems, PublicDashboardItem{
			AgentID:     item.AgentID,
			Hostname:    item.Hostname,
			DisplayName: item.DisplayName,
			Online:      item.Online,
			CPU:         item.CPU,
			Mem:         item.Mem,
			MemTotal:    item.MemTotal,
			MemUsed:     item.MemUsed,
			NetRx:       item.NetRx,
			NetTx:       item.NetTx,
			Load1:       item.Load1,
			Uptime:      item.Uptime,
			PingData:    item.PingData,
			Timestamp:   item.Timestamp,
		})
	}

	c.JSON(http.StatusOK, gin.H{"servers": publicItems})
}

// HandleSystemStatus 获取系统状态
// 路由: GET /api/v1/system/status
func (h *ServerHandler) HandleSystemStatus(c *gin.Context) {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// 在线 Agent 数
	onlineCount := h.monitor.GetOnlineAgentCount()

	// WebSocket 面板连接数
	wsConnCount := h.monitor.GetDashboardWSCount()

	// 数据库文件大小
	var dbSize int64
	if h.recordRepo != nil {
		dbSize = h.recordRepo.GetDBSize()
	}

	// 磁盘剩余空间 (数据目录)
	var diskFree uint64
	var diskTotal uint64
	diskFree, diskTotal = getDiskSpace(h.monitor.GetDataDir())

	c.JSON(http.StatusOK, gin.H{
		"uptime":           int64(time.Since(startTime).Seconds()),
		"mem_alloc":        memStats.Alloc,
		"mem_sys":          memStats.Sys,
		"mem_num_gc":       memStats.NumGC,
		"db_size":          dbSize,
		"online_agents":    onlineCount,
		"ws_connections":   wsConnCount,
		"goroutines":       runtime.NumGoroutine(),
		"disk_total":       diskTotal,
		"disk_free":        diskFree,
		"version":          "1.0.0",
	})
}
