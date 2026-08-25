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

// GetByFingerprintTx 在事务内根据主机指纹获取 Agent（用于注册流程避免 TOCTOU 竞态）
func (r *AgentRepository) GetByFingerprintTx(tx *gorm.DB, fingerprint string) (*model.Agent, error) {
	var agent model.Agent
	if err := tx.Where("host_fingerprint = ?", fingerprint).First(&agent).Error; err != nil {
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

// UpdateDisplayName 仅更新显示名称，避免 Save 覆盖 Online/LastSeen 等字段导致竞态
func (r *AgentRepository) UpdateDisplayName(id int64, displayName string) error {
	return r.db.Model(&model.Agent{}).
		Where("id = ?", id).
		Update("display_name", displayName).Error
}

// UpdateTags 仅更新标签字段（P1-10: 服务器分组）
func (r *AgentRepository) UpdateTags(id int64, tags string) error {
	return r.db.Model(&model.Agent{}).
		Where("id = ?", id).
		Update("tags", tags).Error
}

// UpdateProfile 原子更新显示名称与标签（单条 SQL，避免部分更新）
func (r *AgentRepository) UpdateProfile(id int64, displayName, tags string) error {
	return r.db.Model(&model.Agent{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"display_name": displayName,
			"tags":         tags,
		}).Error
}

// AgentMeta NodeGet 风格元数据（全部由管理员设置）
type AgentMeta struct {
	Region            string
	CountryCode       string
	ISP               string
	ExpiresAt         *time.Time
	PriceAmount       float64
	PriceCurrency     string
	PriceCycle        string
	TrafficQuotaBytes int64
}

// UpdateMeta 原子更新 NodeGet 风格元数据（单条 SQL，避免部分更新）
func (r *AgentRepository) UpdateMeta(id int64, meta AgentMeta) error {
	return r.db.Model(&model.Agent{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"region":              meta.Region,
			"country_code":        meta.CountryCode,
			"isp":                 meta.ISP,
			"expires_at":          meta.ExpiresAt,
			"price_amount":        meta.PriceAmount,
			"price_currency":      meta.PriceCurrency,
			"price_cycle":         meta.PriceCycle,
			"traffic_quota_bytes": meta.TrafficQuotaBytes,
		}).Error
}

// UpdateExitIP 更新出口 IPv4/IPv6（仅值变化时调用，避免高频写库）
func (r *AgentRepository) UpdateExitIP(id int64, ipv4, ipv6 string) error {
	return r.db.Model(&model.Agent{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"ipv4": ipv4,
			"ipv6": ipv6,
		}).Error
}

// UpdateStaticInfo 更新 Agent 静态信息（OS、Arch、Kernel、Hostname、AgentVersion、Virtualization、Distro）
// 仅更新静态字段，不触碰 Online/LastSeen/Token 等动态字段，避免竞态
func (r *AgentRepository) UpdateStaticInfo(id int64, os, arch, kernel, hostname, agentVersion, virtualization, distro string) error {
	return r.db.Model(&model.Agent{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"os":             os,
			"arch":           arch,
			"kernel":         kernel,
			"hostname":       hostname,
			"agent_version":  agentVersion,
			"virtualization": virtualization,
			"distro":         distro,
		}).Error
}

// Delete 删除 Agent
func (r *AgentRepository) Delete(id int64) error {
	return r.db.Delete(&model.Agent{}, id).Error
}

// DeleteWithRecordsTx 在事务内同时删除 Agent 的历史聚合数据和 Agent 记录
// 确保删除操作的原子性，避免部分失败导致数据不一致
func (r *AgentRepository) DeleteWithRecordsTx(agentID int64) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// 先删除关联的历史聚合数据 (metric_records)
		if err := tx.Where("agent_id = ?", agentID).Delete(&model.MetricRecord{}).Error; err != nil {
			return err
		}
		// 删除关联的流量统计记录 (traffic_records)
		if err := tx.Where("agent_id = ?", agentID).Delete(&model.TrafficRecord{}).Error; err != nil {
			return err
		}
		// 清理 register_codes 表中 used_by_agent_id 的悬空引用，避免外键悬空
		if err := tx.Model(&model.RegisterCode{}).
			Where("used_by_agent_id = ?", agentID).
			Update("used_by_agent_id", 0).Error; err != nil {
			return err
		}
		// 再删除 Agent 记录
		if err := tx.Delete(&model.Agent{}, agentID).Error; err != nil {
			return err
		}
		return nil
	})
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
