package repository

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/server-probe/server/internal/model"
)

// AgentRepository Agent 元数据 CRUD
type AgentRepository struct {
	db *gorm.DB
}

// NewAgentRepository 创建 Agent repository
func NewAgentRepository(db *gorm.DB) *AgentRepository {
	return &AgentRepository{db: db}
}

// Create 创建 Agent
func (r *AgentRepository) Create(agent *model.Agent) error {
	return r.db.Create(agent).Error
}

// CreateTx 在事务内创建 Agent
func (r *AgentRepository) CreateTx(tx *gorm.DB, agent *model.Agent) error {
	return tx.Create(agent).Error
}

// UpdateTx 在事务内更新 Agent 信息
func (r *AgentRepository) UpdateTx(tx *gorm.DB, agent *model.Agent) error {
	return tx.Save(agent).Error
}

// GetByID 根据 ID 获取 Agent
func (r *AgentRepository) GetByID(id int64) (*model.Agent, error) {
	var agent model.Agent
	if err := r.db.First(&agent, id).Error; err != nil {
		return nil, err
	}
	return &agent, nil
}

// GetByToken 根据 Token 获取 Agent
func (r *AgentRepository) GetByToken(token string) (*model.Agent, error) {
	var agent model.Agent
	if err := r.db.Where("token = ?", token).First(&agent).Error; err != nil {
		return nil, err
	}
	return &agent, nil
}

// GetByFingerprint 根据主机指纹获取 Agent
func (r *AgentRepository) GetByFingerprint(fingerprint string) (*model.Agent, error) {
	var agent model.Agent
	if err := r.db.Where("host_fingerprint = ?", fingerprint).First(&agent).Error; err != nil {
		return nil, err
	}
	return &agent, nil
}

// List 获取所有 Agent
func (r *AgentRepository) List() ([]model.Agent, error) {
	var agents []model.Agent
	if err := r.db.Order("id ASC").Find(&agents).Error; err != nil {
		return nil, err
	}
	return agents, nil
}

// ListOnline 获取在线 Agent
func (r *AgentRepository) ListOnline() ([]model.Agent, error) {
	var agents []model.Agent
	if err := r.db.Where("online = ?", true).Find(&agents).Error; err != nil {
		return nil, err
	}
	return agents, nil
}

// UpdateLastSeen 更新最后在线时间
func (r *AgentRepository) UpdateLastSeen(id int64, online bool) error {
	return r.db.Model(&model.Agent{}).Where("id = ?", id).
		Updates(map[string]interface{}{
			"last_seen": time.Now(),
			"online":    online,
		}).Error
}

// UpdateOnlineStatus 更新在线状态
func (r *AgentRepository) UpdateOnlineStatus(id int64, online bool) error {
	return r.db.Model(&model.Agent{}).Where("id = ?", id).
		Update("online", online).Error
}

// Update 更新 Agent 信息
func (r *AgentRepository) Update(agent *model.Agent) error {
	return r.db.Save(agent).Error
}

// Delete 删除 Agent
func (r *AgentRepository) Delete(id int64) error {
	return r.db.Delete(&model.Agent{}, id).Error
}

// RegisterCodeRepository 注册码 CRUD
type RegisterCodeRepository struct {
	db *gorm.DB
}

// NewRegisterCodeRepository 创建注册码 repository
func NewRegisterCodeRepository(db *gorm.DB) *RegisterCodeRepository {
	return &RegisterCodeRepository{db: db}
}

// Create 创建注册码
func (r *RegisterCodeRepository) Create(code *model.RegisterCode) error {
	return r.db.Create(code).Error
}

// GetByCode 根据注册码获取
func (r *RegisterCodeRepository) GetByCode(code string) (*model.RegisterCode, error) {
	var rc model.RegisterCode
	if err := r.db.Where("code = ?", code).First(&rc).Error; err != nil {
		return nil, err
	}
	return &rc, nil
}

// ListUnused 列出未使用的注册码
func (r *RegisterCodeRepository) ListUnused() ([]model.RegisterCode, error) {
	var codes []model.RegisterCode
	if err := r.db.Where("used = ?", false).Find(&codes).Error; err != nil {
		return nil, err
	}
	return codes, nil
}

// CountUnused 统计未使用的注册码数量
func (r *RegisterCodeRepository) CountUnused() (int64, error) {
	var count int64
	err := r.db.Model(&model.RegisterCode{}).Where("used = ?", false).Count(&count).Error
	return count, err
}

// MarkUsed 标记注册码已使用
func (r *RegisterCodeRepository) MarkUsed(code string, agentID int64) error {
	result := r.db.Model(&model.RegisterCode{}).
		Where("code = ? AND used = ?", code, false).
		Updates(map[string]interface{}{
			"used":            true,
			"used_by_agent_id": agentID,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("注册码不存在或已被使用")
	}
	return nil
}

// MarkUsedTx 在事务内原子标记注册码已使用 (WHERE used = false)
// 成功返回 nil 表示当前请求赢得了竞争；返回错误表示注册码不存在或已被使用
func (r *RegisterCodeRepository) MarkUsedTx(tx *gorm.DB, code string, agentID int64) error {
	result := tx.Model(&model.RegisterCode{}).
		Where("code = ? AND used = ?", code, false).
		Updates(map[string]interface{}{
			"used":            true,
			"used_by_agent_id": agentID,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("注册码不存在或已被使用")
	}
	return nil
}

// UpdateUsedByAgentIDTx 在事务内更新注册码的 used_by_agent_id 字段
// 用于在创建 Agent 后回填 agent ID（注册码此时已标记为 used）
func (r *RegisterCodeRepository) UpdateUsedByAgentIDTx(tx *gorm.DB, code string, agentID int64) error {
	return tx.Model(&model.RegisterCode{}).
		Where("code = ?", code).
		Update("used_by_agent_id", agentID).Error
}

// GetByCodeTx 在事务内根据注册码获取
func (r *RegisterCodeRepository) GetByCodeTx(tx *gorm.DB, code string) (*model.RegisterCode, error) {
	var rc model.RegisterCode
	if err := tx.Where("code = ?", code).First(&rc).Error; err != nil {
		return nil, err
	}
	return &rc, nil
}

// DeleteExpired 删除过期的注册码
func (r *RegisterCodeRepository) DeleteExpired() error {
	return r.db.Where("expires_at < ? AND used = ?", time.Now(), false).
		Delete(&model.RegisterCode{}).Error
}

// Delete 删除注册码
func (r *RegisterCodeRepository) Delete(code string) error {
	return r.db.Where("code = ?", code).Delete(&model.RegisterCode{}).Error
}
