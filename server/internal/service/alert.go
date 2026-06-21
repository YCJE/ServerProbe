package service

import (
	"fmt"
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

	for _, rule := range rules {
		for _, agentID := range onlineAgents {
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
