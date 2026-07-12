package repository

import (
	"gorm.io/gorm"

	"github.com/server-probe/server/internal/model"
)

// ServiceMonitorRepository 服务监控 CRUD
type ServiceMonitorRepository struct {
	db *gorm.DB
}

// NewServiceMonitorRepository 创建服务监控 repository
func NewServiceMonitorRepository(db *gorm.DB) *ServiceMonitorRepository {
	return &ServiceMonitorRepository{db: db}
}

// Create 创建服务监控
func (r *ServiceMonitorRepository) Create(monitor *model.ServiceMonitor) error {
	return r.db.Create(monitor).Error
}

// GetByID 根据 ID 获取服务监控
func (r *ServiceMonitorRepository) GetByID(id int64) (*model.ServiceMonitor, error) {
	var monitor model.ServiceMonitor
	if err := r.db.First(&monitor, id).Error; err != nil {
		return nil, err
	}
	return &monitor, nil
}

// List 获取所有服务监控
func (r *ServiceMonitorRepository) List() ([]model.ServiceMonitor, error) {
	var monitors []model.ServiceMonitor
	if err := r.db.Order("id ASC").Find(&monitors).Error; err != nil {
		return nil, err
	}
	return monitors, nil
}

// ListEnabled 获取已启用的服务监控
func (r *ServiceMonitorRepository) ListEnabled() ([]model.ServiceMonitor, error) {
	var monitors []model.ServiceMonitor
	if err := r.db.Where("enabled = ?", true).Find(&monitors).Error; err != nil {
		return nil, err
	}
	return monitors, nil
}

// Update 更新服务监控
func (r *ServiceMonitorRepository) Update(monitor *model.ServiceMonitor) error {
	return r.db.Save(monitor).Error
}

// UpdateEnabled 使用 Select 强制更新 enabled 字段，避免 GORM default tag 导致零值被忽略
func (r *ServiceMonitorRepository) UpdateEnabled(monitor *model.ServiceMonitor, enabled bool) error {
	return r.db.Model(monitor).Select("enabled").Update("enabled", enabled).Error
}

// Delete 删除服务监控
func (r *ServiceMonitorRepository) Delete(id int64) error {
	return r.db.Delete(&model.ServiceMonitor{}, id).Error
}
