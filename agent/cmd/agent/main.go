package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
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
	InsecureTLS        bool   `yaml:"insecure_tls"` // 跳过 TLS 证书验证 (自签名证书时使用)
}

func main() {
	configFile := flag.String("config", "/etc/probe-agent/config.yml", "配置文件路径")
	flag.Parse()

	// 加载配置
	cfg := loadConfig(*configFile)
	var cfgMu sync.Mutex

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
	wsClient := reporter.NewWSClient(cfg.ServerURL, cfg.Token, cfg.RegisterCode, cfg.InsecureTLS)

	// 设置回调
	var configSyncer *config.Syncer
	var pingTargets []sharedmodel.PingTarget
	var pingTargetsMu sync.Mutex
	var pingInterval int64 = 60 // 默认 60 秒，会被配置更新覆盖

	wsClient.SetCallbacks(
		// 注册成功回调
		func(token string) {
			log.Printf("注册成功，保存 Token")
			cfgMu.Lock()
			cfg.Token = token
			cfg.RegisterCode = "" // 清除注册码
			cfgMu.Unlock()
			saveConfig(*configFile, cfg, &cfgMu)

			// 启动配置拉取
			if configSyncer != nil {
				configSyncer.SetToken(token)
			}
		},
		// 配置更新回调
		func(config *sharedmodel.AgentConfig) {
			log.Printf("收到配置更新，探测目标 %d 个，间隔 %ds", len(config.PingTargets), config.PingInterval)
			pingTargetsMu.Lock()
			pingTargets = config.PingTargets
			pingTargetsMu.Unlock()
			if config.PingInterval > 0 {
				atomic.StoreInt64(&pingInterval, int64(config.PingInterval))
			}
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
	cfgMu.Lock()
	reportInterval := time.Duration(cfg.ReportInterval) * time.Second
	cfgMu.Unlock()
	uploader := reporter.NewUploader(wsClient, reportInterval)
	uploader.Start(func() (*sharedmodel.MetricData, error) {
		return collectAllData(cpuCollector, memCollector, diskCollector, netCollector, sysCollector)
	})

	// 启动 Ping 探测 (使用动态间隔，支持优雅停止)
	pingStopCh := make(chan struct{})
	go startPingProbe(wsClient, pingCollector, &pingTargets, &pingTargetsMu, &pingInterval, pingStopCh)

	// 启动配置拉取（无条件启动，sync() 内部会检查 Token 是否为空）
	cfgMu.Lock()
	configSyncer = config.NewSyncer(cfg.ServerURL, cfg.Token, time.Duration(cfg.ConfigSyncInterval)*time.Second, cfg.InsecureTLS)
	cfgMu.Unlock()
	configSyncer.Start()

	log.Printf("Agent 已启动，开始监控")

	// 等待退出信号，优雅关闭
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	log.Printf("收到信号 %v，正在退出...", sig)

	// 停止各组件
	close(pingStopCh) // 通知 Ping 探测协程停止
	heartbeat.Stop()
	uploader.Stop()
	configSyncer.Stop()
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
	cpuInfo, ok := cpuResult.(sharedmodel.CPUInfo)
	if !ok {
		return nil, fmt.Errorf("CPU 采集器返回类型错误: %T", cpuResult)
	}

	// 采集内存
	memResult, err := mem.Collect()
	if err != nil {
		return nil, err
	}
	memInfo, ok := memResult.(sharedmodel.MemoryInfo)
	if !ok {
		return nil, fmt.Errorf("Memory 采集器返回类型错误: %T", memResult)
	}

	// 采集磁盘
	diskResult, err := disk.Collect()
	if err != nil {
		return nil, err
	}
	diskInfo, ok := diskResult.([]sharedmodel.DiskInfo)
	if !ok {
		return nil, fmt.Errorf("Disk 采集器返回类型错误: %T", diskResult)
	}

	// 采集网络
	netResult, err := net.Collect()
	if err != nil {
		return nil, err
	}
	netInfo, ok := netResult.(sharedmodel.NetworkInfo)
	if !ok {
		return nil, fmt.Errorf("Network 采集器返回类型错误: %T", netResult)
	}

	// 采集系统信息
	sysResult, err := sys.Collect()
	if err != nil {
		return nil, err
	}
	sysInfo, ok := sysResult.(sharedmodel.SystemInfo)
	if !ok {
		return nil, fmt.Errorf("System 采集器返回类型错误: %T", sysResult)
	}

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

// saveConfig 保存 YAML 配置文件（原子操作：先写临时文件再 rename）
func saveConfig(path string, cfg *AgentConfig, mu *sync.Mutex) {
	mu.Lock()
	defer mu.Unlock()

	data, err := yaml.Marshal(cfg)
	if err != nil {
		log.Printf("序列化配置失败: %v", err)
		return
	}

	// 先写入临时文件，再原子替换，避免写入过程中崩溃导致配置文件损坏
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		log.Printf("保存配置文件失败: %v", err)
		return
	}
	if err := os.Rename(tmpPath, path); err != nil {
		log.Printf("替换配置文件失败: %v", err)
		os.Remove(tmpPath)
	}
}

// startPingProbe 启动 Ping 探测
func startPingProbe(client *reporter.WSClient, pinger *collector.PingCollector, targetsPtr *[]sharedmodel.PingTarget, mu *sync.Mutex, intervalPtr *int64, stopCh <-chan struct{}) {
	// 初始 ticker，使用当前间隔
	currentInterval := atomic.LoadInt64(intervalPtr)
	if currentInterval < 1 {
		currentInterval = 60
	}
	ticker := time.NewTicker(time.Duration(currentInterval) * time.Second)

	for {
		select {
		case <-stopCh:
			ticker.Stop()
			log.Printf("Ping 探测协程已停止")
			return
		case <-ticker.C:
		}

		// 检查间隔是否变化，如变化则重建 ticker
		newInterval := atomic.LoadInt64(intervalPtr)
		if newInterval < 1 {
			newInterval = 60
		}
		if newInterval != currentInterval {
			ticker.Stop()
			currentInterval = newInterval
			ticker = time.NewTicker(time.Duration(currentInterval) * time.Second)
			log.Printf("Ping 探测间隔已更新为 %ds", currentInterval)
		}

		if !client.IsConnected() {
			continue
		}

		// 加锁拷贝一份探测目标，避免长时间持锁
		mu.Lock()
		targets := make([]sharedmodel.PingTarget, len(*targetsPtr))
		copy(targets, *targetsPtr)
		mu.Unlock()

		if len(targets) == 0 {
			continue
		}

		results := pinger.PingTargets(targets)
		if len(results) > 0 {
			if err := client.SendPingResult(results); err != nil {
				log.Printf("上报 Ping 结果失败: %v", err)
			}
		}
	}
}
