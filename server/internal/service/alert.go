package service

import (
	"fmt"
	"strings"
	"log"
	"sync"
	"time"

	"github.com/server-probe/server/internal/model"
	"github.com/server-probe/server/internal/repository"
)

// AlertEngine 告警引擎
type AlertEngine struct {
	alertRepo   *repository.AlertRepository
	monitor     *MonitorService
	notifySvc   *NotifyService
	validator   *DataValidator

	// 告警状态跟踪
	states  map[string]*alertState // key: "agentID:ruleID"
	mu      sync.RWMutex
	ticker  *time.Ticker
	stopCh  chan struct{}

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

// Start 启动告警引擎
func (e *AlertEngine) Start() {
	e.ticker = time.NewTicker(10 * time.Second)

	go func() {
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
	if e.ticker != nil {
		e.ticker.Stop()
	}
	close(e.stopCh)
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

	// 获取所有在线 Agent
	onlineAgents := e.monitor.GetOnlineAgentIDs()
	// 获取所有 Agent（包括离线的），用于 agent_offline 指标检查
	allAgents := e.monitor.GetAllAgentIDs()

	for _, rule := range rules {
		// agent_offline 指标需要检查所有 Agent（包括离线的），
		// 其他指标只需检查在线 Agent
		var agentIDs []int64
		if rule.Metric == model.MetricAgentOffline {
			agentIDs = allAgents
		} else {
			agentIDs = onlineAgents
		}
		for _, agentID := range agentIDs {
			e.checkRuleForAgent(rule, agentID)
		}
	}
}

// checkRuleForAgent 检查单个 Agent 的单条规则
func (e *AlertEngine) checkRuleForAgent(rule model.AlertRule, agentID int64) {
	key := fmt.Sprintf("%d:%d", agentID, rule.ID)

	// 获取当前指标值
	value := e.getMetricValue(agentID, rule.Metric)
	if value < 0 {
		return
	}

	// 检查是否超阈值
	thresholdExceeded := e.checkThreshold(value, rule.Operator, rule.Threshold)

	e.mu.Lock()
	state, ok := e.states[key]
	if !ok {
		state = &alertState{state: model.AlertStateOK}
		e.states[key] = state
	}
	e.mu.Unlock()

	now := time.Now()

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
				e.sendAlertNotification(rule, agentID, value, model.AlertStateFiring)
			}

		case model.AlertStateFiring:
			// 检查静默期
			if now.Sub(state.lastNotified) >= e.silencePeriod {
				state.lastNotified = now
				e.sendAlertNotification(rule, agentID, value, model.AlertStateFiring)
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
			e.sendAlertNotification(rule, agentID, value, model.AlertStateResolved)
		}
	}
}

// getMetricValue 获取指标值
func (e *AlertEngine) getMetricValue(agentID int64, metric string) float64 {
	// agent_offline 特殊处理: 在线返回 0，离线返回 1
	if metric == model.MetricAgentOffline {
		if e.monitor.IsAgentOnline(agentID) {
			return 0
		}
		return 1
	}

	rb := e.monitor.GetRingBuffer(agentID)
	if rb == nil {
		return -1
	}

	points := rb.Latest(1)
	if len(points) == 0 {
		return -1
	}

	switch metric {
	case model.MetricCPUUsage:
		return points[0].CPU
	case model.MetricMemUsage:
		return points[0].Mem
	case model.MetricDiskUsage:
		if len(points[0].Disks) > 0 {
			d := points[0].Disks[0]
			if d.Total > 0 {
				return float64(d.Used) / float64(d.Total) * 100
			}
		}
		return -1
	default:
		return -1
	}
}

// checkThreshold 检查阈值
func (e *AlertEngine) checkThreshold(value float64, operator string, threshold float64) bool {
	switch operator {
	case model.OpGreaterThan:
		return value > threshold
	case model.OpLessThan:
		return value < threshold
	case model.OpEqual:
		return value == threshold
	default:
		return false
	}
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
	if state == model.AlertStateFiring {
		title = fmt.Sprintf("[告警] %s", rule.Name)
		content = fmt.Sprintf("Agent %d 的 %s 当前值 %.2f %s %.2f，已持续 %d 秒",
			agentID, rule.Metric, value, rule.Operator, rule.Threshold, rule.Duration)
	} else {
		title = fmt.Sprintf("[恢复] %s", rule.Name)
		content = fmt.Sprintf("Agent %d 的 %s 已恢复正常（当前值 %.2f）",
			agentID, rule.Metric, value)
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
	content := fmt.Sprintf("这是一条测试通知。规则: %s, 指标: %s, 阈值: %.2f %s %.2f",
		rule.Name, rule.Metric, rule.Threshold, rule.Operator, rule.Threshold)

	return e.notifySvc.SendNotification(rule.NotifyChannelID, title, content)
}
