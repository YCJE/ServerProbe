package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/server-probe/server/internal/api"
	"github.com/server-probe/server/internal/pkg"
	"github.com/server-probe/server/internal/repository"
	"github.com/server-probe/server/internal/service"
	web "github.com/server-probe/server"
)

func main() {
	// 解析命令行参数
	dataDir := flag.String("data-dir", "./data", "数据目录")
	certDir := flag.String("cert-dir", "./certs", "证书目录")
	listen := flag.String("listen", ":443", "监听地址")
	flag.Parse()

	// 初始化 SQLite 数据库
	db, err := repository.NewSQLiteDB(*dataDir)
	if err != nil {
		log.Fatalf("数据库初始化失败: %v", err)
	}
	defer db.Close()

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

	jwtManager := pkg.NewJWTManager(jwtSecret, 12*time.Hour)

	// 创建 SSRF 防护器
	ssrfProtector := pkg.NewSSRFProtector()

	// 创建 services
	monitor := service.NewMonitorService(agentRepo)
	registry := service.NewAgentRegistryService(agentRepo, registerCodeRepo)
	configSync := service.NewConfigSyncService(pingTargetRepo)
	validator := service.NewDataValidator()
	aggregation := service.NewAggregationService(monitor, recordRepo, agentRepo)
	notifySvc := service.NewNotifyService(notifyRepo, ssrfProtector)
	alertEngine := service.NewAlertEngine(alertRepo, monitor, notifySvc)

	// 启动心跳检查
	monitor.StartHeartbeatChecker(90 * time.Second)

	// 启动数据聚合服务
	aggregation.Start()
	aggregation.StartCleanupTask(90)

	// 启动告警引擎
	alertEngine.Start()

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
	)

	// 注册前端静态文件处理器
	router.GetRouter().NoRoute(web.StaticFileHandler())

	// 确保 TLS 证书
	certPath, keyPath, err := pkg.EnsureTLS(*certDir)
	if err != nil {
		log.Fatalf("TLS 证书初始化失败: %v", err)
	}

	log.Printf("Server 探针服务启动，监听 %s", *listen)
	log.Printf("WebSocket 端点: wss://<host>%s/api/v1/agent/report", *listen)
	log.Printf("数据目录: %s", *dataDir)

	// 启动 HTTPS 服务
	if err := router.GetRouter().RunTLS(*listen, certPath, keyPath); err != nil {
		log.Fatalf("服务启动失败: %v", err)
	}
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
