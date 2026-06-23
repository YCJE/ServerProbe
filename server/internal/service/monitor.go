package service

import (
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/server-probe/server/internal/model"
	"github.com/server-probe/server/internal/repository"
	sharedmodel "github.com/server-probe/shared/model"
)

// AgentConn 表示一个 Agent 的 WebSocket 连接
type AgentConn struct {
	AgentID  int64
	Conn     *websocket.Conn
	LastSeen time.Time
	mu       sync.Mutex
}

// Send 向 Agent 发送消息
func (ac *AgentConn) Send(msg sharedmodel.WSMessage) error {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	return ac.Conn.WriteJSON(msg)
}

// MonitorService 实时数据管理服务
type MonitorService struct {
	agentRepo    *repository.AgentRepository
	ringBuffers  map[int64]*repository.RingBuffer
	connections  map[int64]*AgentConn
	mu           sync.RWMutex
	onConfigPush func(agentID int64, config *sharedmodel.AgentConfig)
	dataDir      string
	dashWSCount  int32 // 面板 WebSocket 连接数 (atomic)
	ticker       *time.Ticker
	stopCh       chan struct{}
}

// NewMonitorService 创建监控服务
func NewMonitorService(agentRepo *repository.AgentRepository, dataDir string) *MonitorService {
	return &MonitorService{
		agentRepo:   agentRepo,
		ringBuffers: make(map[int64]*repository.RingBuffer),
		connections: make(map[int64]*AgentConn),
		dataDir:     dataDir,
		stopCh:      make(chan struct{}),
	}
}

// GetOnlineAgentCount 获取在线 Agent 数量
func (m *MonitorService) GetOnlineAgentCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.connections)
}

// IsAgentOnline 检查 Agent 是否在线
func (m *MonitorService) IsAgentOnline(agentID int64) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.connections[agentID]
	return ok
}

// GetDashboardWSCount 获取面板 WebSocket 连接数
func (m *MonitorService) GetDashboardWSCount() int {
	return int(atomic.LoadInt32(&m.dashWSCount))
}

// IncDashboardWS 面板 WS 连接数 +1
func (m *MonitorService) IncDashboardWS() {
	atomic.AddInt32(&m.dashWSCount, 1)
}

// DecDashboardWS 面板 WS 连接数 -1 (防止下溢)
func (m *MonitorService) DecDashboardWS() {
	for {
		old := atomic.LoadInt32(&m.dashWSCount)
		if old <= 0 {
			return
		}
		if atomic.CompareAndSwapInt32(&m.dashWSCount, old, old-1) {
			return
		}
	}
}

// GetDataDir 获取数据目录
func (m *MonitorService) GetDataDir() string {
	return m.dataDir
}

// RegisterConnection 注册 Agent 连接
func (m *MonitorService) RegisterConnection(agentID int64, conn *websocket.Conn) *AgentConn {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 如果已有连接，关闭旧连接
	if oldConn, ok := m.connections[agentID]; ok {
		oldConn.Conn.Close()
	}

	agentConn := &AgentConn{
		AgentID:  agentID,
		Conn:     conn,
		LastSeen: time.Now(),
	}
	m.connections[agentID] = agentConn

	// 确保有环形缓冲
	if _, ok := m.ringBuffers[agentID]; !ok {
		m.ringBuffers[agentID] = repository.NewRingBuffer(7200) // 7200 点 × 3s = 6 小时
	}

	// 更新数据库在线状态
	_ = m.agentRepo.UpdateOnlineStatus(agentID, true)

	log.Printf("Agent %d 已连接", agentID)
	return agentConn
}

// UnregisterConnection 注销 Agent 连接
func (m *MonitorService) UnregisterConnection(agentID int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if conn, ok := m.connections[agentID]; ok {
		conn.Conn.Close()
		delete(m.connections, agentID)
	}

	// 更新数据库在线状态
	_ = m.agentRepo.UpdateOnlineStatus(agentID, false)
}

// UnregisterConnectionIfMatch 条件注销: 仅当注册的连接与传入连接相同时才注销
// 解决 Agent 重连竞态: 旧连接的 defer 不应关闭新连接
func (m *MonitorService) UnregisterConnectionIfMatch(agentID int64, conn *websocket.Conn) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if ac, ok := m.connections[agentID]; ok {
		if ac.Conn == conn {
			ac.Conn.Close()
			delete(m.connections, agentID)
			_ = m.agentRepo.UpdateOnlineStatus(agentID, false)
			return true
		}
		// 连接已被新连接替换，不执行注销
		return false
	}
	return false
}

