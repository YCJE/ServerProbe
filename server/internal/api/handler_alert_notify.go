package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/server-probe/server/internal/model"
	"github.com/server-probe/server/internal/repository"
	"github.com/server-probe/server/internal/service"
)

// AlertHandler 告警规则处理器
type AlertHandler struct {
	repo      *repository.AlertRepository
	notifyRepo *repository.NotifyRepository
	engine    *service.AlertEngine
}

// NewAlertHandler 创建告警规则处理器
func NewAlertHandler(repo *repository.AlertRepository, notifyRepo *repository.NotifyRepository, engine *service.AlertEngine) *AlertHandler {
	return &AlertHandler{repo: repo, notifyRepo: notifyRepo, engine: engine}
}

// HandleListAlerts 获取告警规则列表
// 路由: GET /api/v1/alerts
func (h *AlertHandler) HandleListAlerts(c *gin.Context) {
	rules, err := h.repo.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取告警规则失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"rules": rules})
}

// HandleCreateAlert 创建告警规则
// 路由: POST /api/v1/alerts
func (h *AlertHandler) HandleCreateAlert(c *gin.Context) {
	var req struct {
		Name            string  `json:"name"`
		Metric          string  `json:"metric"`
		Operator        string  `json:"operator"`
		Threshold       float64 `json:"threshold"`
		Duration        int     `json:"duration"`
		Enabled         bool    `json:"enabled"`
		NotifyChannelID int64   `json:"notify_channel_id"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求体"})
		return
	}

	if req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "规则名称不能为空"})
		return
	}
	if len(req.Name) > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "规则名称过长"})
		return
	}

	validMetrics := map[string]bool{
		model.MetricCPUUsage:    true,
		model.MetricMemUsage:    true,
		model.MetricDiskUsage:   true,
		model.MetricAgentOffline: true,
	}
	if !validMetrics[req.Metric] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的监控指标"})
		return
	}

	validOperators := map[string]bool{">": true, "<": true, "=": true}
	if !validOperators[req.Operator] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的操作符，支持 >, <, ="})
		return
	}

	if req.Duration < 1 || req.Duration > 86400 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "持续时间必须在 1-86400 秒之间"})
		return
	}

	// 阈值范围校验
	if err := validateThreshold(req.Metric, req.Threshold); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 验证通知渠道存在 (如果指定了)
	if req.NotifyChannelID > 0 {
		if _, err := h.notifyRepo.GetByID(req.NotifyChannelID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "通知渠道不存在"})
			return
		}
	}

	rule := &model.AlertRule{
		Name:            req.Name,
		Metric:          req.Metric,
		Operator:        req.Operator,
		Threshold:       req.Threshold,
		Duration:        req.Duration,
		Enabled:         req.Enabled,
		NotifyChannelID: req.NotifyChannelID,
	}

	if err := h.repo.Create(rule); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建告警规则失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"rule": rule})
}

// HandleUpdateAlert 更新告警规则
// 路由: PUT /api/v1/alerts/:id
func (h *AlertHandler) HandleUpdateAlert(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的规则 ID"})
		return
	}

	rule, err := h.repo.GetByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "告警规则不存在"})
		return
	}

	var req struct {
		Name            *string  `json:"name"`
		Metric          *string  `json:"metric"`
		Operator        *string  `json:"operator"`
		Threshold       *float64 `json:"threshold"`
		Duration        *int     `json:"duration"`
		Enabled         *bool    `json:"enabled"`
		NotifyChannelID *int64   `json:"notify_channel_id"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求体"})
		return
	}

	if req.Name != nil {
		if *req.Name == "" || len(*req.Name) > 100 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "规则名称无效"})
			return
		}
		rule.Name = *req.Name
	}
	if req.Metric != nil {
		validMetrics := map[string]bool{
			model.MetricCPUUsage:    true,
			model.MetricMemUsage:    true,
			model.MetricDiskUsage:   true,
			model.MetricAgentOffline: true,
		}
		if !validMetrics[*req.Metric] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "无效的监控指标"})
			return
		}
		rule.Metric = *req.Metric
	}
	if req.Operator != nil {
		validOperators := map[string]bool{">": true, "<": true, "=": true}
		if !validOperators[*req.Operator] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "无效的操作符"})
			return
		}
		rule.Operator = *req.Operator
	}
	if req.Threshold != nil {
		// 阈值范围校验 (使用更新后的 metric)
		metricForValidation := rule.Metric
		if req.Metric != nil {
			metricForValidation = *req.Metric
		}
		if err := validateThreshold(metricForValidation, *req.Threshold); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		rule.Threshold = *req.Threshold
	}
	if req.Duration != nil {
		if *req.Duration < 1 || *req.Duration > 86400 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "持续时间必须在 1-86400 秒之间"})
			return
		}
		rule.Duration = *req.Duration
	}
	if req.Enabled != nil {
		rule.Enabled = *req.Enabled
	}
	if req.NotifyChannelID != nil {
		if *req.NotifyChannelID > 0 {
			if _, err := h.notifyRepo.GetByID(*req.NotifyChannelID); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "通知渠道不存在"})
				return
			}
		}
		rule.NotifyChannelID = *req.NotifyChannelID
	}

	if err := h.repo.Update(rule); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新告警规则失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"rule": rule})
}

