package service

import (
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/server-probe/server/internal/model"
	"github.com/server-probe/server/internal/pkg"
	"github.com/server-probe/server/internal/repository"
)

// SSLCertStatusResult SSL 证书监控状态结果（供 API 使用）
type SSLCertStatusResult struct {
	ID               int64     `json:"id"`
	Domain           string    `json:"domain"`
	Port             int       `json:"port"`
	AlertDays        int       `json:"alert_days"`
	LastExpiryDate   time.Time `json:"last_expiry_date"`
	LastRemainingDays int      `json:"last_remaining_days"`
	LastChecked      time.Time `json:"last_checked"`
	Enabled          bool      `json:"enabled"`
}

// SSLMonitorEngine SSL 证书监控引擎
type SSLMonitorEngine struct {
	repo     *repository.SSLCertMonitorRepository
	stopCh   chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

// NewSSLMonitorEngine 创建 SSL 证书监控引擎
func NewSSLMonitorEngine(repo *repository.SSLCertMonitorRepository) *SSLMonitorEngine {
	return &SSLMonitorEngine{
		repo:   repo,
		stopCh: make(chan struct{}),
	}
}

// Start 启动 SSL 证书监控引擎
func (e *SSLMonitorEngine) Start() {
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				e.checkAll()
			case <-e.stopCh:
				return
			}
		}
	}()

	log.Println("SSL 证书监控引擎已启动")
}

// Stop 停止 SSL 证书监控引擎
func (e *SSLMonitorEngine) Stop() {
	e.stopOnce.Do(func() {
		close(e.stopCh)
		e.wg.Wait()
	})
}

// checkAll 检查所有已启用的 SSL 证书监控（并发，信号量限制 10）
func (e *SSLMonitorEngine) checkAll() {
	monitors, err := e.repo.ListEnabled()
	if err != nil {
		log.Printf("获取 SSL 证书监控列表失败: %v", err)
		return
	}

	sem := make(chan struct{}, 10)
	var wg sync.WaitGroup

	for i := range monitors {
		wg.Add(1)
		sem <- struct{}{}
		go func(m model.SSLCertMonitor) {
			defer wg.Done()
			defer func() { <-sem }()
			e.checkCert(&m)
		}(monitors[i])
	}

	wg.Wait()
}

// checkCert 检查单个 SSL 证书
func (e *SSLMonitorEngine) checkCert(monitor *model.SSLCertMonitor) (remainingDays int, expiryDate time.Time, err error) {
	port := monitor.Port
	if port <= 0 {
		port = 443
	}

	// SSRF 防护：检查域名+端口是否指向内网
	if err := pkg.CheckHostPort(monitor.Domain, port); err != nil {
		log.Printf("SSL 证书检查被 SSRF 防护拦截 (ID=%d, domain=%s): %v", monitor.ID, monitor.Domain, err)
		e.markCheckFailed(monitor)
		return 0, time.Time{}, fmt.Errorf("SSRF 防护拦截")
	}

	address := fmt.Sprintf("%s:%d", monitor.Domain, port)

	conf := &tls.Config{
		InsecureSkipVerify: false,
		ServerName:         monitor.Domain,
	}

	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
	}

	conn, dialErr := tls.DialWithDialer(dialer, "tcp", address, conf)
	if dialErr != nil {
		log.Printf("SSL 证书检查连接失败 (ID=%d, domain=%s): %v", monitor.ID, monitor.Domain, dialErr)
		e.markCheckFailed(monitor)
		return 0, time.Time{}, dialErr
	}
	defer conn.Close()

	certs := conn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		log.Printf("SSL 证书链为空 (ID=%d, domain=%s)", monitor.ID, monitor.Domain)
		e.markCheckFailed(monitor)
		return 0, time.Time{}, fmt.Errorf("证书链为空")
	}

	cert := certs[0]
	expiryDate = cert.NotAfter
	now := time.Now()
	remainingDays = int(expiryDate.Sub(now).Hours() / 24)

	monitor.LastExpiryDate = expiryDate
	monitor.LastRemainingDays = remainingDays
	monitor.LastChecked = now

	if updateErr := e.repo.Update(monitor); updateErr != nil {
		log.Printf("更新 SSL 证书监控状态失败 (ID=%d): %v", monitor.ID, updateErr)
	}

	return remainingDays, expiryDate, nil
}

// markCheckFailed 标记检查失败：更新检查时间并重置过期数据，避免前端展示陈旧值
func (e *SSLMonitorEngine) markCheckFailed(monitor *model.SSLCertMonitor) {
	monitor.LastChecked = time.Now()
	monitor.LastRemainingDays = 0
	monitor.LastExpiryDate = time.Time{}
	if err := e.repo.Update(monitor); err != nil {
		log.Printf("更新 SSL 证书监控状态失败 (ID=%d): %v", monitor.ID, err)
	}
}

// CheckCert 公开方法，供 handler 调用进行即时检查
func (e *SSLMonitorEngine) CheckCert(monitor *model.SSLCertMonitor) (int, time.Time, error) {
	return e.checkCert(monitor)
}

// GetAllStatuses 获取所有 SSL 证书监控的当前状态（供 API 使用）
func (e *SSLMonitorEngine) GetAllStatuses() []SSLCertStatusResult {
	monitors, err := e.repo.List()
	if err != nil {
		log.Printf("获取 SSL 证书监控列表失败: %v", err)
		return []SSLCertStatusResult{}
	}

	results := make([]SSLCertStatusResult, 0, len(monitors))
	for _, m := range monitors {
		results = append(results, SSLCertStatusResult{
			ID:                m.ID,
			Domain:            m.Domain,
			Port:              m.Port,
			AlertDays:         m.AlertDays,
			LastExpiryDate:    m.LastExpiryDate,
			LastRemainingDays: m.LastRemainingDays,
			LastChecked:       m.LastChecked,
			Enabled:           m.Enabled,
		})
	}
	return results
}
