package service

import (
	"encoding/json"
	"log"
	"math"
	"sync"
	"time"

	"github.com/server-probe/server/internal/model"
	"github.com/server-probe/server/internal/repository"
	sharedmodel "github.com/server-probe/shared/model"
)

// AggregationService 数据聚合落盘服务
type AggregationService struct {
	monitor     *MonitorService
	recordRepo  *repository.RecordRepository
	hourlyRepo  *repository.HourlyRepository // 小时级 rollup 写入
	agentRepo   *repository.AgentRepository
	trafficRepo *repository.TrafficRepository // P0-1: 流量统计
	ticker      *time.Ticker
	stopCh      chan struct{}
	stopOnce    sync.Once
	wg          sync.WaitGroup // 跟踪后台 goroutine
	// P0-1: 上次聚合时的累计流量值，用于计算增量
	prevTraffic map[int64]struct{ Rx, Tx uint64 }
	trafficMu   sync.Mutex
	// 小时 rollup 增量起点（agentID → 已聚合的最后一个小時起始时间戳）。
	// 仅被 aggregate() 所在的单 goroutine 访问，无需加锁
	lastRolled map[int64]int64
}

// NewAggregationService 创建数据聚合服务
func NewAggregationService(
	monitor *MonitorService,
	recordRepo *repository.RecordRepository,
	hourlyRepo *repository.HourlyRepository,
	agentRepo *repository.AgentRepository,
	trafficRepo *repository.TrafficRepository,
) *AggregationService {
	return &AggregationService{
		monitor:     monitor,
		recordRepo:  recordRepo,
		hourlyRepo:  hourlyRepo,
		agentRepo:   agentRepo,
		trafficRepo: trafficRepo,
		stopCh:      make(chan struct{}),
		prevTraffic: make(map[int64]struct{ Rx, Tx uint64 }),
		lastRolled:  make(map[int64]int64),
	}
}

// Start 启动聚合服务
func (s *AggregationService) Start() {
	s.ticker = time.NewTicker(5 * time.Minute)

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		// 首次立即执行
		s.aggregate()

		for {
			select {
			case <-s.ticker.C:
				s.aggregate()
			case <-s.stopCh:
				return
			}
		}
	}()

	log.Println("数据聚合服务已启动（每 5 分钟聚合一次）")
}

// Stop 停止聚合服务
func (s *AggregationService) Stop() {
	s.stopOnce.Do(func() {
		if s.ticker != nil {
			s.ticker.Stop()
		}
		close(s.stopCh)
		s.wg.Wait()
	})
}

