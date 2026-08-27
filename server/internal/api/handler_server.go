package api

import (
	"encoding/json"
	"math"
	"net/http"
	"runtime"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/server-probe/server/internal/model"
	"github.com/server-probe/server/internal/repository"
	"github.com/server-probe/server/internal/service"
	sharedmodel "github.com/server-probe/shared/model"
)

var startTime = time.Now()

// historyPoint 历史数据点（统一响应格式，字段名与 model.MetricRecord 的 JSON tag 一致）
type historyPoint struct {
	Timestamp    int64                    `json:"timestamp"`
	CPUUsage     float64                  `json:"cpu_usage"`
	CPUModel     string                   `json:"cpu_model"`
	CPUCores     int                      `json:"cpu_cores"`
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
	// Online 在线状态（1=在线, 0=离线占位记录），用于在线率时间线
	Online int `json:"online"`
}

// ServerHandler 服务器信息处理器
type ServerHandler struct {
	agentRepo  *repository.AgentRepository
	monitor    *service.MonitorService
	recordRepo *repository.RecordRepository
	settings   *service.SettingsService // 可选：图表最大点数等运行时设置
}

// NewServerHandler 创建服务器处理器
func NewServerHandler(agentRepo *repository.AgentRepository, monitor *service.MonitorService, recordRepo *repository.RecordRepository) *ServerHandler {
	return &ServerHandler{
		agentRepo:  agentRepo,
		monitor:    monitor,
		recordRepo: recordRepo,
	}
}

// SetSettings 注入系统设置服务（图表降采样上限可后台调整）
func (h *ServerHandler) SetSettings(s *service.SettingsService) {
	h.settings = s
}

