package config

import (
	"crypto/tls"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	sharedmodel "github.com/server-probe/shared/model"
)

// Syncer 配置拉取器
type Syncer struct {
	serverURL   string
	token       string
	interval    time.Duration
	insecureTLS bool

	httpClient *http.Client

	currentConfig *sharedmodel.AgentConfig
	mu             sync.RWMutex
	stopCh         chan struct{}
	stopOnce       sync.Once
}

// NewSyncer 创建配置拉取器
func NewSyncer(serverURL, token string, interval time.Duration, insecureTLS bool) *Syncer {
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}
	if insecureTLS {
		tlsConfig.InsecureSkipVerify = true
	}

	return &Syncer{
		serverURL:   strings.TrimRight(serverURL, "/"),
		token:       token,
		interval:    interval,
		insecureTLS: insecureTLS,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: tlsConfig,
			},
		},
		stopCh: make(chan struct{}),
	}
}

// Start 启动配置拉取
func (s *Syncer) Start() {
	// 首次立即拉取
	s.sync()

	go func() {
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				s.sync()
			case <-s.stopCh:
				return
			}
		}
	}()
}

// Stop 停止拉取
func (s *Syncer) Stop() {
	s.stopOnce.Do(func() { close(s.stopCh) })
}

// GetConfig 获取当前配置
func (s *Syncer) GetConfig() *sharedmodel.AgentConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentConfig
}

// sync 拉取配置
func (s *Syncer) sync() {
	// 强制 TLS（大小写不敏感）
	lowerURL := strings.ToLower(s.serverURL)
	if !strings.HasPrefix(lowerURL, "https://") {
		log.Printf("安全错误：Server 地址必须使用 https://")
		return
	}

	// 加读锁获取 token
	s.mu.RLock()
	token := s.token
	s.mu.RUnlock()

	if token == "" {
		log.Printf("拉取配置失败: 缺少 Token")
		return
	}

	url := s.serverURL + "/api/v1/agent/config"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Printf("创建请求失败: %v", err)
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		log.Printf("拉取配置失败: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("拉取配置失败: HTTP %d", resp.StatusCode)
		return
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024)) // 限制 1MB
	if err != nil {
		log.Printf("读取配置响应失败: %v", err)
		return
	}

	var config sharedmodel.AgentConfig
	if err := json.Unmarshal(body, &config); err != nil {
		log.Printf("解析配置失败: %v", err)
		return
	}

	s.mu.Lock()
	s.currentConfig = &config
	s.mu.Unlock()

	log.Printf("配置拉取成功，探测目标 %d 个，上报间隔 %ds", len(config.PingTargets), config.ReportInterval)
}

// SetToken 设置 Token（注册成功后调用）
func (s *Syncer) SetToken(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.token = token
}
