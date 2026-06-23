package main

import (
	"context"
	"crypto/tls"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/server-probe/server/internal/api"
	"github.com/server-probe/server/internal/pkg"
	"github.com/server-probe/server/internal/repository"
	"github.com/server-probe/server/internal/service"
	web "github.com/server-probe/server"
)

// ServerConfig Server 配置文件结构
type ServerConfig struct {
	Listen   string `yaml:"listen"`
	DataDir  string `yaml:"data_dir"`
	JWTSecret string `yaml:"jwt_secret"`
	TLS      struct {
		Auto     bool   `yaml:"auto"`
		CertFile string `yaml:"cert_file"`
		KeyFile  string `yaml:"key_file"`
	} `yaml:"tls"`
	Aggregation struct {
		Interval      int `yaml:"interval"`
		RetentionDays int `yaml:"retention_days"`
	} `yaml:"aggregation"`
	RingBuffer struct {
		Size int `yaml:"size"`
	} `yaml:"ring_buffer"`
}

func main() {
	// 解析命令行参数
	configFile := flag.String("config", "", "配置文件路径")
	dataDir := flag.String("data-dir", "./data", "数据目录")
	certDir := flag.String("cert-dir", "./certs", "证书目录")
	listen := flag.String("listen", ":443", "监听地址")
	flag.Parse()

	// 加载配置文件（如果指定）
	var cfg ServerConfig
	if *configFile != "" {
		cfg = loadConfig(*configFile)
		// 配置文件覆盖命令行参数
		if cfg.Listen != "" {
			*listen = cfg.Listen
		}
		if cfg.DataDir != "" {
			*dataDir = cfg.DataDir
		}
		if cfg.TLS.CertFile != "" {
			*certDir = filepath.Dir(cfg.TLS.CertFile)
		}
		log.Printf("已加载配置文件: %s", *configFile)
	}

	// 初始化 SQLite 数据库
	db, err := repository.NewSQLiteDB(*dataDir)
	if err != nil {
		log.Fatalf("数据库初始化失败: %v", err)
	}

	// 创建 repositories
	agentRepo := repository.NewAgentRepository(db.DB())
	registerCodeRepo := repository.NewRegisterCodeRepository(db.DB())
	adminRepo := repository.NewAdminRepository(db.DB())
	recordRepo := repository.NewRecordRepository(db.DB())
	pingTargetRepo := repository.NewPingTargetRepository(db.DB())
	alertRepo := repository.NewAlertRepository(db.DB())
	notifyRepo := repository.NewNotifyRepository(db.DB())

	// 生成或加载 JWT 密钥
	jwtSecretFile := filepath.Join(*dataDir, "jwt_secret")
	jwtSecret, err := loadOrCreateSecret(jwtSecretFile)
	if err != nil {
		log.Fatalf("JWT 密钥初始化失败: %v", err)
	}
	// 配置文件中的 JWT 密钥优先
	if cfg.JWTSecret != "" {
		jwtSecret = cfg.JWTSecret
	}

	jwtManager := pkg.NewJWTManager(jwtSecret, 12*time.Hour)

	// 创建 SSRF 防护器
	ssrfProtector := pkg.NewSSRFProtector()

	// 创建 services
	monitor := service.NewMonitorService(agentRepo, *dataDir)
	registry := service.NewAgentRegistryService(agentRepo, registerCodeRepo, db.DB())
	configSync := service.NewConfigSyncService(pingTargetRepo, db.DB())
	validator := service.NewDataValidator()
	aggregation := service.NewAggregationService(monitor, recordRepo, agentRepo)
	notifySvc := service.NewNotifyService(notifyRepo, ssrfProtector)
	alertEngine := service.NewAlertEngine(alertRepo, monitor, notifySvc)

	// 启动心跳检查
	monitor.StartHeartbeatChecker(90 * time.Second)

	// 启动数据聚合服务
	aggregation.Start()
	retentionDays := 2 // 默认保留 2 天
	if cfg.Aggregation.RetentionDays > 0 {
		retentionDays = cfg.Aggregation.RetentionDays
	}
	aggregation.StartCleanupTask(retentionDays)

	// 启动告警引擎
	alertEngine.Start()

	// 启动数据校验器清理任务
	validator.StartCleanupTask()

	// 创建路由
	router := api.NewRouter(
		jwtManager,
		adminRepo,
		agentRepo,
		recordRepo,
		monitor,
		registry,
		configSync,
		validator,
		pingTargetRepo,
		alertRepo,
		notifyRepo,
		alertEngine,
		notifySvc,
	)

	// 注册前端静态文件处理器
	router.GetRouter().NoRoute(web.StaticFileHandler())

	// 确保 TLS 证书
	var certPath, keyPath string
	if cfg.TLS.CertFile != "" && cfg.TLS.KeyFile != "" {
		certPath = cfg.TLS.CertFile
		keyPath = cfg.TLS.KeyFile
		log.Printf("使用指定 TLS 证书: %s", certPath)
	} else {
		// 自签证书存放到数据目录下
		autoCertDir := filepath.Join(*dataDir, "certs")
		certPath, keyPath, err = pkg.EnsureTLS(autoCertDir)
		if err != nil {
			log.Fatalf("TLS 证书初始化失败: %v", err)
		}
	}

	log.Printf("Server 探针服务启动，监听 %s", *listen)
	log.Printf("WebSocket 端点: wss://<host>%s/api/v1/agent/report", *listen)
	log.Printf("数据目录: %s", *dataDir)

	// 创建 HTTP Server
	httpServer := &http.Server{
		Addr:    *listen,
		Handler: router.GetRouter(),
		TLSConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
	}

	// 使用 signal.NotifyContext 监听中断信号，实现优雅关闭
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// 在 goroutine 中启动 HTTPS 服务
	serverErr := make(chan error, 1)
	go func() {
		if err := httpServer.ListenAndServeTLS(certPath, keyPath); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	// 等待中断信号或服务器错误
	select {
	case err := <-serverErr:
		log.Fatalf("服务启动失败: %v", err)
	case <-ctx.Done():
		log.Println("收到关闭信号，正在优雅关闭服务...")
		stop() // 停止接收信号，恢复默认行为
	}

	// 给正在处理的请求 30 秒的时间完成
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP 服务关闭失败: %v", err)
	}

	// 停止各后台服务
	monitor.Stop()
	aggregation.Stop()
	alertEngine.Stop()
	validator.Stop()

	log.Println("正在关闭数据库连接...")
	if err := db.Close(); err != nil {
		log.Printf("数据库关闭失败: %v", err)
	}

	log.Println("服务已关闭")
}

// loadConfig 加载 YAML 配置文件
func loadConfig(path string) ServerConfig {
	var cfg ServerConfig

	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("读取配置文件失败: %v", err)
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		log.Fatalf("解析配置文件失败: %v", err)
	}

	return cfg
}

// loadOrCreateSecret 加载或创建 JWT 密钥
func loadOrCreateSecret(path string) (string, error) {
	// 尝试读取
	if data, err := os.ReadFile(path); err == nil {
		return string(data), nil
	}

	// 生成新密钥
	secret, err := pkg.GenerateSecret(32)
	if err != nil {
		return "", err
	}

	// 确保目录存在
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", err
	}

	// 写入文件（权限 600）
	if err := os.WriteFile(path, []byte(secret), 0600); err != nil {
		return "", err
	}

	log.Println("已生成新的 JWT 密钥")
	return secret, nil
}