// aggregate 执行一次数据聚合
func (s *AggregationService) aggregate() {
	// 获取所有 Agent
	agents, err := s.agentRepo.List()
	if err != nil {
		log.Printf("聚合失败：获取 Agent 列表失败: %v", err)
		return
	}

	// 对齐到 5 分钟边界（向下取整），保证聚合窗口固定且连续，
	// 避免服务启动时刻导致窗口漂移与相邻窗口重叠/留空
	now := time.Now().Unix()
	now = now - now%300
	records := make([]model.MetricRecord, 0, len(agents))
	// 收集每个 Agent 的最新累计流量，避免流量循环中重复查询 RingBuffer
	type trafficInfo struct{ rx, tx uint64 }
	agentTraffic := make(map[int64]trafficInfo, len(agents))

	for _, agent := range agents {
		rb := s.monitor.GetRingBuffer(agent.ID)
		if rb == nil {
			// 从未上线过的 Agent（后台直接创建、尚未连接）不产生记录
			continue
		}

		// 获取最近 5 分钟的数据
		points := rb.GetByTimeRange(now-300, now)
		if len(points) == 0 {
			// 聚合周期内无数据点 = Agent 离线，写入离线占位记录
			// 保证在线率时间线在离线时段也有数据点（参考 Komari 的存储方式）
			records = append(records, model.MetricRecord{
				AgentID:   agent.ID,
				Timestamp: now,
				Offline:   1,
			})
			continue
		}

		// 计算平均值/累计值
		var cpuSum, memSum, load1Sum, load5Sum, load15Sum float64
		var uptimeMax uint64
		var netRxSum, netTxSum uint64
		var tcpConnsSum, udpConnsSum, processCountSum int
		var pingData []sharedmodel.PingResult

		for _, p := range points {
			cpuSum += p.CPU
			memSum += p.Mem
			load1Sum += p.Load1
			load5Sum += p.Load5
			load15Sum += p.Load15
			netRxSum += p.NetRx
			netTxSum += p.NetTx
			tcpConnsSum += p.TCPConns
			udpConnsSum += p.UDPConns
			processCountSum += p.ProcessCount
			if p.Uptime > uptimeMax {
				uptimeMax = p.Uptime
			}
			// 取最后一个有效的 Ping 数据 (而非第一个)
			if len(p.PingData) > 0 {
				pingData = make([]sharedmodel.PingResult, len(p.PingData))
				copy(pingData, p.PingData)
			}
		}

		count := len(points)
		cpuAvg := cpuSum / float64(count)
		memAvg := memSum / float64(count)
		load1Avg := load1Sum / float64(count)
		load5Avg := load5Sum / float64(count)
		load15Avg := load15Sum / float64(count)
		// 固定值取最后一个有效值，不取平均
		memTotalFinal := uint64(0)
		memUsedFinal := uint64(0)
		swapTotalFinal := uint64(0)
		swapUsedFinal := uint64(0)
		for i := len(points) - 1; i >= 0; i-- {
			if points[i].MemTotal > 0 {
				memTotalFinal = points[i].MemTotal
				memUsedFinal = points[i].MemUsed
				break
			}
		}
		for i := len(points) - 1; i >= 0; i-- {
			if points[i].SwapTotal > 0 {
				swapTotalFinal = points[i].SwapTotal
				swapUsedFinal = points[i].SwapUsed
				break
			}
		}
		netRxAvg := netRxSum / uint64(count)
		netTxAvg := netTxSum / uint64(count)
		tcpConnsAvg := tcpConnsSum / count
		udpConnsAvg := udpConnsSum / count
		processCountAvg := processCountSum / count

		// 序列化磁盘数据 (使用最新数据点)
		diskData := ""
		if len(points) > 0 && len(points[len(points)-1].Disks) > 0 {
			diskBytes, _ := json.Marshal(points[len(points)-1].Disks)
			diskData = string(diskBytes)
		}

		// 序列化 Ping 数据
		pingStr := ""
		if len(pingData) > 0 {
			pingBytes, _ := json.Marshal(pingData)
			pingStr = string(pingBytes)
		}

		// 收集聚合记录（批量写入，避免逐条 INSERT）
		// P3: CPUUsage / Load 值存储为 ×10 的整数，减小存储体积并提升查询效率
		records = append(records, model.MetricRecord{
			AgentID:      agent.ID,
			Timestamp:    now,
			CPUUsage:     int(math.Round(cpuAvg * 10)),
			MemUsage:     memAvg,
			MemTotal:     memTotalFinal,
			MemUsed:      memUsedFinal,
			SwapTotal:    swapTotalFinal,
			SwapUsed:     swapUsedFinal,
			DiskUsage:    diskData,
			NetRx:        int64(netRxAvg),
			NetTx:        int64(netTxAvg),
			TCPConns:     tcpConnsAvg,
			UDPConns:     udpConnsAvg,
			Load1:        int(math.Round(load1Avg * 10)),
			Load5:        int(math.Round(load5Avg * 10)),
			Load15:       int(math.Round(load15Avg * 10)),
			Uptime:       uptimeMax,
			ProcessCount: processCountAvg,
			PingData:     pingStr,
		})

		// 收集最新累计流量供流量统计使用（避免重复查询 RingBuffer）
		lastPoint := points[len(points)-1]
		agentTraffic[agent.ID] = trafficInfo{rx: lastPoint.TotalRx, tx: lastPoint.TotalTx}
	}

	// 批量写入聚合记录（通过 BatchWriter 异步缓冲，每条记录推入 channel）
	if len(records) > 0 {
		for i := range records {
			if err := s.recordRepo.CreateRecord(&records[i]); err != nil {
				log.Printf("写入聚合数据失败 (agent_id=%d): %v", records[i].AgentID, err)
			}
		}
	}

	// P0-1: 流量统计 — 计算增量并写入当日流量记录
	if s.trafficRepo != nil {
		s.trafficMu.Lock()
		defer s.trafficMu.Unlock()
		today := time.Now().Format("2006-01-02")

		// 清理已删除 Agent 的 prevTraffic 条目，防止内存泄漏
		activeAgentIDs := make(map[int64]bool, len(agents))
		for _, agent := range agents {
			activeAgentIDs[agent.ID] = true
		}
		for agentID := range s.prevTraffic {
			if !activeAgentIDs[agentID] {
				delete(s.prevTraffic, agentID)
			}
		}

		for agentID, traffic := range agentTraffic {
			prev, hasPrev := s.prevTraffic[agentID]
			var rxDelta, txDelta uint64
			if hasPrev {
				// 正常情况：当前值 >= 上次值，差值为增量
				if traffic.rx >= prev.Rx {
					rxDelta = traffic.rx - prev.Rx
				} else {
					// 计数器重置（Agent 重启）：当前值 < 上次值，当前值即为增量
					rxDelta = traffic.rx
				}
				if traffic.tx >= prev.Tx {
					txDelta = traffic.tx - prev.Tx
				} else {
					txDelta = traffic.tx
				}
			}
			// 更新上次值
			s.prevTraffic[agentID] = struct{ Rx, Tx uint64 }{traffic.rx, traffic.tx}
			// 有增量才写入
			if rxDelta > 0 || txDelta > 0 {
				if err := s.trafficRepo.UpsertDailyTraffic(agentID, today, rxDelta, txDelta); err != nil {
					log.Printf("写入流量统计失败 (agent_id=%d): %v", agentID, err)
				}
			}
		}
	}

	// 小时级 rollup：从 5 分钟层聚合生成长范围查询数据（幂等，任意频率重跑）
	s.rollupHourly(agents, now)
}

