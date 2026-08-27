package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net"
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
}

// MonitorService 实时数据管理服务
type MonitorService struct {
	agentRepo    *repository.AgentRepository
	recordRepo   *repository.RecordRepository // 历史数据 repository（用于 BatchWriter）
	trafficRepo  *repository.TrafficRepository
	ringBuffers  map[int64]*repository.RingBuffer
	connections  map[int64]*AgentConn
	mu           sync.RWMutex
	onConfigPush func(agentID int64, config *sharedmodel.AgentConfig)
	dataDir      string
	dashWSCount     int32 // 管理员面板 WebSocket 连接数 (atomic)
	pubDashWSCount  int32 // 公开面板 WebSocket 连接数 (atomic)
	ticker       *time.Ticker
	stopCh       chan struct{}
	stopOnce     sync.Once
	wg           sync.WaitGroup      // 跟踪后台 goroutine
	lastDBUpdate map[int64]time.Time // 限频更新 last_seen 的记录 (复用 mu 保护)
	// agent 列表缓存，避免 Dashboard WS 每次推送都查询数据库
	agentListCache     []model.Agent
	agentListCacheAt   time.Time
	agentListCacheLock sync.Mutex // 防止惊群效应：确保同一时刻只有一个 goroutine 刷新缓存

	// v1.1: 月度流量合计缓存（仪表盘进度条 + 流量配额告警共用）
	// 每 60 秒最多查询一次，聚合服务每 5 分钟才更新流量表，无需更高频刷新
	trafficCacheMu   sync.Mutex
	trafficCache     map[int64]repository.MonthlyTrafficAgg
	trafficCacheAt   time.Time
	trafficCacheDate string // "2006-01" 缓存所属月份，跨月时强制刷新

	// 离线判定宽限期（后台可调，atomic 保证检查器无锁读取，存纳秒）
	heartbeatTimeout atomic.Int64

	// 批量写入缓冲器
	batchWriter *repository.BatchWriter

	// 静态数据哈希缓存：agentID → 上次上报的 SystemInfo SHA-256 哈希
	// 用于去重，避免静态数据未变化时重复写入数据库
	staticHashCache map[int64]string
	staticHashMu    sync.RWMutex

	// P0-1: 预序列化 JSON 缓存 — 数据变化时失效，首次请求时重建
	// 所有 WS 客户端共享同一份序列化结果，避免 250 个客户端各自序列化
	dashCacheMu     sync.Mutex
	dashCacheJSON   []byte    // 管理员推送 JSON（完整摘要）
	dashCacheTime   time.Time // 缓存生成时间
	pubDashCacheMu  sync.Mutex
	pubDashCacheJSON []byte    // 公开推送 JSON（过滤敏感字段）
	pubDashCacheTime time.Time // 缓存生成时间
}

// NewMonitorService 创建监控服务
// recordRepo 用于初始化 BatchWriter 实现批量写入缓冲
func NewMonitorService(agentRepo *repository.AgentRepository, recordRepo *repository.RecordRepository, dataDir string) *MonitorService {
	m := &MonitorService{
		agentRepo:       agentRepo,
		recordRepo:      recordRepo,
		ringBuffers:     make(map[int64]*repository.RingBuffer),
		connections:     make(map[int64]*AgentConn),
		dataDir:         dataDir,
		stopCh:          make(chan struct{}),
		lastDBUpdate:    make(map[int64]time.Time),
		staticHashCache: make(map[int64]string),
	}

	// 初始化 BatchWriter（批量写入缓冲）
	if recordRepo != nil {
		bw := repository.NewBatchWriter(recordRepo.FlushBatch)
		recordRepo.SetBatchWriter(bw)
		m.batchWriter = bw
		bw.Start()
		log.Println("BatchWriter 已启动（缓冲容量 10000，每 500ms 或 100 条 flush）")
	}

	return m
}

// SetTrafficRepo 注入流量统计 repository（用于仪表盘月流量进度条与流量配额告警）
func (m *MonitorService) SetTrafficRepo(trafficRepo *repository.TrafficRepository) {
	m.trafficRepo = trafficRepo
}