// UnregisterAgent 完全移除 Agent (删除 Agent 时调用)
// 关闭连接、删除 ringBuffer、更新在线状态
func (m *MonitorService) UnregisterAgent(agentID int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 关闭 WebSocket 连接
	if conn, ok := m.connections[agentID]; ok {
		conn.Conn.Close()
		delete(m.connections, agentID)
	}

	// 删除 ringBuffer
	delete(m.ringBuffers, agentID)

	// 更新数据库在线状态
	_ = m.agentRepo.UpdateOnlineStatus(agentID, false)

	log.Printf("Agent %d 已完全移除 (连接+ringBuffer)", agentID)
}

// BroadcastConfigUpdate 向所有在线 Agent 推送配置更新
func (m *MonitorService) BroadcastConfigUpdate(config *sharedmodel.AgentConfig) {
	m.mu.RLock()
	// 收集所有在线 Agent ID (不持锁写入，避免阻塞监控服务)
	agentIDs := make([]int64, 0, len(m.connections))
	for agentID := range m.connections {
		agentIDs = append(agentIDs, agentID)
	}
	m.mu.RUnlock()

	// 通过 OnConfigPush 回调推送 (使用 handler_agent.go 中的 agentWSConn.mu 锁)
	if m.onConfigPush != nil {
		for _, agentID := range agentIDs {
			m.onConfigPush(agentID, config)
		}
	}
}

// SetConfigPushCallback 设置配置推送回调 (由 handler_agent.go 注册)
func (m *MonitorService) SetConfigPushCallback(cb func(agentID int64, config *sharedmodel.AgentConfig)) {
	m.onConfigPush = cb
}

// UpdateHeartbeat 更新心跳时间
func (m *MonitorService) UpdateHeartbeat(agentID int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if conn, ok := m.connections[agentID]; ok {
		conn.LastSeen = time.Now()
	}
}

// WriteMetricData 写入实时监控数据到环形缓冲
func (m *MonitorService) WriteMetricData(agentID int64, data *sharedmodel.MetricData) error {
	m.mu.RLock()
	rb, ok := m.ringBuffers[agentID]
	m.mu.RUnlock()

	if !ok {
		// double-check: 获取写锁后再次检查，避免并发创建多个 RingBuffer
		m.mu.Lock()
		if rb, ok = m.ringBuffers[agentID]; !ok {
			rb = repository.NewRingBuffer(7200) // 7200 点 × 3s = 6 小时
			m.ringBuffers[agentID] = rb
		}
		m.mu.Unlock()
	}

	// 计算内存使用率
	memUsage := 0.0
	if data.Memory.Total > 0 {
		memUsage = float64(data.Memory.Used) / float64(data.Memory.Total) * 100
	}

	// 构建数据点
	point := repository.MetricPoint{
		Timestamp:    time.Now().Unix(),
		CPU:          data.CPU.Usage,
		Mem:          memUsage,
		MemTotal:     data.Memory.Total,
		MemUsed:      data.Memory.Used,
		SwapTotal:    data.Memory.SwapTotal,
		SwapUsed:     data.Memory.SwapUsed,
		Disks:        data.Disks,
		NetRx:        data.Network.RxSpeed,
		NetTx:        data.Network.TxSpeed,
		TCPConns:     data.Network.TCPConnections,
		UDPConns:     data.Network.UDPConnections,
		Load1:        data.CPU.Load1,
		Load5:        data.CPU.Load5,
		Load15:       data.CPU.Load15,
		Uptime:       data.Uptime,
		ProcessCount: data.ProcessCount,
	}

	// 继承上一个数据点的 PingData (Ping 每 60s 上报一次，指标每 3s 上报一次)
	// 避免新数据点覆盖 Ping 数据导致延迟信息丢失
	prevPoints := rb.Latest(1)
	if len(prevPoints) > 0 {
		point.PingData = prevPoints[0].PingData
	}

	rb.Write(point)
	return nil
}

// WritePingData 写入 Ping 探测数据
func (m *MonitorService) WritePingData(agentID int64, pingData []sharedmodel.PingResult) error {
	m.mu.RLock()
	rb, ok := m.ringBuffers[agentID]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("Agent %d 的环形缓冲不存在", agentID)
	}

	// 更新最新数据点的 PingData (不创建新数据点)
	rb.UpdateLastPing(pingData)
	return nil
}

// GetRingBuffer 获取 Agent 的环形缓冲
func (m *MonitorService) GetRingBuffer(agentID int64) *repository.RingBuffer {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.ringBuffers[agentID]
}

// IsOnline 检查 Agent 是否在线
func (m *MonitorService) IsOnline(agentID int64) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.connections[agentID]
	return ok
}

// GetOnlineAgentIDs 获取所有在线 Agent ID
func (m *MonitorService) GetOnlineAgentIDs() []int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ids := make([]int64, 0, len(m.connections))
	for id := range m.connections {
		ids = append(ids, id)
	}
	return ids
}

