package repository

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	"gorm.io/gorm"

	"github.com/server-probe/server/internal/model"
)

// SessionRepository 管理员登录会话 CRUD（P2：会话管理）
type SessionRepository struct {
	db *gorm.DB
}

// NewSessionRepository 创建会话 repository
func NewSessionRepository(db *gorm.DB) *SessionRepository {
	return &SessionRepository{db: db}
}

// GenerateSessionID 生成会话 ID（32 字节随机数 hex 编码，与 JWT jti 一致）
func GenerateSessionID() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// Create 写入一条会话记录
func (r *SessionRepository) Create(s *model.Session) error {
	return r.db.Create(s).Error
}

// GetBySessionID 按 SessionID 查询（用于 AuthRequired 校验，不区分撤销状态）
func (r *SessionRepository) GetBySessionID(sessionID string) (*model.Session, error) {
	var s model.Session
	if err := r.db.Where("session_id = ?", sessionID).First(&s).Error; err != nil {
		return nil, err
	}
	return &s, nil
}

// Touch 更新会话最后活跃时间（节流由调用方负责）
func (r *SessionRepository) Touch(s *model.Session) error {
	return r.db.Model(&model.Session{}).Where("id = ?", s.ID).Update("last_seen_at", time.Now()).Error
}

// Revoke 撤销指定会话（已撤销时幂等）
func (r *SessionRepository) Revoke(sessionID string) error {
	return r.db.Model(&model.Session{}).
		Where("session_id = ? AND revoked_at IS NULL", sessionID).
		Update("revoked_at", time.Now()).Error
}

// RevokeAllOther 撤销某管理员除指定会话外的全部会话
func (r *SessionRepository) RevokeAllOther(adminID int64, keepSessionID string) (int64, error) {
	result := r.db.Model(&model.Session{}).
		Where("admin_id = ? AND session_id != ? AND revoked_at IS NULL", adminID, keepSessionID).
		Update("revoked_at", time.Now())
	return result.RowsAffected, result.Error
}

// ListByAdmin 查询某管理员的全部会话（未过期且 7 天内撤销/过期的，按创建时间倒序）
func (r *SessionRepository) ListByAdmin(adminID int64) ([]model.Session, error) {
	cutoff := time.Now().Add(-7 * 24 * time.Hour)
	var sessions []model.Session
	err := r.db.Where("admin_id = ? AND (revoked_at IS NULL OR revoked_at > ?) AND expires_at > ?",
		adminID, cutoff, cutoff).
		Order("created_at DESC").
		Find(&sessions).Error
	return sessions, err
}

// CleanupExpired 清理过期或已撤销超过 7 天的会话，返回删除行数
func (r *SessionRepository) CleanupExpired() (int64, error) {
	cutoff := time.Now().Add(-7 * 24 * time.Hour)
	result := r.db.Where(
		"(revoked_at IS NOT NULL AND revoked_at < ?) OR expires_at < ?",
		cutoff, cutoff,
	).Delete(&model.Session{})
	return result.RowsAffected, result.Error
}
