package service

import (
	"fmt"
	"log"
	"sync"
	"time"

	sharedmodel "github.com/server-probe/shared/model"
)

// DataValidator 数据合理性校验
type DataValidator struct {
	mu             sync.Mutex
	lastReportTime map[int64]time.Time // Agent ID -> 上次上报时间
	ticker         *time.Ticker
	stopCh         chan struct{}
}

// NewDataValidator 创建数据校验器
func NewDataValidator() *DataValidator {
	return &DataValidator{
		lastReportTime: make(map[int64]time.Time),
		stopCh:         make(chan struct{}),
	}
}

// StartCleanupTask 启动定期清理任务，清理过期的 lastReportTime 条目
// 防止已断开连接的 Agent 条目导致内存泄漏
func (v *DataValidator) StartCleanupTask() {
	v.ticker = time.NewTicker(10 * time.Minute)
	go func() {
		for {
			select {
			case <-v.ticker.C:
				v.cleanupStaleEntries()
			case <-v.stopCh:
				return
			}
		}
	}()
	log.Println("数据校验器清理任务已启动（每 10 分钟清理一次）")
}

// Stop 停止数据校验器
func (v *DataValidator) Stop() {
	if v.ticker != nil {
		v.ticker.Stop()
	}
	close(v.stopCh)
}

// cleanupStaleEntries 清理超过 30 分钟未上报的 Agent 条目
func (v *DataValidator) cleanupStaleEntries() {
	v.mu.Lock()
	defer v.mu.Unlock()

	cutoff := time.Now().Add(-30 * time.Minute)
	for agentID, lastTime := range v.lastReportTime {
		if lastTime.Before(cutoff) {
			delete(v.lastReportTime, agentID)
		}
	}
}

// ValidateMetricData 校验监控数据
func (v *DataValidator) ValidateMetricData(agentID int64, data *sharedmodel.MetricData) error {
	// 校验 CPU 使用率
	if data.CPU.Usage < 0 || data.CPU.Usage > 100 {
		return fmt.Errorf("CPU 使用率超出范围: %f", data.CPU.Usage)
	}

	// 校验内存使用率
	if data.Memory.Total > 0 {
		memUsage := float64(data.Memory.Used) / float64(data.Memory.Total) * 100
		if memUsage < 0 || memUsage > 100 {
			return fmt.Errorf("内存使用率超出范围: %f", memUsage)
		}
		if data.Memory.Used > data.Memory.Total {
			return fmt.Errorf("内存已用大于总量")
		}
	}

	// 校验 Swap
	if data.Memory.SwapTotal > 0 && data.Memory.SwapUsed > data.Memory.SwapTotal {
		return fmt.Errorf("Swap 已用大于总量")
	}

	// 校验磁盘使用率
	for _, disk := range data.Disks {
		if disk.Total > 0 && disk.Used > disk.Total {
			return fmt.Errorf("磁盘 %s 已用大于总量", disk.Device)
		}
	}

	return nil
}

// ValidatePingResult 校验 Ping 探测结果
func (v *DataValidator) ValidatePingResult(result *sharedmodel.PingResult) error {
	if result.AvgLatency < 0 || result.AvgLatency > 60000 {
		return fmt.Errorf("延迟超出范围: %f", result.AvgLatency)
	}
	if result.MinLatency < 0 || result.MinLatency > 60000 {
		return fmt.Errorf("最小延迟超出范围: %f", result.MinLatency)
	}
	if result.MaxLatency < 0 || result.MaxLatency > 60000 {
		return fmt.Errorf("最大延迟超出范围: %f", result.MaxLatency)
	}
	if result.Loss < 0 || result.Loss > 100 {
		return fmt.Errorf("丢包率超出范围: %f", result.Loss)
	}
	if result.PacketsSent < 0 {
		return fmt.Errorf("发送包数不能为负: %d", result.PacketsSent)
	}
	if result.PacketsRecv < 0 || result.PacketsRecv > result.PacketsSent {
		return fmt.Errorf("接收包数无效: sent=%d, recv=%d", result.PacketsSent, result.PacketsRecv)
	}
	return nil
}

// CheckReportFrequency 检查上报频率
// 期望每 3 秒上报一次，允许 ±1 秒误差
// 过快（< 1 秒）拒绝，过慢（> 90 秒）标记离线
func (v *DataValidator) CheckReportFrequency(agentID int64) error {
	now := time.Now()

	v.mu.Lock()
	defer v.mu.Unlock()

	if lastTime, ok := v.lastReportTime[agentID]; ok {
		interval := now.Sub(lastTime)
		if interval < time.Second {
			return fmt.Errorf("上报过于频繁: 间隔 %v", interval)
		}
	}

	v.lastReportTime[agentID] = now
	return nil
}

// CheckDataSize 检查数据大小（单次上报不超过 10KB）
func (v *DataValidator) CheckDataSize(data []byte) error {
	if len(data) > 10*1024 {
		return fmt.Errorf("数据大小超过限制: %d bytes", len(data))
	}
	return nil
}
