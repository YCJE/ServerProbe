package repository

import (
	"sync"

	"github.com/server-probe/server/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SettingRepository 系统设置仓储（键值对存储）
type SettingRepository struct {
	db *gorm.DB
	mu sync.RWMutex
}

// NewSettingRepository 创建设置仓储
func NewSettingRepository(db *gorm.DB) *SettingRepository {
	return &SettingRepository{db: db}
}

// GetAll 读取全部设置项
func (r *SettingRepository) GetAll() (map[string]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var rows []model.SystemSetting
	if err := r.db.Find(&rows).Error; err != nil {
		return nil, err
	}
	m := make(map[string]string, len(rows))
	for _, row := range rows {
		m[row.Key] = row.Value
	}
	return m, nil
}

// SetBatch 批量写入设置项（存在则更新）
func (r *SettingRepository) SetBatch(kv map[string]string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(kv) == 0 {
		return nil
	}
	rows := make([]model.SystemSetting, 0, len(kv))
	for k, v := range kv {
		rows = append(rows, model.SystemSetting{Key: k, Value: v})
	}
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value"}),
	}).Create(&rows).Error
}
