package repository

import (
	"time"

	"gorm.io/gorm"

	"github.com/server-probe/server/internal/model"
)

// AlertRepository 告警规则 CRUD
type AlertRepository struct {
	db *gorm.DB
}

// NewAlertRepository 创建告警规则 repository
func NewAlertRepository(db *gorm.DB) *AlertRepository {
	return &AlertRepository{db: db}
}

// Create 创建告警规则
func (r *AlertRepository) Create(rule *model.AlertRule) error {
	return r.db.Create(rule).Error
}

// GetByID 根据 ID 获取告警规则
func (r *AlertRepository) GetByID(id int64) (*model.AlertRule, error) {
	var rule model.AlertRule
	if err := r.db.First(&rule, id).Error; err != nil {
		return nil, err
	}
	return &rule, nil
}

// List 获取所有告警规则
func (r *AlertRepository) List() ([]model.AlertRule, error) {
	var rules []model.AlertRule
	if err := r.db.Order("id ASC").Find(&rules).Error; err != nil {
		return nil, err
	}
	return rules, nil
}

// ListEnabled 获取已启用的告警规则
func (r *AlertRepository) ListEnabled() ([]model.AlertRule, error) {
	var rules []model.AlertRule
	if err := r.db.Where("enabled = ?", true).Find(&rules).Error; err != nil {
		return nil, err
	}
	return rules, nil
}

// Update 更新告警规则
func (r *AlertRepository) Update(rule *model.AlertRule) error {
	return r.db.Save(rule).Error
}

// UpdateEnabled 使用 Select 强制更新 enabled 字段，避免 GORM default tag 导致零值被忽略
func (r *AlertRepository) UpdateEnabled(rule *model.AlertRule, enabled bool) error {
	return r.db.Model(rule).Select("enabled").Update("enabled", enabled).Error
}

// Delete 删除告警规则
func (r *AlertRepository) Delete(id int64) error {
	return r.db.Delete(&model.AlertRule{}, id).Error
}

// CountByNotifyChannelID 统计引用指定通知渠道的告警规则数量
func (r *AlertRepository) CountByNotifyChannelID(channelID int64) (int64, error) {
	var count int64
	err := r.db.Model(&model.AlertRule{}).Where("notify_channel_id = ?", channelID).Count(&count).Error
	return count, err
}

// CreateHistory 落盘一条告警历史（FIRING 触发）
func (r *AlertRepository) CreateHistory(h *model.AlertHistory) error {
	return r.db.Create(h).Error
}

// ResolveHistoryByID 按 ID 补记恢复信息（RESOLVED）
func (r *AlertRepository) ResolveHistoryByID(id int64, resolvedAt time.Time, resolvedValue float64) error {
	return r.db.Model(&model.AlertHistory{}).Where("id = ?", id).
		Updates(map[string]interface{}{
			"state":          "resolved",
			"resolved_at":    resolvedAt,
			"resolved_value": resolvedValue,
		}).Error
}

// CountHistory 统计告警历史条数（用于分页）
func (r *AlertRepository) CountHistory(state string, agentID, ruleID int64) (int64, error) {
	q := r.db.Model(&model.AlertHistory{})
	if state != "" {
		q = q.Where("state = ?", state)
	}
	if agentID > 0 {
		q = q.Where("agent_id = ?", agentID)
	}
	if ruleID > 0 {
		q = q.Where("rule_id = ?", ruleID)
	}
	var count int64
	err := q.Count(&count).Error
	return count, err
}

// ListHistory 分页查询告警历史（按触发时间倒序）
func (r *AlertRepository) ListHistory(state string, agentID, ruleID int64, offset, limit int) ([]model.AlertHistory, error) {
	q := r.db.Model(&model.AlertHistory{})
	if state != "" {
		q = q.Where("state = ?", state)
	}
	if agentID > 0 {
		q = q.Where("agent_id = ?", agentID)
	}
	if ruleID > 0 {
		q = q.Where("rule_id = ?", ruleID)
	}
	var histories []model.AlertHistory
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	err := q.Order("triggered_at DESC").Offset(offset).Limit(limit).Find(&histories).Error
	return histories, err
}