// HandleDeleteAlert 删除告警规则
// 路由: DELETE /api/v1/alerts/:id
func (h *AlertHandler) HandleDeleteAlert(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的规则 ID"})
		return
	}

	if err := h.repo.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除告警规则失败"})
		return
	}

	// 清理告警引擎中的状态
	if h.engine != nil {
		h.engine.CleanupStatesForRule(id)
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// validateThreshold 校验阈值范围
func validateThreshold(metric string, threshold float64) error {
	switch metric {
	case model.MetricCPUUsage, model.MetricMemUsage, model.MetricDiskUsage:
		if threshold < 0 || threshold > 100 {
			return fmt.Errorf("百分比指标的阈值必须在 0-100 之间")
		}
	case model.MetricAgentOffline:
		if threshold != 0 && threshold != 1 {
			return fmt.Errorf("离线指标的阈值必须为 0 或 1")
		}
	}
	return nil
}

// maskChannelConfig 脱敏通知渠道配置中的敏感字段
func maskChannelConfig(channel *model.NotifyChannel) gin.H {
	result := gin.H{
		"id":         channel.ID,
		"name":       channel.Name,
		"type":       channel.Type,
		"created_at": channel.CreatedAt,
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal([]byte(channel.Config), &cfg); err == nil {
		safeCfg := make(map[string]interface{})
		for k, v := range cfg {
			switch k {
			case "password", "secret", "bot_token", "smtp_password":
				safeCfg[k] = "******"
			default:
				safeCfg[k] = v
			}
		}
		result["config"] = safeCfg
	}
	return result
}

// HandleTestAlert 测试告警规则 (手动触发一次通知)
// 路由: POST /api/v1/alerts/:id/test
func (h *AlertHandler) HandleTestAlert(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的规则 ID"})
		return
	}

	rule, err := h.repo.GetByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "告警规则不存在"})
		return
	}

	if rule.NotifyChannelID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "该规则未绑定通知渠道"})
		return
	}

	// 通过告警引擎发送测试通知
	if h.engine == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "告警引擎不可用"})
		return
	}

	if err := h.engine.SendTestNotification(rule); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "测试通知发送失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// NotifyHandler 通知渠道处理器
type NotifyHandler struct {
	repo      *repository.NotifyRepository
	notifySvc *service.NotifyService
	alertRepo *repository.AlertRepository
}

// NewNotifyHandler 创建通知渠道处理器
func NewNotifyHandler(repo *repository.NotifyRepository, notifySvc *service.NotifyService, alertRepo *repository.AlertRepository) *NotifyHandler {
	return &NotifyHandler{repo: repo, notifySvc: notifySvc, alertRepo: alertRepo}
}

// HandleListChannels 获取通知渠道列表
// 路由: GET /api/v1/notify/channels
func (h *NotifyHandler) HandleListChannels(c *gin.Context) {
	channels, err := h.repo.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取通知渠道失败"})
		return
	}

	// 隐藏敏感信息
	result := make([]gin.H, 0, len(channels))
	for _, ch := range channels {
		item := gin.H{
			"id":         ch.ID,
			"name":       ch.Name,
			"type":       ch.Type,
			"created_at": ch.CreatedAt,
		}
		// 解析配置，隐藏密码/密钥
		var cfg map[string]interface{}
		if err := json.Unmarshal([]byte(ch.Config), &cfg); err == nil {
			safeCfg := make(map[string]interface{})
			for k, v := range cfg {
				switch k {
				case "password", "secret", "bot_token", "smtp_password":
					safeCfg[k] = "******"
				default:
					safeCfg[k] = v
				}
			}
			item["config"] = safeCfg
		}
		result = append(result, item)
	}

	c.JSON(http.StatusOK, gin.H{"channels": result})
}

