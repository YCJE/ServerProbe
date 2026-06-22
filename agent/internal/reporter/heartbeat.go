package reporter

import (
	"log"
	"sync"
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
	client   *WSClient
	interval time.Duration
	stopCh   chan struct{}
	stopOnce sync.Once
}

// NewUploader 创建数据上报器
func NewUploader(client *WSClient, interval time.Duration) *Uploader {
	return &Uploader{
		client:   client,
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

// Start 启动数据上报
// collectFn 是数据采集函数，返回采集到的监控数据
func (u *Uploader) Start(collectFn func() (*sharedmodel.MetricData, error)) {
	go func() {
		ticker := time.NewTicker(u.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
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
