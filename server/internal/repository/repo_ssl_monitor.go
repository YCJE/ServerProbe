package repository

import (
	"gorm.io/gorm"

	"github.com/server-probe/server/internal/model"
)

// SSLCertMonitorRepository SSL 证书监控 CRUD
type SSLCertMonitorRepository struct {
	db *gorm.DB
}

// NewSSLCertMonitorRepository 创建 SSL 证书监控 repository
func NewSSLCertMonitorRepository(db *gorm.DB) *SSLCertMonitorRepository {
	return &SSLCertMonitorRepository{db: db}
}

// Create 创建 SSL 证书监控
func (r *SSLCertMonitorRepository) Create(monitor *model.SSLCertMonitor) error {
	return r.db.Create(monitor).Error
}

// GetByID 根据 ID 获取 SSL 证书监控
func (r *SSLCertMonitorRepository) GetByID(id int64) (*model.SSLCertMonitor, error) {
	var monitor model.SSLCertMonitor
	if err := r.db.First(&monitor, id).Error; err != nil {
		return nil, err
	}
	return &monitor, nil
}

// List 获取所有 SSL 证书监控
func (r *SSLCertMonitorRepository) List() ([]model.SSLCertMonitor, error) {
	var monitors []model.SSLCertMonitor
	if err := r.db.Order("id ASC").Find(&monitors).Error; err != nil {
		return nil, err
	}
	return monitors, nil
}

// ListEnabled 获取已启用的 SSL 证书监控
func (r *SSLCertMonitorRepository) ListEnabled() ([]model.SSLCertMonitor, error) {
	var monitors []model.SSLCertMonitor
	if err := r.db.Where("enabled = ?", true).Find(&monitors).Error; err != nil {
		return nil, err
	}
	return monitors, nil
}

// Update 更新 SSL 证书监控
func (r *SSLCertMonitorRepository) Update(monitor *model.SSLCertMonitor) error {
	return r.db.Save(monitor).Error
}

// UpdateEnabled 使用 Select 强制更新 enabled 字段，避免 GORM default tag 导致零值被忽略
func (r *SSLCertMonitorRepository) UpdateEnabled(monitor *model.SSLCertMonitor, enabled bool) error {
	return r.db.Model(monitor).Select("enabled").Update("enabled", enabled).Error
}

// Delete 删除 SSL 证书监控
func (r *SSLCertMonitorRepository) Delete(id int64) error {
	return r.db.Delete(&model.SSLCertMonitor{}, id).Error
}