// maxChartPoints 读取图表最大点数（未注入设置服务时使用默认值）
func (h *ServerHandler) maxChartPoints() int {
	if h.settings != nil {
		return h.settings.MaxChartPoints()
	}
	return 800
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
		ID           int64                    `json:"id"`
		Hostname     string                   `json:"hostname"`
		DisplayName  string                   `json:"display_name"`
		OS           string                   `json:"os"`
		Arch         string                   `json:"arch"`
		AgentVersion string                   `json:"agent_version"`
		Online       bool                     `json:"online"`
		LastSeen     string                   `json:"last_seen"`
		CPU          float64                  `json:"cpu"`
		Mem          float64                  `json:"mem"`
		MemTotal     uint64                   `json:"mem_total"`
		MemUsed      uint64                   `json:"mem_used"`
		SwapTotal    uint64                   `json:"swap_total"`
		SwapUsed     uint64                   `json:"swap_used"`
		NetRx        uint64                   `json:"net_rx"`
		NetTx        uint64                   `json:"net_tx"`
		Uptime       uint64                   `json:"uptime"`
		CPUModel     string                   `json:"cpu_model"`
		CPUCores     int                      `json:"cpu_cores"`
		TotalRx      uint64                   `json:"total_rx"`
		TotalTx      uint64                   `json:"total_tx"`
		Load1        float64                  `json:"load_1"`
		Load5        float64                  `json:"load_5"`
		Load15       float64                  `json:"load_15"`
		DiskUsage    float64                  `json:"disk_usage"`
		Disks        []sharedmodel.DiskInfo   `json:"disks"`
		TCPConns     int                      `json:"tcp_connections"`
		UDPConns     int                      `json:"udp_connections"`
		ProcessCount int                      `json:"process_count"`
		PingData     []sharedmodel.PingResult `json:"ping_data"`
		// NodeGet 风格元数据
		Tags              string     `json:"tags"`
		Region            string     `json:"region"`
		CountryCode       string     `json:"country_code"`
		ISP               string     `json:"isp"`
		ExpiresAt         *time.Time `json:"expires_at"`
		ExpiresInDays     *int       `json:"expires_in_days"`
		PriceAmount       float64    `json:"price_amount"`
		PriceCurrency     string     `json:"price_currency"`
		PriceCycle        string     `json:"price_cycle"`
		TrafficQuotaBytes int64      `json:"traffic_quota_bytes"`
		MonthlyRx         uint64     `json:"monthly_rx"`
		MonthlyTx         uint64     `json:"monthly_tx"`
		IPv4              string     `json:"ipv4"`
		IPv6              string     `json:"ipv6"`
	}

	monthlyTraffic := h.monitor.GetMonthlyTraffic()
	now := time.Now()

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
			Tags:              agent.Tags,
			Region:            agent.Region,
			CountryCode:       agent.CountryCode,
			ISP:               agent.ISP,
			ExpiresAt:         agent.ExpiresAt,
			ExpiresInDays:     calcExpiresInDays(agent.ExpiresAt, now),
			PriceAmount:       agent.PriceAmount,
			PriceCurrency:     agent.PriceCurrency,
			PriceCycle:        agent.PriceCycle,
			TrafficQuotaBytes: agent.TrafficQuotaBytes,
			IPv4:              agent.IPv4,
			IPv6:              agent.IPv6,
		}
		if monthlyTraffic != nil {
			if agg, ok := monthlyTraffic[agent.ID]; ok {
				item.MonthlyRx = agg.Rx
				item.MonthlyTx = agg.Tx
			}
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
				item.CPUModel = p.CPUModel
				item.CPUCores = p.CPUCores
				item.TotalRx = p.TotalRx
				item.TotalTx = p.TotalTx
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
// 与 monitor.go 中的 calcDiskUsage 保持一致: 汇总所有磁盘
func calcDiskUsage(disks []sharedmodel.DiskInfo) float64 {
	if len(disks) == 0 {
		return 0
	}

	var totalTotal, totalUsed uint64
	for _, d := range disks {
		totalTotal += d.Total
		totalUsed += d.Used
	}

	if totalTotal > 0 {
		return float64(totalUsed) / float64(totalTotal) * 100
	}

	return 0
}

// calcExpiresInDays 计算距到期的剩余天数（向上取整），nil 返回 nil
// 与 service 层同名函数逻辑一致，避免为单一用途跨包导出
func calcExpiresInDays(expiresAt *time.Time, now time.Time) *int {
	if expiresAt == nil {
		return nil
	}
	days := int(math.Ceil(expiresAt.Sub(now).Hours() / 24))
	if days < 0 {
		days = 0
	}
	return &days
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
		"id":            agent.ID,
		"hostname":      agent.Hostname,
		"display_name":  agent.DisplayName,
		"os":            agent.OS,
		"arch":          agent.Arch,
		"agent_version": agent.AgentVersion,
		"online":        h.monitor.IsOnline(id),
		"last_seen":     agent.LastSeen.Unix(),
		// NodeGet 风格元数据
		"tags":               agent.Tags,
		"region":             agent.Region,
		"country_code":       agent.CountryCode,
		"isp":                agent.ISP,
		"expires_at":         agent.ExpiresAt,
		"expires_in_days":    calcExpiresInDays(agent.ExpiresAt, time.Now()),
		"price_amount":       agent.PriceAmount,
		"price_currency":     agent.PriceCurrency,
		"price_cycle":        agent.PriceCycle,
		"traffic_quota_bytes": agent.TrafficQuotaBytes,
		"ipv4":               agent.IPv4,
		"ipv6":               agent.IPv6,
	}

	// 月度流量合计
	if agg, ok := h.monitor.GetMonthlyTraffic()[id]; ok {
		resp["monthly_rx"] = agg.Rx
		resp["monthly_tx"] = agg.Tx
	} else {
		resp["monthly_rx"] = 0
		resp["monthly_tx"] = 0
	}

	// 获取实时数据，补充监控字段
	if rb := h.monitor.GetRingBuffer(id); rb != nil {
		points := rb.Latest(1)
		if len(points) > 0 {
			p := points[0]
			resp["cpu"] = p.CPU
			resp["cpu_model"] = p.CPUModel
			resp["cpu_cores"] = p.CPUCores
			resp["mem"] = p.Mem
			resp["mem_total"] = p.MemTotal
			resp["mem_used"] = p.MemUsed
			resp["swap_total"] = p.SwapTotal
			resp["swap_used"] = p.SwapUsed
			resp["net_rx"] = p.NetRx
			resp["net_tx"] = p.NetTx
			resp["total_rx"] = p.TotalRx
			resp["total_tx"] = p.TotalTx
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
	case "3d":
		startTime = now - 3*24*3600
	default:
		startTime = now - 3600
	}

	// 所有历史范围均从 SQLite 读取（聚合数据，每5分钟一条，数据稳定可靠）
	// RingBuffer 仅用于实时模式（前端 WebSocket 推送）
	records, err := h.recordRepo.GetByAgentAndTimeRange(id, startTime, now)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取历史数据失败"})
		return
	}

	// 降采样保护：点数超过上限时均匀抽稀，防止大范围查询返回过多数据导致前端渲染卡顿
	// （保留首尾点，确保时间边界准确；离线占位记录同样参与抽稀）
	maxHistoryPoints := h.maxChartPoints()
	if len(records) > maxHistoryPoints {
		sampled := make([]model.MetricRecord, 0, maxHistoryPoints+2)
		step := float64(len(records)-1) / float64(maxHistoryPoints-1)
		lastIdx := -1
		for i := 0; i < maxHistoryPoints; i++ {
			idx := int(math.Round(float64(i) * step))
			if idx > lastIdx {
				sampled = append(sampled, records[idx])
				lastIdx = idx
			}
		}
		if lastIdx < len(records)-1 {
			sampled = append(sampled, records[len(records)-1])
		}
		records = sampled
	}

	// 将 MetricRecord 转换为统一格式 (ping_data 从 string 解析为数组)
	// P3: CPUUsage / Load 字段以 ×10 整数存储，查询时除以 10.0 还原为浮点数
	historyPoints := make([]historyPoint, 0, len(records))
	for _, r := range records {
		hp := historyPoint{
			Timestamp:    r.Timestamp,
			CPUUsage:     float64(r.CPUUsage) / 10.0,
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
			Load1:        float64(r.Load1) / 10.0,
			Load5:        float64(r.Load5) / 10.0,
			Load15:       float64(r.Load15) / 10.0,
			Uptime:       r.Uptime,
			ProcessCount: r.ProcessCount,
			Online:       1 - r.Offline,
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

// publicHistoryPoint 公开历史数据点（过滤敏感字段）
type publicHistoryPoint struct {
	Timestamp int64                    `json:"timestamp"`
	CPUUsage  float64                  `json:"cpu_usage"`
	MemUsage  float64                  `json:"mem_usage"`
	MemTotal  uint64                   `json:"mem_total"`
	MemUsed   uint64                   `json:"mem_used"`
	SwapTotal uint64                   `json:"swap_total"`
	SwapUsed  uint64                   `json:"swap_used"`
	DiskUsage float64                  `json:"disk_usage"` // 仅百分比，不暴露磁盘设备信息
	NetRx     uint64                   `json:"net_rx"`
	NetTx     uint64                   `json:"net_tx"`
	Load1     float64                  `json:"load_1"`
	Load5     float64                  `json:"load_5"`
	Load15    float64                  `json:"load_15"`
	Uptime    uint64                   `json:"uptime"`
	PingData  []sharedmodel.PingResult `json:"ping_data"`
	Online    int                      `json:"online"`
}

// toPublicHistoryPoint 将 historyPoint 转换为公开版本，过滤敏感字段
// 过滤: tcp_connections, udp_connections, process_count, disk设备信息, ping_data 中的 target 字段
func toPublicHistoryPoint(hp historyPoint) publicHistoryPoint {
	php := publicHistoryPoint{
		Timestamp: hp.Timestamp,
		CPUUsage:  hp.CPUUsage,
		MemUsage:  hp.MemUsage,
		MemTotal:  hp.MemTotal,
		MemUsed:   hp.MemUsed,
		SwapTotal: hp.SwapTotal,
		SwapUsed:  hp.SwapUsed,
		DiskUsage: 0, // 从原始磁盘数据计算聚合百分比
		NetRx:     hp.NetRx,
		NetTx:     hp.NetTx,
		Load1:     hp.Load1,
		Load5:     hp.Load5,
		Load15:    hp.Load15,
		Uptime:    hp.Uptime,
		Online:    hp.Online,
	}

	// 从原始磁盘 JSON 计算聚合使用率百分比（不暴露磁盘设备信息）
	if hp.DiskUsage != "" {
		var disks []sharedmodel.DiskInfo
		if err := json.Unmarshal([]byte(hp.DiskUsage), &disks); err == nil {
			php.DiskUsage = calcDiskUsage(disks)
		}
	}

	// 过滤 PingData 中的 Target 字段（敏感信息）
	if len(hp.PingData) > 0 {
		php.PingData = make([]sharedmodel.PingResult, 0, len(hp.PingData))
		for _, p := range hp.PingData {
			php.PingData = append(php.PingData, sharedmodel.PingResult{
				Name:        p.Name,
				Method:      p.Method,
				AvgLatency:  p.AvgLatency,
				MinLatency:  p.MinLatency,
				MaxLatency:  p.MaxLatency,
				Jitter:      p.Jitter,
				Loss:        p.Loss,
				PacketsSent: p.PacketsSent,
				PacketsRecv: p.PacketsRecv,
				IPVersion:   p.IPVersion,
				// Target 字段不包含，防止泄露探测目标地址
			})
		}
	}

	return php
}

// HandlePublicServerHistory 公开历史数据 (无需登录，过滤敏感字段)
// 路由: GET /api/v1/public/servers/:id/history?range=1h|6h|12h|1d|2d
func (h *ServerHandler) HandlePublicServerHistory(c *gin.Context) {
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
	case "3d":
		startTime = now - 3*24*3600
	default:
		startTime = now - 3600
	}

	// 所有历史范围均从 SQLite 读取（聚合数据，每5分钟一条，数据稳定可靠）
	records, err := h.recordRepo.GetByAgentAndTimeRange(id, startTime, now)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取历史数据失败"})
		return
	}

	// 降采样保护：与管理端一致，防止点数过多导致公开页渲染卡顿
	const maxHistoryPoints = 800
	if len(records) > maxHistoryPoints {
		sampled := make([]model.MetricRecord, 0, maxHistoryPoints+2)
		step := float64(len(records)-1) / float64(maxHistoryPoints-1)
		lastIdx := -1
		for i := 0; i < maxHistoryPoints; i++ {
			idx := int(math.Round(float64(i) * step))
			if idx > lastIdx {
				sampled = append(sampled, records[idx])
				lastIdx = idx
			}
		}
		if lastIdx < len(records)-1 {
			sampled = append(sampled, records[len(records)-1])
		}
		records = sampled
	}

	publicPoints := make([]publicHistoryPoint, 0, len(records))
	for _, r := range records {
		// P3: CPUUsage / Load 字段以 ×10 整数存储，查询时除以 10.0 还原为浮点数
		hp := historyPoint{
			Timestamp: r.Timestamp,
			CPUUsage:  float64(r.CPUUsage) / 10.0,
			MemUsage:  r.MemUsage,
			MemTotal:  r.MemTotal,
			MemUsed:   r.MemUsed,
			SwapTotal: r.SwapTotal,
			SwapUsed:  r.SwapUsed,
			DiskUsage: r.DiskUsage,
			NetRx:     uint64(r.NetRx),
			NetTx:     uint64(r.NetTx),
			Load1:     float64(r.Load1) / 10.0,
			Load5:     float64(r.Load5) / 10.0,
			Load15:    float64(r.Load15) / 10.0,
			Uptime:    r.Uptime,
			Online:    1 - r.Offline,
		}
		// 解析 ping_data JSON 字符串为数组
		if r.PingData != "" {
			var pings []sharedmodel.PingResult
			if err := json.Unmarshal([]byte(r.PingData), &pings); err == nil {
				hp.PingData = pings
			}
		}
		publicPoints = append(publicPoints, toPublicHistoryPoint(hp))
	}

	c.JSON(http.StatusOK, gin.H{
		"source": "sqlite",
		"points": publicPoints,
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

	type PublicDiskInfo struct {
		Total uint64 `json:"total"`
		Used  uint64 `json:"used"`
		// Device 字段不包含，防止泄露挂载点信息
	}

	type PublicServerItem struct {
		ID           int64                    `json:"id"`
		DisplayName  string                   `json:"display_name"`
		Hostname     string                   `json:"hostname"`
		OS           string                   `json:"os"`
		Arch         string                   `json:"arch"`
		AgentVersion string                   `json:"agent_version"`
		Online       bool                     `json:"online"`
		CPU          float64                  `json:"cpu"`
		Mem          float64                  `json:"mem"`
		MemTotal     uint64                   `json:"mem_total"`
		MemUsed      uint64                   `json:"mem_used"`
		SwapTotal    uint64                   `json:"swap_total"`
		SwapUsed     uint64                   `json:"swap_used"`
		NetRx        uint64                   `json:"net_rx"`
		NetTx        uint64                   `json:"net_tx"`
		Uptime       uint64                   `json:"uptime"`
		CPUModel     string                   `json:"cpu_model"`
		CPUCores     int                      `json:"cpu_cores"`
		TotalRx      uint64                   `json:"total_rx"`
		TotalTx      uint64                   `json:"total_tx"`
		Load1        float64                  `json:"load_1"`
		Load5        float64                  `json:"load_5"`
		Load15       float64                  `json:"load_15"`
		DiskUsage    float64                  `json:"disk_usage"`
		Disks        []PublicDiskInfo         `json:"disks"`
		PingData     []sharedmodel.PingResult `json:"ping_data"`
		Timestamp    int64                    `json:"timestamp"`
		// NodeGet 风格元数据（公开非敏感子集：不含出口 IPv4/IPv6）
		Tags              string     `json:"tags"`
		Region            string     `json:"region"`
		CountryCode       string     `json:"country_code"`
		ISP               string     `json:"isp"`
		ExpiresAt         *time.Time `json:"expires_at"`
		ExpiresInDays     *int       `json:"expires_in_days"`
		PriceAmount       float64    `json:"price_amount"`
		PriceCurrency     string     `json:"price_currency"`
		PriceCycle        string     `json:"price_cycle"`
		TrafficQuotaBytes int64      `json:"traffic_quota_bytes"`
		MonthlyRx         uint64     `json:"monthly_rx"`
		MonthlyTx         uint64     `json:"monthly_tx"`
	}

	monthlyTraffic := h.monitor.GetMonthlyTraffic()
	now := time.Now()

	items := make([]PublicServerItem, 0, len(agents))
	for _, agent := range agents {
		item := PublicServerItem{
			ID:           agent.ID,
			DisplayName:  agent.DisplayName,
			Hostname:     agent.Hostname,
			OS:           agent.OS,
			Arch:         agent.Arch,
			AgentVersion: agent.AgentVersion,
			Online:       h.monitor.IsOnline(agent.ID),
			Tags:              agent.Tags,
			Region:            agent.Region,
			CountryCode:       agent.CountryCode,
			ISP:               agent.ISP,
			ExpiresAt:         agent.ExpiresAt,
			ExpiresInDays:     calcExpiresInDays(agent.ExpiresAt, now),
			PriceAmount:       agent.PriceAmount,
			PriceCurrency:     agent.PriceCurrency,
			PriceCycle:        agent.PriceCycle,
			TrafficQuotaBytes: agent.TrafficQuotaBytes,
		}
		if monthlyTraffic != nil {
			if agg, ok := monthlyTraffic[agent.ID]; ok {
				item.MonthlyRx = agg.Rx
				item.MonthlyTx = agg.Tx
			}
		}

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
				item.CPUModel = p.CPUModel
				item.CPUCores = p.CPUCores
				item.TotalRx = p.TotalRx
				item.TotalTx = p.TotalTx
				item.Timestamp = p.Timestamp

				// 过滤磁盘 Device 字段 (敏感信息)，仅保留容量数据
				safeDisks := make([]PublicDiskInfo, 0, len(p.Disks))
				for _, d := range p.Disks {
					safeDisks = append(safeDisks, PublicDiskInfo{
						Total: d.Total,
						Used:  d.Used,
					})
				}
				item.Disks = safeDisks

				// 过滤 PingData 中的 Target 字段，防止泄露探测目标地址
				safePingData := make([]sharedmodel.PingResult, 0, len(p.PingData))
				for _, ping := range p.PingData {
					safePingData = append(safePingData, sharedmodel.PingResult{
						Name:        ping.Name,
						Method:      ping.Method,
						AvgLatency:  ping.AvgLatency,
						MinLatency:  ping.MinLatency,
						MaxLatency:  ping.MaxLatency,
						Jitter:      ping.Jitter,
						Loss:        ping.Loss,
						PacketsSent: ping.PacketsSent,
						PacketsRecv: ping.PacketsRecv,
						IPVersion:   ping.IPVersion,
						// Target 字段不包含
					})
				}
				item.PingData = safePingData
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
	type PublicDiskInfo struct {
		Total uint64 `json:"total"`
		Used  uint64 `json:"used"`
		// Device 字段不包含，防止泄露挂载点信息
	}

	type PublicDashboardItem struct {
		AgentID      int64                    `json:"agent_id"`
		Hostname     string                   `json:"hostname"`
		DisplayName  string                   `json:"display_name"`
		OS           string                   `json:"os"`
		Arch         string                   `json:"arch"`
		AgentVersion string                   `json:"agent_version"`
		Online       bool                     `json:"online"`
		CPU          float64                  `json:"cpu"`
		Mem          float64                  `json:"mem"`
		MemTotal     uint64                   `json:"mem_total"`
		MemUsed      uint64                   `json:"mem_used"`
		SwapTotal    uint64                   `json:"swap_total"`
		SwapUsed     uint64                   `json:"swap_used"`
		NetRx        uint64                   `json:"net_rx"`
		NetTx        uint64                   `json:"net_tx"`
		Load1        float64                  `json:"load_1"`
		Load5        float64                  `json:"load_5"`
		Load15       float64                  `json:"load_15"`
		Uptime       uint64                   `json:"uptime"`
		CPUModel     string                   `json:"cpu_model"`
		CPUCores     int                      `json:"cpu_cores"`
		TotalRx      uint64                   `json:"total_rx"`
		TotalTx      uint64                   `json:"total_tx"`
		DiskUsage    float64                  `json:"disk_usage"`
		Disks        []PublicDiskInfo         `json:"disks"`
		PingData     []sharedmodel.PingResult `json:"ping_data"`
		Timestamp    int64                    `json:"timestamp"`
		// NodeGet 风格元数据（公开非敏感子集：不含出口 IPv4/IPv6）
		Tags              string     `json:"tags"`
		Region            string     `json:"region"`
		CountryCode       string     `json:"country_code"`
		ISP               string     `json:"isp"`
		ExpiresAt         *time.Time `json:"expires_at"`
		ExpiresInDays     *int       `json:"expires_in_days"`
		PriceAmount       float64    `json:"price_amount"`
		PriceCurrency     string     `json:"price_currency"`
		PriceCycle        string     `json:"price_cycle"`
		TrafficQuotaBytes int64      `json:"traffic_quota_bytes"`
		MonthlyRx         uint64     `json:"monthly_rx"`
		MonthlyTx         uint64     `json:"monthly_tx"`
	}

	publicItems := make([]PublicDashboardItem, 0, len(items))
	for _, item := range items {
		// 过滤 PingData 中的 Target 字段，防止泄露探测目标地址
		safePingData := make([]sharedmodel.PingResult, 0, len(item.PingData))
		for _, p := range item.PingData {
			safePingData = append(safePingData, sharedmodel.PingResult{
				Name:        p.Name,
				Method:      p.Method,
				AvgLatency:  p.AvgLatency,
				MinLatency:  p.MinLatency,
				MaxLatency:  p.MaxLatency,
				Jitter:      p.Jitter,
				Loss:        p.Loss,
				PacketsSent: p.PacketsSent,
				PacketsRecv: p.PacketsRecv,
				IPVersion:   p.IPVersion,
				// Target 字段不包含
			})
		}

		// 过滤磁盘 Device 字段 (敏感信息)，仅保留容量数据
		safeDisks := make([]PublicDiskInfo, 0, len(item.Disks))
		for _, d := range item.Disks {
			safeDisks = append(safeDisks, PublicDiskInfo{
				Total: d.Total,
				Used:  d.Used,
			})
		}

		publicItems = append(publicItems, PublicDashboardItem{
			AgentID:      item.AgentID,
			Hostname:     item.Hostname,
			DisplayName:  item.DisplayName,
			OS:           item.OS,
			Arch:         item.Arch,
			AgentVersion: item.AgentVersion,
			Online:       item.Online,
			CPU:          item.CPU,
			Mem:          item.Mem,
			MemTotal:     item.MemTotal,
			MemUsed:      item.MemUsed,
			SwapTotal:    item.SwapTotal,
			SwapUsed:     item.SwapUsed,
			NetRx:        item.NetRx,
			NetTx:        item.NetTx,
			Load1:        item.Load1,
			Load5:        item.Load5,
			Load15:       item.Load15,
			Uptime:       item.Uptime,
			CPUModel:     item.CPUModel,
			CPUCores:     item.CPUCores,
			TotalRx:      item.TotalRx,
			TotalTx:      item.TotalTx,
			DiskUsage:    item.DiskUsage,
			Disks:        safeDisks,
			PingData:     safePingData,
			Timestamp:    item.Timestamp,
			Tags:              item.Tags,
			Region:            item.Region,
			CountryCode:       item.CountryCode,
			ISP:               item.ISP,
			ExpiresAt:         item.ExpiresAt,
			ExpiresInDays:     item.ExpiresInDays,
			PriceAmount:       item.PriceAmount,
			PriceCurrency:     item.PriceCurrency,
			PriceCycle:        item.PriceCycle,
			TrafficQuotaBytes: item.TrafficQuotaBytes,
			MonthlyRx:         item.MonthlyRx,
			MonthlyTx:         item.MonthlyTx,
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
		"uptime":         int64(time.Since(startTime).Seconds()),
		"mem_alloc":      memStats.Alloc,
		"mem_sys":        memStats.Sys,
		"mem_num_gc":     memStats.NumGC,
		"db_size":        dbSize,
		"online_agents":  onlineCount,
		"ws_connections": wsConnCount,
		"goroutines":     runtime.NumGoroutine(),
		"disk_total":     diskTotal,
		"disk_free":      diskFree,
		"version":        "1.0.0",
	})
}
