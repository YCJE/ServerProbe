package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
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
	ServerURL           string `yaml:"server"`
	Token               string `yaml:"token"`
	RegisterCode        string `yaml:"register_code"`
	ReportInterval      int    `yaml:"report_interval"`
	ConfigSyncInterval  int    `yaml:"config_sync_interval"`
	PingMethod          string `yaml:"ping_method"`
	InsecureTLS         bool   `yaml:"insecure_tls"`           // 跳过 TLS 证书验证 (自签名证书时使用)
	AllowPrivateTargets bool   `yaml:"allow_private_targets"` // 允许 Ping 私有网段地址 (默认禁止，防 SSRF)
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
	pingCollector := collector.NewPingCollector(cfg.PingMethod, cfg.InsecureTLS, cfg.AllowPrivateTargets)
	processCollector := collector.NewProcessCollector(fileReader)
	ntpCollector := collector.NewNTPCollector("") // 使用默认 NTP 服务器

	// 创建 WebSocket 客户端
	wsClient := reporter.NewWSClient(cfg.ServerURL, cfg.Token, cfg.RegisterCode, cfg.InsecureTLS)

	// 设置回调
	var configSyncer *config.Syncer
	var pingTargets []sharedmodel.PingTarget
	var pingTargetsMu sync.Mutex
	var pingInterval int64 = 60    // 默认 60 秒，会被配置更新覆盖
	var reportIntervalVal int64    // 上报间隔（秒），原子操作，支持热重载

	cfgMu.Lock()
	reportIntervalVal = int64(cfg.ReportInterval)
	cfgMu.Unlock()

	// lastAppliedCfg 记录最近一次已应用的配置快照，供兜底协程与 WS 回调比较配置是否变化
	var lastAppliedCfg *sharedmodel.AgentConfig
	var lastAppliedCfgMu sync.Mutex

	// configEqual 比较两份配置的 ping targets / ping interval / report interval 是否一致
	configEqual := func(a, b *sharedmodel.AgentConfig) bool {
		if a == nil || b == nil {
			return a == b
		}
		if a.PingInterval != b.PingInterval || a.ReportInterval != b.ReportInterval {
			return false
		}
		if len(a.PingTargets) != len(b.PingTargets) {
			return false
		}
		for i := range a.PingTargets {
			if a.PingTargets[i] != b.PingTargets[i] {
				return false
			}
		}
		return true
	}

	// applyConfig 应用配置更新到本地运行状态（ping 目标、探测间隔、上报间隔）
	// 同时记录最近一次应用的配置快照，供兜底协程比较
	applyConfig := func(config *sharedmodel.AgentConfig) {
		if config == nil {
			return
		}
		log.Printf("应用配置更新，探测目标 %d 个，间隔 %ds，上报间隔 %ds",
			len(config.PingTargets), config.PingInterval, config.ReportInterval)
		pingTargetsMu.Lock()
		pingTargets = config.PingTargets
		pingTargetsMu.Unlock()
		// 边界钳制：防止异常配置（如超大值撑爆 ticker、0 值引发除零/空转）
		if config.PingInterval > 0 {
			v := int64(config.PingInterval)
			if v > 600 {
				v = 600 // 上限 10 分钟
			}
			atomic.StoreInt64(&pingInterval, v)
		}
		// 支持 Server 下发新的上报间隔
		if config.ReportInterval > 0 {
			v := int64(config.ReportInterval)
			if v > 60 {
				v = 60 // 上限 60s
			}
			atomic.StoreInt64(&reportIntervalVal, v)
		}
		lastAppliedCfgMu.Lock()
		lastAppliedCfg = config
		lastAppliedCfgMu.Unlock()
	}

	wsClient.SetCallbacks(
		// 注册成功回调
		func(token string) {
			log.Printf("注册成功，保存 Token")
			cfgMu.Lock()
			cfg.Token = token
			cfg.RegisterCode = "" // 清除注册码
			cfgMu.Unlock()
			// Token 持久化失败时重试；若最终仍失败，Agent 重启后将丢失认证凭据
			var saveErr error
			for attempt := 1; attempt <= 3; attempt++ {
				if saveErr = saveConfig(*configFile, cfg, &cfgMu); saveErr == nil {
					break
				}
				log.Printf("保存 Token 失败 (第 %d 次): %v", attempt, saveErr)
				time.Sleep(time.Duration(attempt) * time.Second)
			}
			if saveErr != nil {
				log.Printf("严重错误: Token 持久化失败，Agent 重启后将无法自动恢复会话，请检查配置目录权限: %v", saveErr)
			}

			// 启动配置拉取
			if configSyncer != nil {
				configSyncer.SetToken(token)
			}
		},
		// 配置更新回调
		func(config *sharedmodel.AgentConfig) {
			applyConfig(config)
		},
		nil,
	)

	// 创建配置同步器（必须在 wsClient.Connect()/Run() 之前，避免注册回调中读到 nil）
	cfgMu.Lock()
	configSyncer = config.NewSyncer(cfg.ServerURL, cfg.Token, time.Duration(cfg.ConfigSyncInterval)*time.Second, cfg.InsecureTLS)
	cfgMu.Unlock()

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
	var collectCounter int64 // 采集计数器，用于控制静态数据采集频率
	uploader := reporter.NewUploader(wsClient, reportInterval, &reportIntervalVal)
	uploader.Start(func() (*sharedmodel.MetricData, error) {
		collectCounter++
		return collectAllData(cpuCollector, memCollector, diskCollector, netCollector, sysCollector, processCollector, ntpCollector, collectCounter)
	})

	// 启动 Ping 探测 (使用动态间隔，支持优雅停止)
	pingStopCh := make(chan struct{})
	go startPingProbe(wsClient, pingCollector, &pingTargets, &pingTargetsMu, &pingInterval, pingStopCh)

	// 启动配置拉取（无条件启动，sync() 内部会检查 Token 是否为空）
	configSyncer.Start()

	// 启动配置应用兜底协程：定期调用 GetConfig()，若拉取到的配置与当前已应用配置不同则应用
	// 作为 WebSocket 推送丢失时的兜底机制，确保 Agent 最终能应用 Server 端最新配置
	configApplyStopCh := make(chan struct{})
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				fetched := configSyncer.GetConfig()
				if fetched == nil {
					continue
				}
				lastAppliedCfgMu.Lock()
				same := configEqual(lastAppliedCfg, fetched)
				lastAppliedCfgMu.Unlock()
				if same {
					continue
				}
				log.Printf("兜底协程检测到配置变更，开始应用")
				applyConfig(fetched)
			case <-configApplyStopCh:
				log.Printf("配置应用兜底协程已停止")
				return
			}
		}
	}()

	log.Printf("Agent 已启动，开始监控")

	// 等待退出信号，优雅关闭
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	log.Printf("收到信号 %v，正在退出...", sig)

	// 停止各组件
	close(pingStopCh)        // 通知 Ping 探测协程停止
	close(configApplyStopCh) // 通知配置应用兜底协程停止
	wsClient.Stop()          // 停止 WebSocket 客户端
	heartbeat.Stop()
	uploader.Stop()
	configSyncer.Stop()
	ntpCollector.Stop() // 停止 NTP 重试 goroutine，防止泄漏
}