// CleanupHistoryBefore 删除指定时间之前的告警历史
func (r *AlertRepository) CleanupHistoryBefore(t time.Time) (int64, error) {
	res := r.db.Where("triggered_at < ?", t).Delete(&model.AlertHistory{})
	return res.RowsAffected, res.Error
}

// NotifyRepository 通知渠道 CRUD
type NotifyRepository struct {
	db *gorm.DB
}

// NewNotifyRepository 创建通知渠道 repository
func NewNotifyRepository(db *gorm.DB) *NotifyRepository {
	return &NotifyRepository{db: db}
}

// Create 创建通知渠道
func (r *NotifyRepository) Create(channel *model.NotifyChannel) error {
	return r.db.Create(channel).Error
}

// GetByID 根据 ID 获取通知渠道
func (r *NotifyRepository) GetByID(id int64) (*model.NotifyChannel, error) {
	var channel model.NotifyChannel
	if err := r.db.First(&channel, id).Error; err != nil {
		return nil, err
	}
	return &channel, nil
}

// List 获取所有通知渠道
func (r *NotifyRepository) List() ([]model.NotifyChannel, error) {
	var channels []model.NotifyChannel
	if err := r.db.Order("id ASC").Find(&channels).Error; err != nil {
		return nil, err
	}
	return channels, nil
}

// Update 更新通知渠道
func (r *NotifyRepository) Update(channel *model.NotifyChannel) error {
	return r.db.Save(channel).Error
}

// Delete 删除通知渠道
func (r *NotifyRepository) Delete(id int64) error {
	return r.db.Delete(&model.NotifyChannel{}, id).Error
}

// PingTargetRepository 探测目标 CRUD
type PingTargetRepository struct {
	db *gorm.DB
}

// NewPingTargetRepository 创建探测目标 repository
func NewPingTargetRepository(db *gorm.DB) *PingTargetRepository {
	return &PingTargetRepository{db: db}
}

// Create 创建探测目标
func (r *PingTargetRepository) Create(target *model.PingTarget) error {
	return r.db.Create(target).Error
}

// GetByID 根据 ID 获取探测目标
func (r *PingTargetRepository) GetByID(id int64) (*model.PingTarget, error) {
	var target model.PingTarget
	if err := r.db.First(&target, id).Error; err != nil {
		return nil, err
	}
	return &target, nil
}

// List 获取所有探测目标
func (r *PingTargetRepository) List() ([]model.PingTarget, error) {
	var targets []model.PingTarget
	if err := r.db.Order("sort_order ASC, id ASC").Find(&targets).Error; err != nil {
		return nil, err
	}
	return targets, nil
}

// ListEnabled 获取已启用的探测目标
func (r *PingTargetRepository) ListEnabled() ([]model.PingTarget, error) {
	var targets []model.PingTarget
	if err := r.db.Where("enabled = ?", true).Order("sort_order ASC, id ASC").Find(&targets).Error; err != nil {
		return nil, err
	}
	return targets, nil
}

// Update 更新探测目标
func (r *PingTargetRepository) Update(target *model.PingTarget) error {
	return r.db.Save(target).Error
}

// UpdateEnabled 使用 Select 强制更新 enabled 字段，避免 GORM default tag 导致零值被忽略
func (r *PingTargetRepository) UpdateEnabled(target *model.PingTarget, enabled bool) error {
	return r.db.Model(target).Select("enabled").Update("enabled", enabled).Error
}

// Delete 删除探测目标
func (r *PingTargetRepository) Delete(id int64) error {
	return r.db.Delete(&model.PingTarget{}, id).Error
}