// rollupHourly 将 5 分钟层记录聚合为小时级记录
//
// 增量语义：lastRolled 记录每个 Agent 已聚合的最后一个小時；启动时从数据库恢复
// （无记录则从最早的 5 分钟记录回填），崩溃/停机后自动补算缺失小时（upsert 覆盖）。
// 幂等性：每小时由该小时全部 5 分钟行全量重算，ON CONFLICT 覆盖写入。
//
// 时间窗口约定：5 分钟行的时间戳是窗口右边界（T 行覆盖 (T-300, T]），
// 故小时 H 的源行范围为 (H, H+3600]，即时间戳 H+300 ... H+3600 共 12 行。
func (s *AggregationService) rollupHourly(agents []model.Agent, now int64) {
	// 可聚合的最新小时：要求该小时结束后再经过一个聚合周期（300s），
	// 确保小时末行（时间戳 = H+3600，由该 tick 提交）已从 BatchWriter 异步落盘
	lastReady := (now-300)/3600*3600 - 3600
	if lastReady < 0 {
		return
	}

	// 清理已删除 Agent 的 lastRolled 缓存，防止内存泄漏
	if len(s.lastRolled) > 0 {
		active := make(map[int64]bool, len(agents))
		for _, a := range agents {
			active[a.ID] = true
		}
		for id := range s.lastRolled {
			if !active[id] {
				delete(s.lastRolled, id)
			}
		}
	}

	for _, agent := range agents {
		last, ok := s.lastRolled[agent.ID]
		if !ok {
			var has bool
			var err error
			// 恢复增量起点：小时表已有记录则续算，否则从最早 5 分钟记录回填
			last, has, err = s.hourlyRepo.GetLastTimestamp(agent.ID)
			if err != nil {
				log.Printf("小时聚合失败：查询增量起点失败 (agent_id=%d): %v", agent.ID, err)
				continue
			}
			if !has {
				first, hasFirst, err := s.recordRepo.GetFirstTimestamp(agent.ID)
				if err != nil {
					log.Printf("小时聚合失败：查询回填起点失败 (agent_id=%d): %v", agent.ID, err)
					continue
				}
				if !hasFirst {
					// 无任何 5 分钟数据（从未上线的 Agent）：跳过，不写占位
					continue
				}
				// 回填从最早记录所属的小时窗口开始。lastRolled 语义为
				// "已聚合的最后一小时"，故初始化为首个待聚合小时的前一小时；
				// 用 first-1 取窗口：T 行覆盖 (T-300, T]，T 恰为整点时
				// 该行属于上一小时的窗口
				last = (first-1)-(first-1)%3600 - 3600
			}
			s.lastRolled[agent.ID] = last
		}

		start := last + 3600
		// 源数据保留期清理会造成历史空洞（停机超过保留期），早于现存最早
		// 5 分钟记录的小时无法区分"离线"与"未采集"，跳过而非伪造离线占位。
		// 同样以 first-1 取窗口归属，与回填起点口径一致
		if firstSrc, hasSrc, err := s.recordRepo.GetFirstTimestamp(agent.ID); err == nil && hasSrc {
			if h := (firstSrc - 1) - (firstSrc-1)%3600; h > start {
				start = h
			}
		}

		for hour := start; hour <= lastReady; hour += 3600 {
			rec, err := s.computeHourly(agent.ID, hour)
			if err != nil {
				log.Printf("小时聚合失败 (agent_id=%d, hour=%d): %v", agent.ID, hour, err)
				break
			}
			if err := s.hourlyRepo.UpsertHourly(rec); err != nil {
				log.Printf("小时聚合写入失败 (agent_id=%d, hour=%d): %v", agent.ID, hour, err)
				break
			}
			s.lastRolled[agent.ID] = hour
		}
	}
}

