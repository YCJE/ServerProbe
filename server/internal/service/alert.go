package service

import (
	"fmt"
	"log"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/server-probe/server/internal/model"
	"github.com/server-probe/server/internal/repository"
)

// AlertEngine 告警引擎
type AlertEngine struct {
	alertRepo         *repository.AlertRepository
	monitor           *MonitorService
	notifySvc         *NotifyService
	validator         *DataValidator
	serviceMonitorRepo *repository.ServiceMonitorRepository // P0-3: 服务监控告警
	sslMonitorRepo     *repository.SSLCertMonitorRepository  // P0-4: SSL 证书告警

	// 告警状态跟踪
	states  map[string]*alertState // key: "agentID:ruleID"
	mu      sync.RWMutex
	ticker  *time.Ticker
	stopCh  chan struct{}
	stopOnce sync.Once
	wg      sync.WaitGroup // 跟踪后台 goroutine

	// 静默期（默认 60 分钟）
	silencePeriod time.Duration
}

// alertState 告警状态
type alertState struct {
	state          model.AlertState
	firstTriggered time.Time // 首次超阈值时间
	lastNotified   time.Time // 上次通知时间
}

// NewAlertEngine 创建告警引擎
func NewAlertEngine(
	alertRepo *repository.AlertRepository,
	monitor *MonitorService,
	notifySvc *NotifyService,
) *AlertEngine {
	return &AlertEngine{
		alertRepo:     alertRepo,
		monitor:       monitor,
		notifySvc:     notifySvc,
		states:        make(map[string]*alertState),
		stopCh:        make(chan struct{}),
		silencePeriod: 60 * time.Minute,
	}
}

// SetMonitorRepos 注入服务监控和 SSL 监控 Repository（用于全局指标告警）
func (e *AlertEngine) SetMonitorRepos(
	serviceMonitorRepo *repository.ServiceMonitorRepository,
	sslMonitorRepo *repository.SSLCertMonitorRepository,
) {
	e.serviceMonitorRepo = serviceMonitorRepo
	e.sslMonitorRepo = sslMonitorRepo
}

// Start 启动告警引擎
func (e *AlertEngine) Start() {
	e.ticker = time.NewTicker(10 * time.Second)

	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		for {
			select {
			case <-e.ticker.C:
				e.checkAlerts()
			case <-e.stopCh:
				return
			}
		}
	}()

	log.Println("告警引擎已启动")
}

// Stop 停止告警引擎
func (e *AlertEngine) Stop() {
	e.stopOnce.Do(func() {
		if e.ticker != nil {
			e.ticker.Stop()
		}
		close(e.stopCh)
		e.wg.Wait()
	})
}

// CleanupStatesForAgent 清理指定 Agent 的所有告警状态 (删除 Agent 时调用)
func (e *AlertEngine) CleanupStatesForAgent(agentID int64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	prefix := fmt.Sprintf("%d:", agentID)
	for key := range e.states {
		if strings.HasPrefix(key, prefix) {
			delete(e.states, key)
		}
	}
}

// CleanupStatesForRule 清理指定规则的所有告警状态 (删除规则时调用)
func (e *AlertEngine) CleanupStatesForRule(ruleID int64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	suffix := fmt.Sprintf(":%d", ruleID)
	for key := range e.states {
		if strings.HasSuffix(key, suffix) {
			delete(e.states, key)
		}
	}
}