// HandleCreateChannel 创建通知渠道
// 路由: POST /api/v1/notify/channels
func (h *NotifyHandler) HandleCreateChannel(c *gin.Context) {
	var req struct {
		Name   string `json:"name"`
		Type   string `json:"type"`
		Config string `json:"config"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求体"})
		return
	}

	if req.Name == "" || len(req.Name) > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "渠道名称无效"})
		return
	}

	validTypes := map[string]bool{
		model.NotifyTypeWebhook:  true,
		model.NotifyTypeTelegram: true,
		model.NotifyTypeEmail:    true,
	}
	if !validTypes[req.Type] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的渠道类型，支持: webhook, telegram, email"})
		return
	}

	// 验证配置格式
	if req.Config == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "渠道配置不能为空"})
		return
	}
	var cfgMap map[string]interface{}
	if err := json.Unmarshal([]byte(req.Config), &cfgMap); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "渠道配置必须是有效的 JSON"})
		return
	}

	// 使用 NotifyService 验证配置
	if h.notifySvc != nil {
		if err := h.notifySvc.ValidateChannelConfig(req.Type, req.Config); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}

	channel := &model.NotifyChannel{
		Name:   req.Name,
		Type:   req.Type,
		Config: req.Config,
	}

	if err := h.repo.Create(channel); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建通知渠道失败"})
		return
	}

	// 返回脱敏后的渠道信息
	c.JSON(http.StatusOK, gin.H{"channel": maskChannelConfig(channel)})
}

// HandleUpdateChannel 更新通知渠道
// 路由: PUT /api/v1/notify/channels/:id
func (h *NotifyHandler) HandleUpdateChannel(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的渠道 ID"})
		return
	}

	channel, err := h.repo.GetByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "通知渠道不存在"})
		return
	}

	var req struct {
		Name   *string `json:"name"`
		Type   *string `json:"type"`
		Config *string `json:"config"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求体"})
		return
	}

	if req.Name != nil {
		if *req.Name == "" || len(*req.Name) > 100 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "渠道名称无效"})
			return
		}
		channel.Name = *req.Name
	}
	if req.Type != nil {
		validTypes := map[string]bool{
			model.NotifyTypeWebhook:  true,
			model.NotifyTypeTelegram: true,
			model.NotifyTypeEmail:    true,
		}
		if !validTypes[*req.Type] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "无效的渠道类型"})
			return
		}
		channel.Type = *req.Type
	}
	if req.Config != nil {
		var newCfg map[string]interface{}
		if err := json.Unmarshal([]byte(*req.Config), &newCfg); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "渠道配置必须是有效的 JSON"})
			return
		}

		// 合并旧配置中的敏感字段 (密码/密钥留空时保留旧值)
		var oldCfg map[string]interface{}
		if err := json.Unmarshal([]byte(channel.Config), &oldCfg); err == nil {
			sensitiveKeys := map[string]bool{
				"password": true, "secret": true, "bot_token": true, "smtp_password": true,
			}
			for k, oldVal := range oldCfg {
				if sensitiveKeys[k] {
					if newVal, exists := newCfg[k]; !exists || newVal == "" {
						newCfg[k] = oldVal
					}
				}
			}
		}

		mergedConfig, _ := json.Marshal(newCfg)
		if h.notifySvc != nil {
			if err := h.notifySvc.ValidateChannelConfig(channel.Type, string(mergedConfig)); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
		}
		channel.Config = string(mergedConfig)
	}

	if err := h.repo.Update(channel); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新通知渠道失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"channel": maskChannelConfig(channel)})
}

// HandleDeleteChannel 删除通知渠道
// 路由: DELETE /api/v1/notify/channels/:id
func (h *NotifyHandler) HandleDeleteChannel(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的渠道 ID"})
		return
	}

	// 引用完整性检查: 检查是否有告警规则引用该通知渠道
	if h.alertRepo != nil {
		count, err := h.alertRepo.CountByNotifyChannelID(id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "检查引用关系失败"})
			return
		}
		if count > 0 {
			c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("该通知渠道被 %d 条告警规则引用，请先解除关联后再删除", count)})
			return
		}
	}

	if err := h.repo.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除通知渠道失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// HandleTestChannel 测试通知渠道
// 路由: POST /api/v1/notify/channels/:id/test
func (h *NotifyHandler) HandleTestChannel(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的渠道 ID"})
		return
	}

	if h.notifySvc == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "通知服务不可用"})
		return
	}

	if err := h.notifySvc.TestChannel(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "测试通知发送失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}
