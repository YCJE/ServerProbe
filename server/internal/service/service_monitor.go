package service

import (
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/server-probe/server/internal/model"
	"github.com/server-probe/server/internal/repository"
)

// ServiceStatusResult 服务监控状态结果（供 API 使用）
type ServiceStatusResult struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Type        string    `json:"type"`
	Target      string    `json:"target"`
	LastStatus  string    `json:"last_status"`
	LastLatency float64   `json:"last_latency"`
	LastChecked time.Time `json:"last_checked"`
	Enabled     bool      `json:"enabled"`
}

// ServiceMonitorEngine 服务监控引擎
type ServiceMonitorEngine struct {
	repo     *repository.ServiceMonitorRepository
	stopCh   chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

// NewServiceMonitorEngine 创建服务监控引擎
func NewServiceMonitorEngine(repo *repository.ServiceMonitorRepository) *ServiceMonitorEngine {
	return &ServiceMonitorEngine{
		repo:   repo,
		stopCh: make(chan struct{}),
	}
}

// Start 启动服务监控引擎
func (e *ServiceMonitorEngine) Start() {
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				e.probeAll()
			case <-e.stopCh:
				return
			}
		}
	}()

	log.Println("服务监控引擎已启动")
}

// Stop 停止服务监控引擎
func (e *ServiceMonitorEngine) Stop() {
	e.stopOnce.Do(func() {
		close(e.stopCh)
		e.wg.Wait()
	})
}

// probeAll 探测所有已启用的服务监控（并发，信号量限制 10）
func (e *ServiceMonitorEngine) probeAll() {
	monitors, err := e.repo.ListEnabled()
	if err != nil {
		log.Printf("获取服务监控列表失败: %v", err)
		return
	}

	sem := make(chan struct{}, 10)
	var wg sync.WaitGroup

	for i := range monitors {
		wg.Add(1)
		sem <- struct{}{}
		go func(m model.ServiceMonitor) {
			defer wg.Done()
			defer func() { <-sem }()
			e.probeService(&m)
		}(monitors[i])
	}

	wg.Wait()
}

// probeService 探测单个服务
func (e *ServiceMonitorEngine) probeService(monitor *model.ServiceMonitor) (status string, latency float64) {
	switch monitor.Type {
	case "http":
		status, latency = e.probeHTTP(monitor)
	case "tcp":
		status, latency = e.probeTCP(monitor)
	default:
		status = "down"
		latency = 0
		log.Printf("未知的服务监控类型: %s (ID=%d)", monitor.Type, monitor.ID)
	}

	monitor.LastStatus = status
	monitor.LastLatency = latency
	monitor.LastChecked = time.Now()

	if err := e.repo.Update(monitor); err != nil {
		log.Printf("更新服务监控状态失败 (ID=%d): %v", monitor.ID, err)
	}

	return status, latency
}

// probeHTTP 探测 HTTP 服务
func (e *ServiceMonitorEngine) probeHTTP(monitor *model.ServiceMonitor) (status string, latency float64) {
	timeout := time.Duration(monitor.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	client := &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	start := time.Now()
	resp, err := client.Get(monitor.Target)
	latency = float64(time.Since(start).Milliseconds())

	if err != nil {
		log.Printf("HTTP 探测失败 (ID=%d, target=%s): %v", monitor.ID, monitor.Target, err)
		return "down", latency
	}
	defer resp.Body.Close()

	if resp.StatusCode == monitor.ExpectedStatus {
		return "up", latency
	}

	log.Printf("HTTP 状态码不匹配 (ID=%d): expected=%d, got=%d", monitor.ID, monitor.ExpectedStatus, resp.StatusCode)
	return "down", latency
}

// probeTCP 探测 TCP 服务
func (e *ServiceMonitorEngine) probeTCP(monitor *model.ServiceMonitor) (status string, latency float64) {
	timeout := time.Duration(monitor.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	start := time.Now()
	conn, err := net.DialTimeout("tcp", monitor.Target, timeout)
	latency = float64(time.Since(start).Milliseconds())

	if err != nil {
		log.Printf("TCP 探测失败 (ID=%d, target=%s): %v", monitor.ID, monitor.Target, err)
		return "down", latency
	}
	defer conn.Close()

	return "up", latency
}

// ProbeService 公开方法，供 handler 调用进行即时探测
func (e *ServiceMonitorEngine) ProbeService(monitor *model.ServiceMonitor) (string, float64) {
	return e.probeService(monitor)
}

// GetAllStatuses 获取所有服务监控的当前状态（供 API 使用）
func (e *ServiceMonitorEngine) GetAllStatuses() []ServiceStatusResult {
	monitors, err := e.repo.List()
	if err != nil {
		log.Printf("获取服务监控列表失败: %v", err)
		return []ServiceStatusResult{}
	}

	results := make([]ServiceStatusResult, 0, len(monitors))
	for _, m := range monitors {
		results = append(results, ServiceStatusResult{
			ID:          m.ID,
			Name:        m.Name,
			Type:        m.Type,
			Target:      m.Target,
			LastStatus:  m.LastStatus,
			LastLatency: m.LastLatency,
			LastChecked: m.LastChecked,
			Enabled:     m.Enabled,
		})
	}
	return results
}