// checkAlerts 检查所有告警规则
func (e *AlertEngine) checkAlerts() {
	// 获取已启用的告警规则
	rules, err := e.alertRepo.ListEnabled()
	if err != nil {
		log.Printf("获取告警规则失败: %v", err)
		return
	}

	// 获取所有 Agent（包括离线的），用于 agent_offline 指标检查
	allAgents := e.monitor.GetAllAgentIDs()

	// 清理不在 Agent 列表中的过期告警状态（Agent 已被删除但状态残留）
	e.cleanupStaleStates(allAgents)

	// 分离 per-agent 规则、元数据规则和全局规则
	var perAgentRules, metaRules, globalRules []model.AlertRule
	for _, rule := range rules {
		switch rule.Metric {
		case model.MetricServiceStatus, model.MetricSSLCertExpiry:
			globalRules = append(globalRules, rule)
		case model.MetricTrafficQuota, model.MetricExpireDays:
			metaRules = append(metaRules, rule)
		default:
			perAgentRules = append(perAgentRules, rule)
		}
	}

	// 按 Agent 分组检查 per-agent 规则，每个 Agent 只读一次 RingBuffer（避免 N+1 读取）
	for _, agentID := range allAgents {
		isOnline := e.monitor.IsAgentOnline(agentID)
		var points []repository.MetricPoint
		if isOnline {
			rb := e.monitor.GetRingBuffer(agentID)
			if rb != nil {
				points = rb.Latest(1)
			}
		}
		for _, rule := range perAgentRules {
			if rule.Metric == model.MetricAgentOffline {
				// agent_offline 检查所有 Agent（包括在线的）
				e.checkRuleForAgent(rule, agentID, nil)
			} else if isOnline && len(points) > 0 {
				e.checkRuleForAgent(rule, agentID, points)
			}
		}
	}

	// 检查元数据规则（traffic_quota / expire_days）：值来自 Agent 元数据与月度流量聚合
	if len(metaRules) > 0 {
		agents := e.monitor.GetAllAgents()
		monthly := e.monitor.GetMonthlyTraffic()
		now := time.Now()
		for i := range agents {
			agent := &agents[i]
			for _, rule := range metaRules {
				value := getMetaMetricValue(rule.Metric, agent, monthly[agent.ID], now)
				e.evaluateRule(rule, agent.ID, value)
			}
		}
	}

	// 检查全局规则（service_status, ssl_cert_expiry），使用 agentID=0 作为状态键
	for _, rule := range globalRules {
		value := e.getGlobalMetricValue(rule.Metric)
		if value < 0 {
			continue
		}
		e.checkRuleGlobal(rule, value)
	}
}

