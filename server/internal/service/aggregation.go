package service

import (
	"encoding/json"
	"log"
	"time"

	"github.com/server-probe/server/internal/model"
	"github.com/server-probe/server/internal/repository"
)

// AggregationService 数据聚合落盘服务
type AggregationService struct {
	monitor     *MonitorService
	recordRepo  *repository.RecordRepository
	agentRepo   *repository.AgentRepository
	ticker      *time.Ticker
	stopCh      chan struct{}
}

// NewAggregationService 创建数据聚合服务
func NewAggregationService(
	monitor *MonitorService,
	recordRepo *repository.RecordRepository,
	agentRepo *repository.AgentRepository,
) *AggregationService {
	return &AggregationService{
		monitor:    monitor,
		recordRepo: recordRepo,
		agentRepo:  agentRepo,
		stopCh:     make(chan struct{}),
	}
}

// Start 启动聚合服务
func (s *AggregationService) Start() {
	s.ticker = time.NewTicker(5 * time.Minute)

	go func() {
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
	if s.ticker != nil {
		s.ticker.Stop()
	}
	close(s.stopCh)
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

	for _, agent := range agents {
		rb := s.monitor.GetRingBuffer(agent.ID)
		if rb == nil {
			continue
		}

		// 获取最近 5 分钟的数据
		points := rb.GetByTimeRange(now-300, now)
		if len(points) == 0 {
			continue
		}

		// 计算平均值
		var cpuSum, memSum float64
		var netRxSum, netTxSum int64
		var pingData []interface{}

		for _, p := range points {
			cpuSum += p.CPU
			memSum += p.Mem
			netRxSum += int64(p.NetRx)
			netTxSum += int64(p.NetTx)
			if len(p.PingData) > 0 && pingData == nil {
				// 取最后一个有效的 Ping 数据
				pingData = make([]interface{}, len(p.PingData))
				for i, pd := range p.PingData {
					pingData[i] = pd
				}
			}
		}

		count := len(points)
		cpuAvg := cpuSum / float64(count)
		memAvg := memSum / float64(count)

		// 序列化磁盘数据
		diskData := ""
		if len(points) > 0 && len(points[0].Disks) > 0 {
			diskBytes, _ := json.Marshal(points[0].Disks)
			diskData = string(diskBytes)
		}

		// 序列化 Ping 数据
		pingStr := ""
		if len(pingData) > 0 {
			pingBytes, _ := json.Marshal(pingData)
			pingStr = string(pingBytes)
		}

		// 创建聚合记录
		record := &model.MetricRecord{
			AgentID:   agent.ID,
			Timestamp: now,
			CPUUsage:  cpuAvg,
			MemUsage:  memAvg,
			DiskUsage: diskData,
			NetRx:     netRxSum / int64(count),
			NetTx:     netTxSum / int64(count),
			PingData:  pingStr,
		}

		if err := s.recordRepo.Create(record); err != nil {
			log.Printf("Agent %d 聚合数据写入失败: %v", agent.ID, err)
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

	go func() {
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
