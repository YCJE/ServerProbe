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
	agentRepo   *repository.AgentRepository
	trafficRepo *repository.TrafficRepository // P0-1: 流量统计
	ticker      *time.Ticker
	stopCh      chan struct{}
	stopOnce    sync.Once
	wg          sync.WaitGroup // 跟踪后台 goroutine
	// P0-1: 上次聚合时的累计流量值，用于计算增量
	prevTraffic map[int64]struct{ Rx, Tx uint64 }
	trafficMu   sync.Mutex
}

// NewAggregationService 创建数据聚合服务
func NewAggregationService(
	monitor *MonitorService,
	recordRepo *repository.RecordRepository,
	agentRepo *repository.AgentRepository,
	trafficRepo *repository.TrafficRepository,
) *AggregationService {
	return &AggregationService{
		monitor:     monitor,
		recordRepo:  recordRepo,
		agentRepo:   agentRepo,
		trafficRepo: trafficRepo,
		stopCh:      make(chan struct{}),
		prevTraffic: make(map[int64]struct{ Rx, Tx uint64 }),
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

	now := time.Now().Unix()
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
}

// CleanupExpiredData 清理过期数据
func (s *AggregationService) CleanupExpiredData(retentionDays int) {
	deleted, err := s.recordRepo.CleanupExpired(retentionDays)
	if err != nil {
		log.Printf("清理过期数据失败: %v", err)
		return
	}
	if deleted > 0 {
		log.Printf("已清理 %d 条过期数据", deleted)
	}
}

// StartCleanupTask 启动定时清理任务
func (s *AggregationService) StartCleanupTask(retentionDays int) {
	ticker := time.NewTicker(24 * time.Hour) // 每天清理一次

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			select {
			case <-ticker.C:
				s.CleanupExpiredData(retentionDays)
			case <-s.stopCh:
				ticker.Stop()
				return
			}
		}
	}()
}
