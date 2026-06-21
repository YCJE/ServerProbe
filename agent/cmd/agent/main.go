package main

import (
	"flag"
	"log"
	"os"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/server-probe/agent/internal/collector"
	"github.com/server-probe/agent/internal/config"
	"github.com/server-probe/agent/internal/reporter"
	sharedmodel "github.com/server-probe/shared/model"
)

// AgentConfig Agent 配置
type AgentConfig struct {
	ServerURL          string `yaml:"server"`
	Token              string `yaml:"token"`
	RegisterCode       string `yaml:"register_code"`
	ReportInterval     int    `yaml:"report_interval"`
	ConfigSyncInterval int    `yaml:"config_sync_interval"`
	PingMethod         string `yaml:"ping_method"`
}

func main() {
	configFile := flag.String("config", "/etc/probe-agent/config.yml", "配置文件路径")
	flag.Parse()

	// 加载配置
	cfg := loadConfig(*configFile)

	log.Printf("Server 探针 Agent 启动")
	log.Printf("Server: %s", cfg.ServerURL)
	log.Printf("上报间隔: %ds", cfg.ReportInterval)

	// 创建采集器
	fileReader := &collector.OSFileReader{}
	cpuCollector := collector.NewCPUCollector(fileReader)
	memCollector := collector.NewMemoryCollector(fileReader)
	diskCollector := collector.NewDiskCollector(&collector.OSDiskMounter{})
	netCollector := collector.NewNetworkCollector(fileReader)
	sysCollector := collector.NewSystemCollector(fileReader, "v1.0.0")
	pingCollector := collector.NewPingCollector(cfg.PingMethod)

	// 创建 WebSocket 客户端
	wsClient := reporter.NewWSClient(cfg.ServerURL, cfg.Token, cfg.RegisterCode)

	// 设置回调
	var configSyncer *config.Syncer
	var pingTargets []sharedmodel.PingTarget

	wsClient.SetCallbacks(
		// 注册成功回调
		func(token string) {
			log.Printf("注册成功，保存 Token")
			cfg.Token = token
			cfg.RegisterCode = "" // 清除注册码
			saveConfig(*configFile, cfg)

			// 启动配置拉取
			if configSyncer != nil {
				configSyncer.SetToken(token)
			}
		},
		// 配置更新回调
		func(config *sharedmodel.AgentConfig) {
			log.Printf("收到配置更新，探测目标 %d 个", len(config.PingTargets))
			pingTargets = config.PingTargets
		},
		nil,
	)

	// 连接 Server
	if err := wsClient.Connect(); err != nil {
		log.Printf("连接 Server 失败: %v", err)
		log.Printf("将在后台重试连接...")
	}

	// 启动 WebSocket 消息循环
	go wsClient.Run()

	// 启动心跳
	heartbeat := reporter.NewHeartbeat(wsClient, 30*time.Second)
	heartbeat.Start()

	// 启动数据上报
	uploader := reporter.NewUploader(wsClient, time.Duration(cfg.ReportInterval)*time.Second)
	uploader.Start(func() (*sharedmodel.MetricData, error) {
		return collectAllData(cpuCollector, memCollector, diskCollector, netCollector, sysCollector)
	})

	// 启动 Ping 探测
	go startPingProbe(wsClient, pingCollector, &pingTargets, 60*time.Second)

	// 启动配置拉取
	configSyncer = config.NewSyncer(cfg.ServerURL, cfg.Token, time.Duration(cfg.ConfigSyncInterval)*time.Second)
	if cfg.Token != "" {
		configSyncer.Start()
	}

	log.Printf("Agent 已启动，开始监控")

	// 阻塞主 goroutine
	select {}
}

// collectAllData 采集所有监控数据
func collectAllData(
	cpu *collector.CPUCollector,
	mem *collector.MemoryCollector,
	disk *collector.DiskCollector,
	net *collector.NetworkCollector,
	sys *collector.SystemCollector,
) (*sharedmodel.MetricData, error) {
	// 采集 CPU
	cpuResult, err := cpu.Collect()
	if err != nil {
		return nil, err
	}
	cpuInfo := cpuResult.(sharedmodel.CPUInfo)

	// 采集内存
	memResult, err := mem.Collect()
	if err != nil {
		return nil, err
	}
	memInfo := memResult.(sharedmodel.MemoryInfo)

	// 采集磁盘
	diskResult, err := disk.Collect()
	if err != nil {
		return nil, err
	}
	diskInfo := diskResult.([]sharedmodel.DiskInfo)

	// 采集网络
	netResult, err := net.Collect()
	if err != nil {
		return nil, err
	}
	netInfo := netResult.(sharedmodel.NetworkInfo)

	// 采集系统信息
	sysResult, err := sys.Collect()
	if err != nil {
		return nil, err
	}
	sysInfo := sysResult.(sharedmodel.SystemInfo)

	// 采集运行时间
	uptime, _ := sys.CollectUptime()

	// 采集进程数
	processCount, _ := sys.CollectProcessCount()

	return &sharedmodel.MetricData{
		CPU:          cpuInfo,
		Memory:       memInfo,
		Disks:        diskInfo,
		Network:      netInfo,
		Uptime:       uptime,
		ProcessCount: processCount,
		System:       sysInfo,
	}, nil
}

// loadConfig 加载 YAML 配置文件
func loadConfig(path string) *AgentConfig {
	cfg := &AgentConfig{
		ReportInterval:     3,
		ConfigSyncInterval: 3600,
		PingMethod:         "auto",
	}

	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("读取配置文件失败: %v", err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		log.Fatalf("解析配置文件失败: %v", err)
	}

	// 设置默认值
	if cfg.ReportInterval <= 0 {
		cfg.ReportInterval = 3
	}
	if cfg.ConfigSyncInterval <= 0 {
		cfg.ConfigSyncInterval = 3600
	}
	if cfg.PingMethod == "" {
		cfg.PingMethod = "auto"
	}

	return cfg
}

// saveConfig 保存 YAML 配置文件
func saveConfig(path string, cfg *AgentConfig) {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		log.Printf("序列化配置失败: %v", err)
		return
	}

	// 保持文件权限 600
	if err := os.WriteFile(path, data, 0600); err != nil {
		log.Printf("保存配置文件失败: %v", err)
	}
}

// startPingProbe 启动 Ping 探测
func startPingProbe(client *reporter.WSClient, pinger *collector.PingCollector, targets *[]sharedmodel.PingTarget, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if !client.IsConnected() || len(*targets) == 0 {
				continue
			}

			results := pinger.PingTargets(*targets)
			if len(results) > 0 {
				if err := client.SendPingResult(results); err != nil {
					log.Printf("上报 Ping 结果失败: %v", err)
				}
			}
		}
	}
}