// GetMonthlyTraffic 获取所有 Agent 当月流量合计（带 60 秒缓存，跨月强制刷新）
// 供仪表盘下发的 monthly_rx/monthly_tx 字段与告警引擎 traffic_quota 指标共用
func (m *MonitorService) GetMonthlyTraffic() map[int64]repository.MonthlyTrafficAgg {
	now := time.Now()
	monthKey := now.Format("2006-01")

	m.trafficCacheMu.Lock()
	defer m.trafficCacheMu.Unlock()

	if m.trafficRepo != nil &&
		(m.trafficCache == nil || m.trafficCacheDate != monthKey || time.Since(m.trafficCacheAt) >= 60*time.Second) {
		if agg, err := m.trafficRepo.GetAllMonthlyTrafficAgg(now.Year(), int(now.Month())); err == nil {
			m.trafficCache = agg
			m.trafficCacheAt = now
			m.trafficCacheDate = monthKey
		} else {
			log.Printf("[Monitor] 查询月度流量合计失败: %v", err)
		}
	}

	if m.trafficCache == nil {
		return nil
	}
	return m.trafficCache
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

// GetDashboardWSCount 获取管理员面板 WebSocket 连接数
func (m *MonitorService) GetDashboardWSCount() int {
	return int(atomic.LoadInt32(&m.dashWSCount))
}

// IncDashboardWS 管理员面板 WS 连接数 +1，返回递增后的值
func (m *MonitorService) IncDashboardWS() int {
	return int(atomic.AddInt32(&m.dashWSCount, 1))
}

// DecDashboardWS 管理员面板 WS 连接数 -1 (防止下溢)
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

// IncPublicDashboardWS 公开面板 WS 连接数 +1，返回递增后的值
func (m *MonitorService) IncPublicDashboardWS() int {
	return int(atomic.AddInt32(&m.pubDashWSCount, 1))
}

// DecPublicDashboardWS 公开面板 WS 连接数 -1 (防止下溢)
func (m *MonitorService) DecPublicDashboardWS() {
	for {
		old := atomic.LoadInt32(&m.pubDashWSCount)
		if old <= 0 {
			return
		}
		if atomic.CompareAndSwapInt32(&m.pubDashWSCount, old, old-1) {
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

	// 在锁内收集需要关闭的旧连接引用，锁外再关闭，避免持锁阻塞
	var oldConn *AgentConn
	if oc, ok := m.connections[agentID]; ok {
		oldConn = oc
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

	m.mu.Unlock()

	// 在锁外关闭旧连接，避免持锁阻塞监控服务
	if oldConn != nil {
		oldConn.Conn.Close()
	}

	// 更新数据库在线状态（锁外执行，避免持锁阻塞监控服务）
	_ = m.agentRepo.UpdateOnlineStatus(agentID, true)
	// 更新 last_seen 时间戳（标记上线）
	_ = m.agentRepo.UpdateLastSeen(agentID, true)
	// 记录出口 IPv4/IPv6（连接 Server 的真实出口 IP，值变化时才写库）
	m.updateExitIP(agentID, conn)

	log.Printf("Agent %d 已连接", agentID)
	return agentConn
}

// updateExitIP 从 WS 连接的 RemoteAddr 提取出口 IP 并更新 Agent 表
// 一次连接只反映单栈出口（v4 或 v6 之一），另一栈保留历史值
func (m *MonitorService) updateExitIP(agentID int64, conn *websocket.Conn) {
	if conn == nil {
		return
	}
	host, _, err := net.SplitHostPort(conn.RemoteAddr().String())
	if err != nil {
		host = conn.RemoteAddr().String()
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return
	}

	agent, err := m.agentRepo.GetByID(agentID)
	if err != nil {
		return
	}

	if ip.To4() != nil {
		if agent.IPv4 == host {
			return // 值未变化，跳过写库
		}
		_ = m.agentRepo.UpdateExitIP(agentID, host, agent.IPv6)
	} else {
		if agent.IPv6 == host {
			return
		}
		_ = m.agentRepo.UpdateExitIP(agentID, agent.IPv4, host)
	}
}

// UnregisterConnection 注销 Agent 连接
func (m *MonitorService) UnregisterConnection(agentID int64) {
	m.mu.Lock()

	if conn, ok := m.connections[agentID]; ok {
		conn.Conn.Close()
		delete(m.connections, agentID)
	}

	// 清理 lastDBUpdate 记录，防止内存泄漏（与 UnregisterConnectionIfMatch 保持一致）
	delete(m.lastDBUpdate, agentID)

	m.mu.Unlock()

	// 清理静态数据哈希缓存，防止内存泄漏
	m.staticHashMu.Lock()
	delete(m.staticHashCache, agentID)
	m.staticHashMu.Unlock()

	// 更新数据库在线状态
	_ = m.agentRepo.UpdateOnlineStatus(agentID, false)
}

// UnregisterConnectionIfMatch 条件注销: 仅当注册的连接与传入连接相同时才注销
// 解决 Agent 重连竞态: 旧连接的 defer 不应关闭新连接
func (m *MonitorService) UnregisterConnectionIfMatch(agentID int64, conn *websocket.Conn) bool {
	m.mu.Lock()

	// 在锁内仅收集需要清理的状态、关闭连接、删除 map 条目
	needCleanup := false
	if ac, ok := m.connections[agentID]; ok {
		if ac.Conn == conn {
			ac.Conn.Close()
			delete(m.connections, agentID)
			// 清理 lastDBUpdate 记录，防止内存泄漏
			delete(m.lastDBUpdate, agentID)
			needCleanup = true
		}
		// 连接已被新连接替换，不执行注销
	}
	m.mu.Unlock()

	if !needCleanup {
		return false
	}

	// 清理静态数据哈希缓存，防止内存泄漏
	m.staticHashMu.Lock()
	delete(m.staticHashCache, agentID)
	m.staticHashMu.Unlock()

	// 在锁外执行数据库写入，避免持锁阻塞监控服务
	// 更新 last_seen 时间戳（标记离线）
	_ = m.agentRepo.UpdateLastSeen(agentID, false)
	_ = m.agentRepo.UpdateOnlineStatus(agentID, false)
	return true
}

// UnregisterAgent 完全移除 Agent (删除 Agent 时调用)
// 关闭连接、删除 ringBuffer、更新在线状态、清理哈希缓存
func (m *MonitorService) UnregisterAgent(agentID int64) {
	m.mu.Lock()

	// 关闭 WebSocket 连接
	if conn, ok := m.connections[agentID]; ok {
		conn.Conn.Close()
		delete(m.connections, agentID)
	}

	// 删除 ringBuffer
	delete(m.ringBuffers, agentID)

	// 清理 lastDBUpdate 记录，防止内存泄漏
	delete(m.lastDBUpdate, agentID)

	// 使 Agent 列表缓存立即失效（与 GetAllAgents 的 m.mu 读写锁保持一致），
	// 避免已删除 Agent 在缓存 TTL 内仍出现在仪表盘
	m.agentListCache = nil
	m.agentListCacheAt = time.Time{}

	m.mu.Unlock()

	// 在锁外执行数据库写入，避免持锁阻塞监控服务
	// 更新数据库在线状态
	if err := m.agentRepo.UpdateOnlineStatus(agentID, false); err != nil {
		log.Printf("[Monitor] UpdateOnlineStatus failed for agent %d: %v", agentID, err)
	}

	// 清理静态数据哈希缓存，防止内存泄漏
	m.staticHashMu.Lock()
	delete(m.staticHashCache, agentID)
	m.staticHashMu.Unlock()

	log.Printf("Agent %d 已完全移除 (连接+ringBuffer+哈希缓存)", agentID)
}

// BroadcastConfigUpdate 向所有在线 Agent 推送配置更新
func (m *MonitorService) BroadcastConfigUpdate(config *sharedmodel.AgentConfig) {
	m.mu.RLock()
	// 收集所有在线 Agent ID (不持锁写入，避免阻塞监控服务)
	agentIDs := make([]int64, 0, len(m.connections))
	for agentID := range m.connections {
		agentIDs = append(agentIDs, agentID)
	}
	// 在 RLock 内复制回调函数指针，避免锁释放后被 SetConfigPushCallback 替换
	onConfigPush := m.onConfigPush
	m.mu.RUnlock()

	if onConfigPush == nil {
		return
	}

	// 并发推送配置，使用 context 超时控制，超时后不再启动新的推送 goroutine
	var wg sync.WaitGroup
	sem := make(chan struct{}, 10) // 限制并发数为 10
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, agentID := range agentIDs {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
				// 获取信号后再次检查是否已超时
				select {
				case <-ctx.Done():
					return
				default:
					onConfigPush(id, config)
				}
			case <-ctx.Done():
				return // 超时后跳过推送
			}
		}(agentID)
	}
	// 等待推送完成或超时
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		log.Printf("[Monitor] BroadcastConfigUpdate timed out after 5s, some agents may not have received the update")
	}
}

// SetConfigPushCallback 设置配置推送回调 (由 handler_agent.go 注册)
func (m *MonitorService) SetConfigPushCallback(cb func(agentID int64, config *sharedmodel.AgentConfig)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onConfigPush = cb
}

// UpdateHeartbeat 更新心跳时间
func (m *MonitorService) UpdateHeartbeat(agentID int64) {
	var needDBUpdate bool

	m.mu.Lock()
	if conn, ok := m.connections[agentID]; ok {
		conn.LastSeen = time.Now()
	}
	// 限频更新数据库 last_seen（每 60 秒最多一次），避免持续在线时 last_seen 始终停留在上线时间
	if last, ok := m.lastDBUpdate[agentID]; !ok || time.Since(last) >= 60*time.Second {
		m.lastDBUpdate[agentID] = time.Now()
		needDBUpdate = true
	}
	m.mu.Unlock()

	// 在锁外执行数据库写入，避免持锁阻塞监控服务
	if needDBUpdate {
		_ = m.agentRepo.UpdateLastSeen(agentID, true)
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
			// 如果连接已不存在（Agent 已被删除/注销），拒绝创建 ringBuffer
			// 防止 UnregisterAgent 后的竞态导致 ringBuffer 永久驻留内存
			if _, connOk := m.connections[agentID]; !connOk {
				m.mu.Unlock()
				return fmt.Errorf("Agent %d 连接不存在，拒绝创建 ringBuffer", agentID)
			}
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
		CPUModel:     data.CPU.Model,
		CPUCores:     data.CPU.Cores,
		Mem:          memUsage,
		MemTotal:     data.Memory.Total,
		MemUsed:      data.Memory.Used,
		SwapTotal:    data.Memory.SwapTotal,
		SwapUsed:     data.Memory.SwapUsed,
		Disks:        data.Disks,
		NetRx:        data.Network.RxSpeed,
		NetTx:        data.Network.TxSpeed,
		TotalRx:      data.Network.TotalRx,
		TotalTx:      data.Network.TotalTx,
		TCPConns:     data.Network.TCPConnections,
		UDPConns:     data.Network.UDPConnections,
		Load1:        data.CPU.Load1,
		Load5:        data.CPU.Load5,
		Load15:       data.CPU.Load15,
		Uptime:       data.Uptime,
		ProcessCount: data.ProcessCount,
		TimeOffset:   data.TimeOffset,
		Temperature:  data.Temperature,
		Processes:    data.Processes,
	}

	// 继承上一个数据点的 PingData (Ping 每 60s 上报一次，指标每 3s 上报一次)
	// 避免新数据点覆盖 Ping 数据导致延迟信息丢失
	prevPoints := rb.Latest(1)
	if len(prevPoints) > 0 {
		point.PingData = prevPoints[0].PingData
	}

	rb.Write(point)

	// P0-1: 数据变化时失效预序列化缓存
	m.invalidateDashboardCache()
	return nil
}

// HandleAgentReport 处理 Agent 上报的监控数据（任务 1 + 任务 3）
//   - 动态数据（CPU、内存、磁盘、网络等）写入环形缓冲，照常存储
//   - 静态数据（SystemInfo）通过 SHA-256 哈希去重后更新 Agent 表
//   - 如果 SystemInfo.OS 为空（非静态采集周期），跳过静态数据的数据库写入
func (m *MonitorService) HandleAgentReport(agentID int64, data *sharedmodel.MetricData) error {
	// 1. 写入动态数据到环形缓冲（照常存储）
	if err := m.WriteMetricData(agentID, data); err != nil {
		return err
	}

	// 2. 处理静态数据 - 如果 SystemInfo.OS 为空，跳过静态数据写入
	if data.System.OS == "" {
		return nil
	}

	// 3. 计算 SHA-256 哈希，去重
	hash := computeStaticHash(&data.System)

	// 使用单一写锁覆盖整个"检查哈希 -> DB 更新 -> 更新缓存"流程，
	// 消除 TOCTOU 竞态（原先先读锁检查后写锁更新，两个并发请求可能同时通过哈希检查）
	m.staticHashMu.Lock()
	defer m.staticHashMu.Unlock()

	cachedHash, exists := m.staticHashCache[agentID]

	// 哈希与上次相同，跳过静态数据的数据库写入
	if exists && cachedHash == hash {
		return nil
	}

	// 4. 哈希不同（或首次上报），写入新数据并更新哈希缓存
	if err := m.agentRepo.UpdateStaticInfo(
		agentID,
		data.System.OS,
		data.System.Arch,
		data.System.Kernel,
		data.System.Hostname,
		data.System.AgentVersion,
		data.System.Virtualization,
		data.System.Distro,
	); err != nil {
		log.Printf("[Monitor] 更新 Agent %d 静态信息失败: %v", agentID, err)
		return err
	}

	m.staticHashCache[agentID] = hash

	log.Printf("[Monitor] Agent %d 静态信息已更新 (hash=%s)", agentID, hash[:16])
	return nil
}

// computeStaticHash 计算静态数据的 SHA-256 哈希
// 将 SystemInfo 的所有字段拼接为确定性字符串后计算哈希
func computeStaticHash(sys *sharedmodel.SystemInfo) string {
	data := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s",
		sys.OS, sys.Arch, sys.Kernel, sys.Hostname,
		sys.AgentVersion, sys.Virtualization, sys.Distro)
	h := sha256.Sum256([]byte(data))
	return hex.EncodeToString(h[:])
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

	// P0-1: Ping 数据变化时失效预序列化缓存
	m.invalidateDashboardCache()
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

// GetAllAgents 获取所有 Agent 完整元数据（复用 5 秒 TTL 的列表缓存）
// 供告警引擎的 traffic_quota / expire_days 指标读取 NodeGet 风格元数据
func (m *MonitorService) GetAllAgents() []model.Agent {
	m.mu.RLock()
	cacheAge := time.Since(m.agentListCacheAt)
	agents := m.agentListCache
	m.mu.RUnlock()

	if cacheAge > 5*time.Second || agents == nil {
		m.agentListCacheLock.Lock()
		m.mu.RLock()
		cacheAge = time.Since(m.agentListCacheAt)
		agents = m.agentListCache
		m.mu.RUnlock()
		if cacheAge > 5*time.Second || agents == nil {
			if fresh, err := m.agentRepo.List(); err == nil {
				m.mu.Lock()
				m.agentListCache = fresh
				m.agentListCacheAt = time.Now()
				m.mu.Unlock()
				agents = fresh
			}
		}
		m.agentListCacheLock.Unlock()
	}
	return agents
}

// SetHeartbeatTimeout 更新离线判定宽限期（设置变更时由 SettingsService 回调触发）
func (m *MonitorService) SetHeartbeatTimeout(d time.Duration) {
	if d < 30*time.Second {
		d = 30 * time.Second
	}
	m.heartbeatTimeout.Store(int64(d))
	log.Printf("[Monitor] 心跳宽限期已更新: %v", d)
}

// CheckHeartbeatTimeout 检查心跳超时（宽限期从设置服务动态读取）
func (m *MonitorService) CheckHeartbeatTimeout(timeout time.Duration) {
	if v := m.heartbeatTimeout.Load(); v > 0 {
		timeout = time.Duration(v)
	}
	m.mu.Lock()

	now := time.Now()
	// 先在锁内收集超时 Agent ID 列表、关闭连接并清理 map 条目
	var timedOut []int64
	for agentID, conn := range m.connections {
		if now.Sub(conn.LastSeen) > timeout {
			log.Printf("Agent %d 心跳超时，断开连接", agentID)
			conn.Conn.Close()
			delete(m.connections, agentID)
			// 清理 lastDBUpdate 记录，防止内存泄漏
			delete(m.lastDBUpdate, agentID)
			timedOut = append(timedOut, agentID)
		}
	}

	m.mu.Unlock()

	// 清理静态数据哈希缓存，防止内存泄漏
	m.staticHashMu.Lock()
	for _, agentID := range timedOut {
		delete(m.staticHashCache, agentID)
	}
	m.staticHashMu.Unlock()

	// 在锁外执行数据库写入，避免持锁阻塞监控服务
	for _, agentID := range timedOut {
		if err := m.agentRepo.UpdateOnlineStatus(agentID, false); err != nil {
			log.Printf("[Monitor] UpdateOnlineStatus failed for agent %d: %v", agentID, err)
		}
		// 更新 last_seen 时间戳（标记离线）
		_ = m.agentRepo.UpdateLastSeen(agentID, false)
	}
}

// StartHeartbeatChecker 启动心跳检查器
func (m *MonitorService) StartHeartbeatChecker(timeout time.Duration) {
	m.ticker = time.NewTicker(30 * time.Second)
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
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

// Stop 停止监控服务（停止心跳检查器，flush 批量写入缓冲）
func (m *MonitorService) Stop() {
	m.stopOnce.Do(func() {
		if m.ticker != nil {
			m.ticker.Stop()
		}
		close(m.stopCh)
		m.wg.Wait()
		// 优雅关闭 BatchWriter，flush 剩余数据
		if m.batchWriter != nil {
			m.batchWriter.FlushAndShutdown()
			log.Println("BatchWriter 已关闭")
		}
	})
}

// GetDashboardData 获取仪表盘数据
func (m *MonitorService) GetDashboardData() []DashboardItem {
	// 使用缓存的 agent 列表（5 秒 TTL），避免 Dashboard WS 每次推送都查询数据库
	// 当 200 个 WS 客户端每 3 秒推送一次时，可将 DB 查询从 ~67 次/秒降至 ~0.2 次/秒
	m.mu.RLock()
	cacheAge := time.Since(m.agentListCacheAt)
	agents := m.agentListCache
	m.mu.RUnlock()

	if cacheAge > 5*time.Second || agents == nil {
		// 使用独立锁防止惊群效应：多个 goroutine 同时发现缓存过期时，
		// 只有第一个执行 DB 查询，其余等待后直接使用刷新后的缓存
		m.agentListCacheLock.Lock()
		// double-check：等待期间可能已有其他 goroutine 完成刷新
		m.mu.RLock()
		cacheAge = time.Since(m.agentListCacheAt)
		agents = m.agentListCache
		m.mu.RUnlock()
		if cacheAge > 5*time.Second || agents == nil {
			if fresh, err := m.agentRepo.List(); err == nil {
				m.mu.Lock()
				m.agentListCache = fresh
				m.agentListCacheAt = time.Now()
				m.mu.Unlock()
				agents = fresh
			}
		}
		m.agentListCacheLock.Unlock()
	}

	agentMap := make(map[int64]*model.Agent, len(agents))
	for i := range agents {
		agentMap[agents[i].ID] = &agents[i]
	}

	// v1.1: 月度流量合计（60 秒缓存），用于卡片月流量进度条
	monthlyTraffic := m.GetMonthlyTraffic()
	now := time.Now()

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
			CPUModel:     p.CPUModel,
			CPUCores:     p.CPUCores,
			Mem:          p.Mem,
			MemTotal:     p.MemTotal,
			MemUsed:      p.MemUsed,
			SwapTotal:    p.SwapTotal,
			SwapUsed:     p.SwapUsed,
			NetRx:        p.NetRx,
			NetTx:        p.NetTx,
			TotalRx:      p.TotalRx,
			TotalTx:      p.TotalTx,
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
		TimeOffset:   p.TimeOffset,
		Temperature:  p.Temperature,
		Processes:    p.Processes,
		Timestamp:    p.Timestamp,
	}

		// 补充 hostname, display_name, os, arch, agent_version 及 NodeGet 风格元数据
		if agent := agentMap[agentID]; agent != nil {
			item.Hostname = agent.Hostname
			item.DisplayName = agent.DisplayName
			item.OS = agent.OS
			item.Arch = agent.Arch
			item.AgentVersion = agent.AgentVersion
			item.Tags = agent.Tags
			item.Region = agent.Region
			item.CountryCode = agent.CountryCode
			item.ISP = agent.ISP
			item.ExpiresAt = agent.ExpiresAt
			item.ExpiresInDays = calcExpiresInDays(agent.ExpiresAt, now)
			item.PriceAmount = agent.PriceAmount
			item.PriceCurrency = agent.PriceCurrency
			item.PriceCycle = agent.PriceCycle
			item.TrafficQuotaBytes = agent.TrafficQuotaBytes
			item.IPv4 = agent.IPv4
			item.IPv6 = agent.IPv6
			if monthlyTraffic != nil {
				if agg, ok := monthlyTraffic[agentID]; ok {
					item.MonthlyRx = agg.Rx
					item.MonthlyTx = agg.Tx
				}
			}
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

// calcExpiresInDays 计算距到期的剩余天数（向上取整），nil 返回 nil
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

// DashboardItem 仪表盘数据项（完整，含 Processes/Disks/TimeOffset，用于详情页 REST API）
type DashboardItem struct {
	AgentID      int64                    `json:"agent_id"`
	Hostname     string                   `json:"hostname"`
	DisplayName  string                   `json:"display_name"`
	OS           string                   `json:"os"`
	Arch         string                   `json:"arch"`
	AgentVersion string                   `json:"agent_version"`
	Online       bool                     `json:"online"`
	CPU          float64                  `json:"cpu"`
	CPUModel     string                   `json:"cpu_model"`
	CPUCores     int                      `json:"cpu_cores"`
	Mem          float64                  `json:"mem"`
	MemTotal     uint64                   `json:"mem_total"`
	MemUsed      uint64                   `json:"mem_used"`
	SwapTotal    uint64                   `json:"swap_total"`
	SwapUsed     uint64                   `json:"swap_used"`
	NetRx        uint64                   `json:"net_rx"`
	NetTx        uint64                   `json:"net_tx"`
	TotalRx      uint64                   `json:"total_rx"`
	TotalTx      uint64                   `json:"total_tx"`
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
	TimeOffset   int64                    `json:"time_offset"`
	Temperature  float64                  `json:"temperature"`
	Processes    []sharedmodel.ProcessInfo `json:"processes"`
	Timestamp    int64                    `json:"timestamp"`
	// NodeGet 风格元数据（管理员设置，Agent 上报不覆盖）
	Tags              string     `json:"tags"`                 // 逗号分隔标签
	Region            string     `json:"region"`               // 位置："上海"/"Tokyo"
	CountryCode       string     `json:"country_code"`         // 国家代码："CN"/"JP"（旗帜+地图）
	ISP               string     `json:"isp"`                  // 供应商备注
	ExpiresAt         *time.Time `json:"expires_at"`           // 到期时间（null=永不过期）
	ExpiresInDays     *int       `json:"expires_in_days"`      // 剩余天数（null=永不过期）
	PriceAmount       float64    `json:"price_amount"`         // 周期费用
	PriceCurrency     string     `json:"price_currency"`       // 币种: CNY/USD/EUR/JPY
	PriceCycle        string     `json:"price_cycle"`          // 周期: monthly/yearly
	TrafficQuotaBytes int64      `json:"traffic_quota_bytes"`  // 月流量配额（0=不限）
	MonthlyRx         uint64     `json:"monthly_rx"`           // 当月累计下行
	MonthlyTx         uint64     `json:"monthly_tx"`           // 当月累计上行
	IPv4              string     `json:"ipv4"`                 // 出口 IPv4
	IPv6              string     `json:"ipv6"`                 // 出口 IPv6
}

// DashboardSummary 仪表盘摘要数据项（P1-1: 轻量级，用于 WS 推送）
// 排除 Processes、Disks 详情、TimeOffset — 这些字段仅详情页使用，通过 REST API 按需获取
type DashboardSummary struct {
	AgentID      int64                    `json:"agent_id"`
	Hostname     string                   `json:"hostname"`
	DisplayName  string                   `json:"display_name"`
	OS           string                   `json:"os"`
	Arch         string                   `json:"arch"`
	AgentVersion string                   `json:"agent_version"`
	Online       bool                     `json:"online"`
	CPU          float64                  `json:"cpu"`
	CPUModel     string                   `json:"cpu_model"`
	CPUCores     int                      `json:"cpu_cores"`
	Mem          float64                  `json:"mem"`
	MemTotal     uint64                   `json:"mem_total"`
	MemUsed      uint64                   `json:"mem_used"`
	SwapTotal    uint64                   `json:"swap_total"`
	SwapUsed     uint64                   `json:"swap_used"`
	NetRx        uint64                   `json:"net_rx"`
	NetTx        uint64                   `json:"net_tx"`
	TotalRx      uint64                   `json:"total_rx"`
	TotalTx      uint64                   `json:"total_tx"`
	Load1        float64                  `json:"load_1"`
	Load5        float64                  `json:"load_5"`
	Load15       float64                  `json:"load_15"`
	Uptime       uint64                   `json:"uptime"`
	DiskUsage    float64                  `json:"disk_usage"`
	TCPConns     int                      `json:"tcp_connections"`
	UDPConns     int                      `json:"udp_connections"`
	ProcessCount int                      `json:"process_count"`
	PingData     []sharedmodel.PingResult `json:"ping_data"`
	Temperature  float64                  `json:"temperature"`
	Timestamp    int64                    `json:"timestamp"`
	// NodeGet 风格元数据（管理员设置，Agent 上报不覆盖）
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

// toSummary 将 DashboardItem 转换为 DashboardSummary（剥离重型字段）
func (item DashboardItem) toSummary() DashboardSummary {
	return DashboardSummary{
		AgentID:      item.AgentID,
		Hostname:     item.Hostname,
		DisplayName:  item.DisplayName,
		OS:           item.OS,
		Arch:         item.Arch,
		AgentVersion: item.AgentVersion,
		Online:       item.Online,
		CPU:          item.CPU,
		CPUModel:     item.CPUModel,
		CPUCores:     item.CPUCores,
		Mem:          item.Mem,
		MemTotal:     item.MemTotal,
		MemUsed:      item.MemUsed,
		SwapTotal:    item.SwapTotal,
		SwapUsed:     item.SwapUsed,
		NetRx:        item.NetRx,
		NetTx:        item.NetTx,
		TotalRx:      item.TotalRx,
		TotalTx:      item.TotalTx,
		Load1:        item.Load1,
		Load5:        item.Load5,
		Load15:       item.Load15,
		Uptime:       item.Uptime,
		DiskUsage:    item.DiskUsage,
		TCPConns:     item.TCPConns,
		UDPConns:     item.UDPConns,
		ProcessCount: item.ProcessCount,
		PingData:     item.PingData,
		Temperature:  item.Temperature,
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
		IPv4:              item.IPv4,
		IPv6:              item.IPv6,
	}
}

// invalidateDashboardCache 失效预序列化缓存（数据变化时调用）
func (m *MonitorService) invalidateDashboardCache() {
	m.dashCacheMu.Lock()
	m.dashCacheJSON = nil
	m.dashCacheMu.Unlock()
	m.pubDashCacheMu.Lock()
	m.pubDashCacheJSON = nil
	m.pubDashCacheMu.Unlock()
}

// GetDashboardJSON 获取管理员推送的预序列化 JSON（P0-1: 所有 WS 客户端共享同一份）
// 缓存有效期 3 秒，过期或失效后首次调用时重建
func (m *MonitorService) GetDashboardJSON() []byte {
	m.dashCacheMu.Lock()
	defer m.dashCacheMu.Unlock()

	// 缓存有效（3 秒内且非 nil）
	if m.dashCacheJSON != nil && time.Since(m.dashCacheTime) < 3*time.Second {
		return m.dashCacheJSON
	}

	// 重建缓存
	items := m.GetDashboardData()
	summaries := make([]DashboardSummary, 0, len(items))
	for _, item := range items {
		summaries = append(summaries, item.toSummary())
	}

	message := struct {
		Type    string             `json:"type"`
		Servers []DashboardSummary `json:"servers"`
	}{
		Type:    "dashboard_update",
		Servers: summaries,
	}

	data, err := json.Marshal(message)
	if err != nil {
		log.Printf("Dashboard JSON 序列化失败: %v", err)
		return nil
	}

	m.dashCacheJSON = data
	m.dashCacheTime = time.Now()
	return data
}

// GetPublicDashboardJSON 获取公开推送的预序列化 JSON（过滤敏感字段 + 摘要）
func (m *MonitorService) GetPublicDashboardJSON() []byte {
	m.pubDashCacheMu.Lock()
	defer m.pubDashCacheMu.Unlock()

	// 缓存有效（3 秒内且非 nil）
	if m.pubDashCacheJSON != nil && time.Since(m.pubDashCacheTime) < 3*time.Second {
		return m.pubDashCacheJSON
	}

	// 重建缓存
	items := m.GetDashboardData()

	type PublicSummary struct {
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
		// 注意: 不包含 TCPConns/UDPConns/ProcessCount/Disks 详情，
		// 与 HTTP 公开端点 (HandlePublicServers/HandlePublicDashboard/publicHistoryPoint) 保持一致，防止泄露连接数与进程信息
		PingData    []sharedmodel.PingResult `json:"ping_data"`
		Temperature float64                  `json:"temperature"`
		Timestamp   int64                    `json:"timestamp"`
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

	publicItems := make([]PublicSummary, 0, len(items))
	for _, item := range items {
		// 过滤 PingData 中的 Target 字段
		publicPing := make([]sharedmodel.PingResult, 0, len(item.PingData))
		for _, p := range item.PingData {
			publicPing = append(publicPing, sharedmodel.PingResult{
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
		})
		}

		publicItems = append(publicItems, PublicSummary{
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
			PingData:     publicPing,
			Temperature:  item.Temperature,
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

	message := struct {
		Type    string          `json:"type"`
		Servers []PublicSummary `json:"servers"`
	}{
		Type:    "dashboard_update",
		Servers: publicItems,
	}

	data, err := json.Marshal(message)
	if err != nil {
		log.Printf("Public Dashboard JSON 序列化失败: %v", err)
		return nil
	}

	m.pubDashCacheJSON = data
	m.pubDashCacheTime = time.Now()
	return data
}