// computeHourly 从 5 分钟层记录计算一条小时聚合记录
// 均值/极值仅统计在线行（离线占位行指标为零值）；固定值取最后一条在线行
func (s *AggregationService) computeHourly(agentID, hour int64) (*model.MetricRecordHourly, error) {
	rows, err := s.recordRepo.GetByAgentAndTimeRange(agentID, hour+1, hour+3600)
	if err != nil {
		return nil, err
	}

	rec := &model.MetricRecordHourly{
		AgentID:     agentID,
		Timestamp:   hour,
		SampleCount: len(rows),
	}
	if len(rows) == 0 {
		// Agent 离线期间 5 分钟层持续写占位行，此处 0 行仅出现在
		// 服务停机窗口或从未采集的时段；占位保证小时粒度时间线连续
		rec.Offline = 1
		return rec, nil
	}

	online := make([]model.MetricRecord, 0, len(rows))
	offlineSamples := 0
	for i := range rows {
		if rows[i].Offline == 1 {
			offlineSamples++
		} else {
			online = append(online, rows[i])
		}
	}
	rec.OfflineSamples = offlineSamples
	if len(online) == 0 || offlineSamples*2 >= len(rows) {
		// 多数规则：离线样本过半 → 该小时判定为离线
		rec.Offline = 1
		if len(online) == 0 {
			return rec, nil
		}
	}

	n := len(online)
	var cpuSum, memSum, load1Sum, load5Sum, load15Sum float64
	var netRxSum, netTxSum int64
	cpuMin, cpuMax := online[0].CPUUsage, online[0].CPUUsage
	memMin, memMax := online[0].MemUsage, online[0].MemUsage
	load1Max := online[0].Load1
	netRxMax, netTxMax := online[0].NetRx, online[0].NetTx

	for i := range online {
		r := &online[i]
		cpuSum += float64(r.CPUUsage)
		memSum += r.MemUsage
		load1Sum += float64(r.Load1)
		load5Sum += float64(r.Load5)
		load15Sum += float64(r.Load15)
		netRxSum += r.NetRx
		netTxSum += r.NetTx
		if r.CPUUsage < cpuMin {
			cpuMin = r.CPUUsage
		}
		if r.CPUUsage > cpuMax {
			cpuMax = r.CPUUsage
		}
		if r.MemUsage < memMin {
			memMin = r.MemUsage
		}
		if r.MemUsage > memMax {
			memMax = r.MemUsage
		}
		if r.Load1 > load1Max {
			load1Max = r.Load1
		}
		if r.NetRx > netRxMax {
			netRxMax = r.NetRx
		}
		if r.NetTx > netTxMax {
			netTxMax = r.NetTx
		}
	}

	fn := float64(n)
	rec.CPUUsage = int(math.Round(cpuSum / fn))
	rec.MemUsage = memSum / fn
	rec.Load1 = int(math.Round(load1Sum / fn))
	rec.Load5 = int(math.Round(load5Sum / fn))
	rec.Load15 = int(math.Round(load15Sum / fn))
	rec.NetRx = netRxSum / int64(n)
	rec.NetTx = netTxSum / int64(n)
	rec.CPUMin = cpuMin
	rec.CPUMax = cpuMax
	rec.MemMin = memMin
	rec.MemMax = memMax
	rec.Load1Max = load1Max
	rec.NetRxMax = netRxMax
	rec.NetTxMax = netTxMax

	// 固定值取最后一条在线行（与 5 分钟层"取最后有效值"口径一致）
	last := &online[n-1]
	rec.MemTotal = last.MemTotal
	rec.MemUsed = last.MemUsed
	rec.SwapTotal = last.SwapTotal
	rec.SwapUsed = last.SwapUsed
	rec.DiskUsage = last.DiskUsage
	rec.Uptime = last.Uptime
	rec.ProcessCount = last.ProcessCount
	rec.TCPConns = last.TCPConns
	rec.UDPConns = last.UDPConns
	rec.PingData = aggregateHourlyPing(online)

	return rec, nil
}

