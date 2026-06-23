package repository

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/server-probe/server/internal/model"
)

// RecordRepository 历史聚合数据 CRUD
type RecordRepository struct {
	db *gorm.DB
}

// NewRecordRepository 创建历史数据 repository
func NewRecordRepository(db *gorm.DB) *RecordRepository {
	return &RecordRepository{db: db}
}

// Create 创建历史记录
func (r *RecordRepository) Create(record *model.MetricRecord) error {
	return r.db.Create(record).Error
}

// CreateBatch 批量创建历史记录
func (r *RecordRepository) CreateBatch(records []model.MetricRecord) error {
	if len(records) == 0 {
		return nil
	}
	return r.db.CreateInBatches(records, 100).Error
}

// GetByAgentAndTimeRange 根据 Agent ID 和时间范围查询历史数据
func (r *RecordRepository) GetByAgentAndTimeRange(agentID int64, startTime, endTime int64) ([]model.MetricRecord, error) {
	var records []model.MetricRecord
	err := r.db.Where("agent_id = ? AND timestamp >= ? AND timestamp <= ?", agentID, startTime, endTime).
		Order("timestamp ASC").
		Find(&records).Error
	if err != nil {
		return nil, err
	}
	return records, nil
}

// DeleteOlderThan 删除指定时间之前的数据
func (r *RecordRepository) DeleteOlderThan(before int64) (int64, error) {
	result := r.db.Where("timestamp < ?", before).Delete(&model.MetricRecord{})
	return result.RowsAffected, result.Error
}

// CleanupExpired 清理过期数据（默认保留 2 天）
func (r *RecordRepository) CleanupExpired(retentionDays int) (int64, error) {
	if retentionDays <= 0 {
		retentionDays = 2 // 默认保留 2 天
	}
	cutoff := time.Now().AddDate(0, 0, -retentionDays).Unix()
	deleted, err := r.DeleteOlderThan(cutoff)
	if err != nil {
		return 0, fmt.Errorf("清理过期数据失败: %w", err)
	}
	return deleted, nil
}

// GetDBSize 获取数据库文件大小 (字节)
func (r *RecordRepository) GetDBSize() int64 {
	var page_count int64
	r.db.Raw("PRAGMA page_count").Scan(&page_count)
	var page_size int64
	r.db.Raw("PRAGMA page_size").Scan(&page_size)
	return page_count * page_size
}

// AdminRepository 管理员账户 CRUD
type AdminRepository struct {
	db *gorm.DB
}

// NewAdminRepository 创建管理员 repository
func NewAdminRepository(db *gorm.DB) *AdminRepository {
	return &AdminRepository{db: db}
}

// Create 创建管理员
func (r *AdminRepository) Create(admin *model.Admin) error {
	return r.db.Create(admin).Error
}

// GetByUsername 根据用户名获取管理员
func (r *AdminRepository) GetByUsername(username string) (*model.Admin, error) {
	var admin model.Admin
	if err := r.db.Where("username = ?", username).First(&admin).Error; err != nil {
		return nil, err
	}
	return &admin, nil
}

// GetByID 根据 ID 获取管理员
func (r *AdminRepository) GetByID(id int64) (*model.Admin, error) {
	var admin model.Admin
	if err := r.db.First(&admin, id).Error; err != nil {
		return nil, err
	}
	return &admin, nil
}

// Update 更新管理员
func (r *AdminRepository) Update(admin *model.Admin) error {
	return r.db.Save(admin).Error
}

// Count 统计管理员数量
func (r *AdminRepository) Count() (int64, error) {
	var count int64
	err := r.db.Model(&model.Admin{}).Count(&count).Error
	return count, err
}

// SharePageRepository 分享页 CRUD
type SharePageRepository struct {
	db *gorm.DB
}

// NewSharePageRepository 创建分享页 repository
func NewSharePageRepository(db *gorm.DB) *SharePageRepository {
	return &SharePageRepository{db: db}
}

// Create 创建分享页
func (r *SharePageRepository) Create(page *model.SharePage) error {
	return r.db.Create(page).Error
}

// GetByShareID 根据 share_id 获取分享页
func (r *SharePageRepository) GetByShareID(shareID string) (*model.SharePage, error) {
	var page model.SharePage
	if err := r.db.Where("share_id = ?", shareID).First(&page).Error; err != nil {
		return nil, err
	}
	return &page, nil
}

// List 获取所有分享页
func (r *SharePageRepository) List() ([]model.SharePage, error) {
	var pages []model.SharePage
	if err := r.db.Order("id ASC").Find(&pages).Error; err != nil {
		return nil, err
	}
	return pages, nil
}

// Delete 删除分享页
func (r *SharePageRepository) Delete(id int64) error {
	return r.db.Delete(&model.SharePage{}, id).Error
}
