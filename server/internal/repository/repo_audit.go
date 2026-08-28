package repository

import (
	"time"

	"gorm.io/gorm"

	"github.com/server-probe/server/internal/model"
)

// AuditLogRepository 审计日志 CRUD
type AuditLogRepository struct {
	db *gorm.DB
}

// NewAuditLogRepository 创建审计日志 repository
func NewAuditLogRepository(db *gorm.DB) *AuditLogRepository {
	return &AuditLogRepository{db: db}
}

// Create 写入一条审计日志
func (r *AuditLogRepository) Create(entry *model.AuditLog) error {
	return r.db.Create(entry).Error
}

// AuditLogQuery 审计日志查询条件
type AuditLogQuery struct {
	Username string // 精确匹配用户名（空=不过滤）
	Action   string // 模糊匹配操作（空=不过滤）
	Success  *bool  // 成功/失败过滤（nil=不过滤）
	Page     int    // 从 1 开始
	PageSize int
}

// List 分页查询审计日志（按时间倒序），返回记录总数
func (r *AuditLogRepository) List(q AuditLogQuery) ([]model.AuditLog, int64, error) {
	tx := r.db.Model(&model.AuditLog{})
	if q.Username != "" {
		tx = tx.Where("username = ?", q.Username)
	}
	if q.Action != "" {
		tx = tx.Where("action LIKE ?", "%"+q.Action+"%")
	}
	if q.Success != nil {
		tx = tx.Where("success = ?", *q.Success)
	}

	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize < 1 || q.PageSize > 200 {
		q.PageSize = 50
	}

	var logs []model.AuditLog
	err := tx.Order("id DESC").
		Offset((q.Page - 1) * q.PageSize).
		Limit(q.PageSize).
		Find(&logs).Error
	return logs, total, err
}

// CleanupOlderThan 删除指定时间之前的审计日志，返回删除行数
func (r *AuditLogRepository) CleanupOlderThan(before time.Time) (int64, error) {
	result := r.db.Where("created_at < ?", before).Delete(&model.AuditLog{})
	return result.RowsAffected, result.Error
}
