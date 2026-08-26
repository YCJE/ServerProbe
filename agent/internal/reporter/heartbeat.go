package reporter

import (
	"log"
	"sync"
	"sync/atomic"
	"time"

	sharedmodel "github.com/server-probe/shared/model"
)

// Heartbeat 心跳维持器
type Heartbeat struct {
	client   *WSClient
	interval time.Duration
	stopCh   chan struct{}
	stopOnce sync.Once
}

// NewHeartbeat 创建心跳维持器
func NewHeartbeat(client *WSClient, interval time.Duration) *Heartbeat {
	return &Heartbeat{
		client:   client,
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

// Start 启动心跳
func (h *Heartbeat) Start() {
	go func() {
		ticker := time.NewTicker(h.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if h.client.IsConnected() {
					if err := h.client.SendHeartbeat(); err != nil {
						log.Printf("发送心跳失败: %v", err)
						// 写失败说明连接已半死，主动断开让 Run() 读循环立即触发重连，
						// 避免等到 90s 读超时才恢复
						h.client.DropConnection()
					}
				}
			case <-h.stopCh:
				return
			}
		}
	}()
}

// Stop 停止心跳
func (h *Heartbeat) Stop() {
	h.stopOnce.Do(func() { close(h.stopCh) })
}

// Uploader 数据上报器
type Uploader struct {
	client      *WSClient
	interval    time.Duration        // 初始间隔（用于首次创建 ticker）
	intervalPtr *int64               // 动态间隔指针（原子操作），允许运行时热重载
	stopCh      chan struct{}
	stopOnce    sync.Once
}

// NewUploader 创建数据上报器
// intervalPtr 为可选的动态间隔指针（非 nil 时支持热重载），可为 nil
func NewUploader(client *WSClient, interval time.Duration, intervalPtr *int64) *Uploader {
	return &Uploader{
		client:      client,
		interval:    interval,
		intervalPtr: intervalPtr,
		stopCh:      make(chan struct{}),
	}
}

// Start 启动数据上报
// collectFn 是数据采集函数，返回采集到的监控数据
func (u *Uploader) Start(collectFn func() (*sharedmodel.MetricData, error)) {
	go func() {
		// 初始 ticker，优先使用 intervalPtr 的值
		currentInterval := int64(u.interval / time.Second)
		if u.intervalPtr != nil {
			if v := atomic.LoadInt64(u.intervalPtr); v > 0 {
				currentInterval = v
			}
		}
		ticker := time.NewTicker(time.Duration(currentInterval) * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				// 检查间隔是否变化，如变化则重建 ticker
				if u.intervalPtr != nil {
					newInterval := atomic.LoadInt64(u.intervalPtr)
					if newInterval < 1 {
						newInterval = currentInterval
					}
					if newInterval != currentInterval {
						ticker.Stop()
						currentInterval = newInterval
						ticker = time.NewTicker(time.Duration(currentInterval) * time.Second)
						log.Printf("数据上报间隔已更新为 %ds", currentInterval)
					}
				}

				if !u.client.IsConnected() {
					continue
				}

				data, err := collectFn()
				if err != nil {
					log.Printf("采集数据失败: %v", err)
					continue
				}

				if err := u.client.SendReport(data); err != nil {
					log.Printf("上报数据失败: %v", err)
					// 写失败说明连接已半死，主动断开触发重连（同心跳处理）
					u.client.DropConnection()
				}

			case <-u.stopCh:
				return
			}
		}
	}()
}

// Stop 停止上报
func (u *Uploader) Stop() {
	u.stopOnce.Do(func() { close(u.stopCh) })
}
