package config

import (
	"crypto/tls"
	"encoding/json"
	"log"
	"io"
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

	currentConfig *sharedmodel.AgentConfig
	mu             sync.RWMutex
	stopCh         chan struct{}
}

// NewSyncer 创建配置拉取器
func NewSyncer(serverURL, token string, interval time.Duration, insecureTLS bool) *Syncer {
	return &Syncer{
		serverURL:   serverURL,
		token:       token,
		interval:    interval,
		insecureTLS: insecureTLS,
		stopCh:      make(chan struct{}),
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
	close(s.stopCh)
}

// GetConfig 获取当前配置
func (s *Syncer) GetConfig() *sharedmodel.AgentConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentConfig
}

// sync 拉取配置
func (s *Syncer) sync() {
	// 强制 TLS
	if !strings.HasPrefix(s.serverURL, "https://") {
		log.Printf("安全错误：Server 地址必须使用 https://")
		return
	}

	url := s.serverURL + "/api/v1/agent/config?token=" + s.token

	// 创建 HTTP 客户端
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}
	if s.insecureTLS {
		tlsConfig.InsecureSkipVerify = true
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}

	resp, err := client.Get(url)
	if err != nil {
		log.Printf("拉取配置失败: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("拉取配置失败: HTTP %d", resp.StatusCode)
		return
	}

	body, err := io.ReadAll(resp.Body)
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

	log.Printf("配置拉取成功，探测目标 %d 个", len(config.PingTargets))
}

// SetToken 设置 Token（注册成功后调用）
func (s *Syncer) SetToken(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.token = token
}
