package service

import (
	"fmt"
	"log"
	"sync"
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
}

// NewMonitorService 创建监控服务
func NewMonitorService(agentRepo *repository.AgentRepository) *MonitorService {
	return &MonitorService{
		agentRepo:   agentRepo,
		ringBuffers: make(map[int64]*repository.RingBuffer),
		connections: make(map[int64]*AgentConn),
	}
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
		m.ringBuffers[agentID] = repository.NewRingBuffer(3600)
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

	log.Printf("Agent %d 已断开", agentID)
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
		m.mu.Lock()
		rb = repository.NewRingBuffer(3600)
		m.ringBuffers[agentID] = rb
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

	// 获取最新的数据点，更新 Ping 数据
	points := rb.Latest(1)
	if len(points) > 0 {
		points[0].PingData = pingData
		// 重新写入更新后的数据点
		rb.Write(points[0])
	}

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
	ticker := time.NewTicker(30 * time.Second)
	go func() {
		for range ticker.C {
			m.CheckHeartbeatTimeout(timeout)
		}
	}()
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
			AgentID:    agentID,
			Online:     m.isOnlineLocked(agentID),
			CPU:        p.CPU,
			Mem:        p.Mem,
			MemTotal:   p.MemTotal,
			MemUsed:    p.MemUsed,
			NetRx:      p.NetRx,
			NetTx:      p.NetTx,
			Load1:      p.Load1,
			Uptime:     p.Uptime,
			PingData:   p.PingData,
			Timestamp:  p.Timestamp,
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

// DashboardItem 仪表盘数据项
type DashboardItem struct {
	AgentID     int64                        `json:"agent_id"`
	Hostname    string                       `json:"hostname"`
	DisplayName string                       `json:"display_name"`
	Online      bool                         `json:"online"`
	CPU         float64                      `json:"cpu"`
	Mem         float64                      `json:"mem"`
	MemTotal    uint64                       `json:"mem_total"`
	MemUsed     uint64                       `json:"mem_used"`
	NetRx       uint64                       `json:"net_rx"`
	NetTx       uint64                       `json:"net_tx"`
	Load1       float64                      `json:"load_1"`
	Uptime      uint64                       `json:"uptime"`
	PingData    []sharedmodel.PingResult     `json:"ping_data"`
	Timestamp   int64                        `json:"timestamp"`
}
