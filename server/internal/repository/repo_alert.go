package repository

import (
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

// Delete 删除告警规则
func (r *AlertRepository) Delete(id int64) error {
	return r.db.Delete(&model.AlertRule{}, id).Error
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

// Delete 删除探测目标
func (r *PingTargetRepository) Delete(id int64) error {
	return r.db.Delete(&model.PingTarget{}, id).Error
}