// aggregateHourlyPing 将多行 5 分钟记录的 ping 结果按目标聚合
// 延迟/抖动/丢包率求均值；Name/Method 等配置字段取最新一次出现（最新目标配置）
func aggregateHourlyPing(rows []model.MetricRecord) string {
	type pingAcc struct {
		avg, min, max, jitter, loss float64
		count                       int
		tmpl                        sharedmodel.PingResult
	}
	accs := make(map[string]*pingAcc)
	var order []string

	for i := range rows {
		if rows[i].PingData == "" {
			continue
		}
		var pings []sharedmodel.PingResult
		if err := json.Unmarshal([]byte(rows[i].PingData), &pings); err != nil {
			continue
		}
		for j := range pings {
			p := pings[j]
			a, ok := accs[p.Target]
			if !ok {
				a = &pingAcc{}
				accs[p.Target] = a
				order = append(order, p.Target)
			}
			a.avg += p.AvgLatency
			a.min += p.MinLatency
			a.max += p.MaxLatency
			a.jitter += p.Jitter
			a.loss += p.Loss
			a.count++
			a.tmpl = p
		}
	}

	if len(order) == 0 {
		return ""
	}

	out := make([]sharedmodel.PingResult, 0, len(order))
	for _, target := range order {
		a := accs[target]
		t := a.tmpl
		c := float64(a.count)
		t.AvgLatency = a.avg / c
		t.MinLatency = a.min / c
		t.MaxLatency = a.max / c
		t.Jitter = a.jitter / c
		t.Loss = a.loss / c
		out = append(out, t)
	}

	b, err := json.Marshal(out)
	if err != nil {
		return ""
	}
	return string(b)
}

// CleanupExpiredData 清理过期数据（5 分钟层与小时层独立保留期）
func (s *AggregationService) CleanupExpiredData(retentionDays, hourlyRetentionDays int) {
	deleted, err := s.recordRepo.CleanupExpired(retentionDays)
	if err != nil {
		log.Printf("清理过期数据失败: %v", err)
	} else if deleted > 0 {
		log.Printf("已清理 %d 条过期数据（5 分钟层）", deleted)
	}

	if s.hourlyRepo != nil {
		deleted, err := s.hourlyRepo.CleanupExpired(hourlyRetentionDays)
		if err != nil {
			log.Printf("清理过期小时数据失败: %v", err)
		} else if deleted > 0 {
			log.Printf("已清理 %d 条过期数据（小时层）", deleted)
		}
	}
}

// StartCleanupTask 启动定时清理任务（保留天数通过函数动态读取，支持后台设置修改）
func (s *AggregationService) StartCleanupTask(retentionFn func() int, hourlyRetentionFn func() int) {
	ticker := time.NewTicker(24 * time.Hour) // 每天清理一次

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			select {
			case <-ticker.C:
				s.CleanupExpiredData(retentionFn(), hourlyRetentionFn())
			case <-s.stopCh:
				ticker.Stop()
				return
			}
		}
	}()
}