// GetAllAgentIDs 获取所有 Agent ID（包括离线的），从数据库读取
func (m *MonitorService) GetAllAgentIDs() []int64 {
	agents, err := m.agentRepo.List()
	if err != nil {
		log.Printf("获取所有 Agent 列表失败: %v", err)
		return nil
	}
	ids := make([]int64, 0, len(agents))
	for _, agent := range agents {
		ids = append(ids, agent.ID)
	}
	return ids
}

// CheckHeartbeatTimeout 检查心跳超时
func (m *MonitorService) CheckHeartbeatTimeout(timeout time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for agentID, conn := range m.connections {
		if now.Sub(conn.LastSeen) > timeout {
			log.Printf("Agent %d 心跳超时，断开连接", agentID)
			conn.Conn.Close()
			delete(m.connections, agentID)
			_ = m.agentRepo.UpdateOnlineStatus(agentID, false)
		}
	}
}

// StartHeartbeatChecker 启动心跳检查器
func (m *MonitorService) StartHeartbeatChecker(timeout time.Duration) {
	m.ticker = time.NewTicker(30 * time.Second)
	go func() {
		for {
			select {
			case <-m.ticker.C:
				m.CheckHeartbeatTimeout(timeout)
			case <-m.stopCh:
				return
			}
		}
	}()
}

// Stop 停止监控服务（停止心跳检查器）
func (m *MonitorService) Stop() {
	if m.ticker != nil {
		m.ticker.Stop()
	}
	close(m.stopCh)
}

// GetDashboardData 获取仪表盘数据
func (m *MonitorService) GetDashboardData() []DashboardItem {
	// 先获取所有 Agent 的 hostname 和 display_name（避免在持锁期间进行 DB 调用）
	agents, _ := m.agentRepo.List()
	agentMap := make(map[int64]*model.Agent, len(agents))
	for i := range agents {
		agentMap[agents[i].ID] = &agents[i]
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	items := make([]DashboardItem, 0, len(m.ringBuffers))

	for agentID, rb := range m.ringBuffers {
		points := rb.Latest(1)
		if len(points) == 0 {
			continue
		}

		p := points[0]
		item := DashboardItem{
			AgentID:      agentID,
			Online:       m.isOnlineLocked(agentID),
			CPU:          p.CPU,
			Mem:          p.Mem,
			MemTotal:     p.MemTotal,
			MemUsed:      p.MemUsed,
			SwapTotal:    p.SwapTotal,
			SwapUsed:     p.SwapUsed,
			NetRx:        p.NetRx,
			NetTx:        p.NetTx,
			Load1:        p.Load1,
			Load5:        p.Load5,
			Load15:       p.Load15,
			Uptime:       p.Uptime,
			DiskUsage:    calcDiskUsage(p.Disks),
			Disks:        p.Disks,
			TCPConns:     p.TCPConns,
			UDPConns:     p.UDPConns,
			ProcessCount: p.ProcessCount,
			PingData:     p.PingData,
			Timestamp:    p.Timestamp,
		}

		// 补充 hostname 和 display_name
		if agent := agentMap[agentID]; agent != nil {
			item.Hostname = agent.Hostname
			item.DisplayName = agent.DisplayName
		}

		items = append(items, item)
	}

	return items
}

// isOnlineLocked 检查是否在线（调用方已持有锁）
func (m *MonitorService) isOnlineLocked(agentID int64) bool {
	_, ok := m.connections[agentID]
	return ok
}

// calcDiskUsage 计算磁盘使用率 (优先根分区，否则取最大分区)
func calcDiskUsage(disks []sharedmodel.DiskInfo) float64 {
	if len(disks) == 0 {
		return 0
	}
	// Agent 现在返回单个汇总磁盘 (Device="total")
	// 直接计算总使用率
	var totalUsed, totalTotal uint64
	for _, d := range disks {
		totalUsed += d.Used
		totalTotal += d.Total
	}
	if totalTotal > 0 {
		return float64(totalUsed) / float64(totalTotal) * 100
	}
	return 0
}

// DashboardItem 仪表盘数据项
type DashboardItem struct {
	AgentID      int64                    `json:"agent_id"`
	Hostname     string                   `json:"hostname"`
	DisplayName  string                   `json:"display_name"`
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
	DiskUsage    float64                  `json:"disk_usage"`
	Disks        []sharedmodel.DiskInfo   `json:"disks"`
	TCPConns     int                      `json:"tcp_connections"`
	UDPConns     int                      `json:"udp_connections"`
	ProcessCount int                      `json:"process_count"`
	PingData     []sharedmodel.PingResult `json:"ping_data"`
	Timestamp    int64                    `json:"timestamp"`
}