// cleanupStaleStates 清理不属于当前 Agent 列表的过期告警状态
func (e *AlertEngine) cleanupStaleStates(activeAgentIDs []int64) {
	activeSet := make(map[int64]bool, len(activeAgentIDs))
	for _, id := range activeAgentIDs {
		activeSet[id] = true
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	for key := range e.states {
		// key 格式为 "agentID:ruleID"
		var agentID, ruleID int64
		if _, err := fmt.Sscanf(key, "%d:%d", &agentID, &ruleID); err != nil {
			continue
		}
		// 保留 agentID=0 的全局规则状态（service_status, ssl_cert_expiry）
		if agentID == 0 {
			continue
		}
		if !activeSet[agentID] {
			delete(e.states, key)
		}
	}
}

// checkRuleForAgent 检查单个 Agent 的单条规则（接受预读取的数据点，避免重复读取 RingBuffer）
func (e *AlertEngine) checkRuleForAgent(rule model.AlertRule, agentID int64, points []repository.MetricPoint) {
	// 获取当前指标值
	value := e.getMetricValue(agentID, rule.Metric, points)
	if value < 0 {
		return
	}
	e.evaluateRule(rule, agentID, value)
}

// getMetaMetricValue 获取元数据类指标值（traffic_quota / expire_days）
// 值来自管理员设置的 NodeGet 风格元数据与月度流量聚合，与 RingBuffer 无关
// 未设置配额/到期时间的 Agent 返回"永不触发"的哨兵值，保证已 FIRING 的告警能自动恢复
func getMetaMetricValue(metric string, agent *model.Agent, monthly repository.MonthlyTrafficAgg, now time.Time) float64 {
	switch metric {
	case model.MetricTrafficQuota:
		if agent.TrafficQuotaBytes <= 0 {
			return 0 // 未设置配额 = 不限流量，使用率恒为 0%
		}
		used := monthly.Rx + monthly.Tx
		return float64(used) / float64(agent.TrafficQuotaBytes) * 100

	case model.MetricExpireDays:
		if agent.ExpiresAt == nil {
			return math.MaxFloat64 / 1e6 // 永不过期，剩余天数视为极大（配合 < 阈值永不触发）
		}
		days := agent.ExpiresAt.Sub(now).Hours() / 24
		if days < 0 {
			return 0 // 已过期
		}
		return days

	default:
		return -1
	}
}

// evaluateRule 评估一条规则对单个 Agent 的当前值并推进告警状态机
// per-agent 规则、元数据规则、全局规则共用同一套状态转移逻辑
func (e *AlertEngine) evaluateRule(rule model.AlertRule, agentID int64, value float64) {
	key := fmt.Sprintf("%d:%d", agentID, rule.ID)

	// 检查是否超阈值
	thresholdExceeded := e.checkThreshold(value, rule.Operator, rule.Threshold)

	now := time.Now()

	// 待发送通知（锁外执行，避免持锁发送通知导致阻塞）
	type pendingNotification struct {
		rule    model.AlertRule
		agentID int64
		value   float64
		state   model.AlertState
	}
	var pendingNotifications []pendingNotification

	// 整个状态检查和修改过程放在锁内
	e.mu.Lock()

	state, ok := e.states[key]
	if !ok {
		state = &alertState{state: model.AlertStateOK}
		e.states[key] = state
	}

	if thresholdExceeded {
		switch state.state {
		case model.AlertStateOK:
			// OK → PENDING
			state.state = model.AlertStatePending
			state.firstTriggered = now

		case model.AlertStatePending:
			// PENDING → FIRING（达到 duration）
			if now.Sub(state.firstTriggered) >= time.Duration(rule.Duration)*time.Second {
				state.state = model.AlertStateFiring
				state.lastNotified = now
				pendingNotifications = append(pendingNotifications, pendingNotification{rule, agentID, value, model.AlertStateFiring})
			}

		case model.AlertStateFiring:
			// 检查静默期
			if now.Sub(state.lastNotified) >= e.silencePeriod {
				state.lastNotified = now
				pendingNotifications = append(pendingNotifications, pendingNotification{rule, agentID, value, model.AlertStateFiring})
			}

		case model.AlertStateResolved:
			// RESOLVED → PENDING
			state.state = model.AlertStatePending
			state.firstTriggered = now
		}
	} else {
		switch state.state {
		case model.AlertStatePending:
			// PENDING → OK
			state.state = model.AlertStateOK

		case model.AlertStateFiring:
			// FIRING → RESOLVED
			state.state = model.AlertStateResolved
			pendingNotifications = append(pendingNotifications, pendingNotification{rule, agentID, value, model.AlertStateResolved})

		case model.AlertStateResolved:
			// RESOLVED → OK (已恢复告警回到正常状态，避免 states map 中条目永远存在)
			state.state = model.AlertStateOK
		}
	}

	e.mu.Unlock()

	// 锁外发送通知
	for _, n := range pendingNotifications {
		e.sendAlertNotification(n.rule, n.agentID, n.value, n.state)
	}
}

// getMetricValue 获取指标值（使用预读取的数据点，避免重复读取 RingBuffer）
func (e *AlertEngine) getMetricValue(agentID int64, metric string, points []repository.MetricPoint) float64 {
	// agent_offline 特殊处理: 在线返回 0，离线返回 1
	if metric == model.MetricAgentOffline {
		if e.monitor.IsAgentOnline(agentID) {
			return 0
		}
		return 1
	}

	if len(points) == 0 {
		return -1
	}

	switch metric {
	case model.MetricCPUUsage:
		return points[0].CPU
	case model.MetricMemUsage:
		return points[0].Mem
	case model.MetricDiskUsage:
		// 聚合所有磁盘计算总使用率，与 monitor.go/handler_server.go 保持一致
		var totalTotal, totalUsed uint64
		for _, d := range points[0].Disks {
			totalTotal += d.Total
			totalUsed += d.Used
		}
		if totalTotal > 0 {
			return float64(totalUsed) / float64(totalTotal) * 100
		}
		return -1
	default:
		return -1
	}
}

// getGlobalMetricValue 获取全局指标值（非 per-agent 指标）
// service_status: 返回 1 如果有任一服务 down，0 如果全部 up
// ssl_cert_expiry: 返回所有 SSL 证书中最小的剩余天数
func (e *AlertEngine) getGlobalMetricValue(metric string) float64 {
	switch metric {
	case model.MetricServiceStatus:
		if e.serviceMonitorRepo == nil {
			return -1
		}
		monitors, err := e.serviceMonitorRepo.ListEnabled()
		if err != nil {
			log.Printf("获取服务监控列表失败: %v", err)
			return -1
		}
		for _, m := range monitors {
			if m.LastStatus == "down" {
				return 1
			}
		}
		return 0

	case model.MetricSSLCertExpiry:
		if e.sslMonitorRepo == nil {
			return -1
		}
		monitors, err := e.sslMonitorRepo.ListEnabled()
		if err != nil {
			log.Printf("获取 SSL 证书监控列表失败: %v", err)
			return -1
		}
		minDays := -1
		for _, m := range monitors {
			if m.LastRemainingDays < minDays || minDays == -1 {
				minDays = m.LastRemainingDays
			}
		}
		if minDays == -1 {
			return -1 // 无已检查的监控项
		}
		return float64(minDays)

	default:
		return -1
	}
}

// checkRuleGlobal 检查全局规则（使用 agentID=0 作为状态键）
func (e *AlertEngine) checkRuleGlobal(rule model.AlertRule, value float64) {
	e.evaluateRule(rule, 0, value)
}

// checkThreshold 检查阈值
func (e *AlertEngine) checkThreshold(value float64, operator string, threshold float64) bool {
	switch operator {
	case model.OpGreaterThan:
		return value > threshold
	case model.OpLessThan:
		return value < threshold
	case model.OpEqual:
		// 使用容差比较避免浮点精度问题
		return (value-threshold) < 0.001 && (threshold-value) < 0.001
	default:
		return false
	}
}

// getServerDisplayName 获取 Agent 显示名（display_name > hostname > "Agent #id"）
// 通知频率低（状态切换/静默期），5 秒 TTL 缓存的列表查找开销可忽略
func (e *AlertEngine) getServerDisplayName(agentID int64) string {
	agents := e.monitor.GetAllAgents()
	for i := range agents {
		if agents[i].ID == agentID {
			if agents[i].DisplayName != "" {
				return agents[i].DisplayName
			}
			if agents[i].Hostname != "" {
				return agents[i].Hostname
			}
			break
		}
	}
	return fmt.Sprintf("Agent #%d", agentID)
}

// sendAlertNotification 发送告警通知
func (e *AlertEngine) sendAlertNotification(rule model.AlertRule, agentID int64, value float64, state model.AlertState) {
	if e.notifySvc == nil {
		return
	}

	if rule.NotifyChannelID == 0 {
		return
	}

	var title, content string
	// 全局规则（agentID=0）使用不同的消息格式
	if agentID == 0 {
		if state == model.AlertStateFiring {
			title = fmt.Sprintf("[告警] %s", rule.Name)
			content = fmt.Sprintf("全局指标 %s 当前值 %.2f %s %.2f，已持续 %d 秒",
				rule.Metric, value, rule.Operator, rule.Threshold, rule.Duration)
		} else {
			title = fmt.Sprintf("[恢复] %s", rule.Name)
			content = fmt.Sprintf("全局指标 %s 已恢复正常（当前值 %.2f）",
				rule.Metric, value)
		}
	} else {
		serverName := e.getServerDisplayName(agentID)
		if state == model.AlertStateFiring {
			title = fmt.Sprintf("[告警] %s", rule.Name)
			content = fmt.Sprintf("服务器 %s 的 %s 当前值 %.2f %s %.2f，已持续 %d 秒",
				serverName, rule.Metric, value, rule.Operator, rule.Threshold, rule.Duration)
		} else {
			title = fmt.Sprintf("[恢复] %s", rule.Name)
			content = fmt.Sprintf("服务器 %s 的 %s 已恢复正常（当前值 %.2f）",
				serverName, rule.Metric, value)
		}
	}

	err := e.notifySvc.SendNotification(rule.NotifyChannelID, title, content)
	if err != nil {
		log.Printf("发送告警通知失败: %v", err)
	}
}

// SendTestNotification 发送测试通知
func (e *AlertEngine) SendTestNotification(rule *model.AlertRule) error {
	if e.notifySvc == nil {
		return fmt.Errorf("通知服务不可用")
	}
	if rule.NotifyChannelID == 0 {
		return fmt.Errorf("该规则未绑定通知渠道")
	}

	title := fmt.Sprintf("[测试] %s", rule.Name)
	content := fmt.Sprintf("这是一条测试通知。规则: %s, 指标: %s, 阈值: %.2f, 操作符: %s",
		rule.Name, rule.Metric, rule.Threshold, rule.Operator)

	return e.notifySvc.SendNotification(rule.NotifyChannelID, title, content)
}