// collectAllData 采集所有监控数据
// 各采集器独立采集：单个采集器失败不影响其他指标，只要有一项成功就上报数据
// counter 为采集计数器，每 100 次才采集一次静态数据（SystemInfo），降低开销
func collectAllData(
	cpu *collector.CPUCollector,
	mem *collector.MemoryCollector,
	disk *collector.DiskCollector,
	net *collector.NetworkCollector,
	sys *collector.SystemCollector,
	proc *collector.ProcessCollector,
	ntp *collector.NTPCollector,
	counter int64,
) (*sharedmodel.MetricData, error) {
	var (
		data    sharedmodel.MetricData
		errs    []string
		success int
	)

	// 采集 CPU
	if cpuResult, err := cpu.Collect(); err != nil {
		errs = append(errs, fmt.Sprintf("CPU: %v", err))
	} else if cpuInfo, ok := cpuResult.(sharedmodel.CPUInfo); !ok {
		errs = append(errs, fmt.Sprintf("CPU 采集器返回类型错误: %T", cpuResult))
	} else {
		data.CPU = cpuInfo
		success++
	}

	// 采集内存
	if memResult, err := mem.Collect(); err != nil {
		errs = append(errs, fmt.Sprintf("Memory: %v", err))
	} else if memInfo, ok := memResult.(sharedmodel.MemoryInfo); !ok {
		errs = append(errs, fmt.Sprintf("Memory 采集器返回类型错误: %T", memResult))
	} else {
		data.Memory = memInfo
		success++
	}

	// 采集磁盘
	if diskResult, err := disk.Collect(); err != nil {
		errs = append(errs, fmt.Sprintf("Disk: %v", err))
	} else if diskInfo, ok := diskResult.([]sharedmodel.DiskInfo); !ok {
		errs = append(errs, fmt.Sprintf("Disk 采集器返回类型错误: %T", diskResult))
	} else {
		data.Disks = diskInfo
		success++
	}

	// 采集网络
	if netResult, err := net.Collect(); err != nil {
		errs = append(errs, fmt.Sprintf("Network: %v", err))
	} else if netInfo, ok := netResult.(sharedmodel.NetworkInfo); !ok {
		errs = append(errs, fmt.Sprintf("Network 采集器返回类型错误: %T", netResult))
	} else {
		data.Network = netInfo
		success++
	}

	// 采集系统信息（静态数据）- 每 100 次动态采集才采集一次静态数据
	// counter 从 1 开始（collectCounter++ 后首次调用），counter%100 == 1 使首次即采集静态数据
	// 非静态采集周期中，SystemInfo 字段为零值，Server 端通过 SystemInfo.OS == "" 判断
	if counter%100 == 1 {
		if sysResult, err := sys.Collect(); err != nil {
			errs = append(errs, fmt.Sprintf("System: %v", err))
		} else if sysInfo, ok := sysResult.(sharedmodel.SystemInfo); !ok {
			errs = append(errs, fmt.Sprintf("System 采集器返回类型错误: %T", sysResult))
		} else {
			data.System = sysInfo
			success++
		}
	}

	// 采集运行时间（失败不影响整体上报）
	if uptime, err := sys.CollectUptime(); err == nil {
		data.Uptime = uptime
	}

	// 采集进程数（失败不影响整体上报）
	if processCount, err := sys.CollectProcessCount(); err == nil {
		data.ProcessCount = processCount
	}

	// 采集进程列表 Top 10（失败不影响整体上报）
	if procResult, err := proc.Collect(); err != nil {
		errs = append(errs, fmt.Sprintf("Process: %v", err))
	} else if processes, ok := procResult.([]sharedmodel.ProcessInfo); !ok {
		errs = append(errs, fmt.Sprintf("Process 采集器返回类型错误: %T", procResult))
	} else {
		data.Processes = processes
		success++
	}

	// 采集 NTP 时间偏移（失败不影响整体上报，返回 0）
	if ntpResult, err := ntp.Collect(); err != nil {
		errs = append(errs, fmt.Sprintf("NTP: %v", err))
	} else if offset, ok := ntpResult.(int64); !ok {
		errs = append(errs, fmt.Sprintf("NTP 采集器返回类型错误: %T", ntpResult))
	} else {
		data.TimeOffset = offset
		success++
	}

	// 所有采集器均失败才放弃本轮上报
	if success == 0 {
		return nil, fmt.Errorf("所有采集器均失败: %s", strings.Join(errs, "; "))
	}

	if len(errs) > 0 {
		log.Printf("部分采集器失败（仍上报已成功的数据）: %s", strings.Join(errs, "; "))
	}

	return &data, nil
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
// 返回 error 供调用方感知失败（Token 丢失会导致重启后无法恢复会话）
func saveConfig(path string, cfg *AgentConfig, mu *sync.Mutex) error {
	mu.Lock()
	defer mu.Unlock()

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	// 先写入临时文件，再原子替换，避免写入过程中崩溃导致配置文件损坏
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return fmt.Errorf("保存配置文件失败: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("替换配置文件失败: %w", err)
	}
	return nil
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
