package service

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"time"

	"github.com/server-probe/server/internal/model"
	"github.com/server-probe/server/internal/repository"
	"gorm.io/gorm"
)

// AgentRegistryService Agent 注册管理服务
type AgentRegistryService struct {
	agentRepo      *repository.AgentRepository
	registerRepo   *repository.RegisterCodeRepository
	db             *gorm.DB
	maxUnusedCodes int
}

// NewAgentRegistryService 创建 Agent 注册服务
func NewAgentRegistryService(agentRepo *repository.AgentRepository, registerRepo *repository.RegisterCodeRepository, db *gorm.DB) *AgentRegistryService {
	return &AgentRegistryService{
		agentRepo:      agentRepo,
		registerRepo:   registerRepo,
		db:             db,
		maxUnusedCodes: 5,
	}
}

// GenerateRegisterCode 生成注册码
func (s *AgentRegistryService) GenerateRegisterCode(displayName, remark string) (*model.RegisterCode, error) {
	// 检查未使用注册码数量
	count, err := s.registerRepo.CountUnused()
	if err != nil {
		return nil, fmt.Errorf("检查注册码数量失败: %w", err)
	}
	if count >= int64(s.maxUnusedCodes) {
		return nil, fmt.Errorf("未使用注册码已达上限(%d)", s.maxUnusedCodes)
	}

	// 生成 8 位随机注册码
	code, err := generateRandomCode(8)
	if err != nil {
		return nil, fmt.Errorf("生成注册码失败: %w", err)
	}

	rc := &model.RegisterCode{
		Code:        code,
		DisplayName: displayName,
		Remark:      remark,
		ExpiresAt:   time.Now().Add(15 * time.Minute),
	}

	if err := s.registerRepo.Create(rc); err != nil {
		return nil, fmt.Errorf("保存注册码失败: %w", err)
	}

	log.Printf("生成注册码: %s, 名称: %s, 有效期 15 分钟", code, displayName)
	return rc, nil
}

// RegisterAgent 注册 Agent
// 使用数据库事务解决注册码竞态条件: 在事务内先原子标记注册码已使用 (WHERE used = false)，
// 成功后再创建/更新 Agent，确保并发请求不会重复使用同一注册码
func (s *AgentRegistryService) RegisterAgent(req RegisterAgentRequest) (*RegisterAgentResult, error) {
	// 先在事务外验证注册码是否存在并获取信息（用于过期检查和显示名称）
	rc, err := s.registerRepo.GetByCode(req.Code)
	if err != nil {
		return nil, fmt.Errorf("注册码不存在")
	}

	// 检查是否过期
	if time.Now().After(rc.ExpiresAt) {
		return nil, fmt.Errorf("注册码已过期")
	}

	// 检查主机指纹是否已注册（同一台机器重新注册）
	existingAgent, fpErr := s.agentRepo.GetByFingerprint(req.HostFingerprint)

	// 生成持久 Token
	token, err := generateRandomToken(32)
	if err != nil {
		return nil, fmt.Errorf("生成 Token 失败: %w", err)
	}

	// 使用事务原子标记注册码已使用并创建/更新 Agent
	var result *RegisterAgentResult
	txErr := s.db.Transaction(func(tx *gorm.DB) error {
		// 在事务内原子标记注册码已使用 (WHERE used = false)
		// 并发请求中只有一个能成功通过此检查
		if err := s.registerRepo.MarkUsedTx(tx, req.Code, 0); err != nil {
			return fmt.Errorf("注册码已被使用")
		}

		if fpErr == nil && existingAgent != nil {
			// 同一台机器重新注册，更新 Token
			existingAgent.Token = token
			existingAgent.Hostname = req.Hostname
			existingAgent.OS = req.OS
			existingAgent.Arch = req.Arch
			existingAgent.AgentVersion = req.AgentVersion
			existingAgent.Online = false
			// 如果注册码有显示名称且 Agent 没有显示名称，则设置
			if rc.DisplayName != "" && existingAgent.DisplayName == "" {
				existingAgent.DisplayName = rc.DisplayName
			}
			if err := s.agentRepo.UpdateTx(tx, existingAgent); err != nil {
				return fmt.Errorf("更新 Agent 失败: %w", err)
			}

			// 回填注册码的 used_by_agent_id
			_ = s.registerRepo.UpdateUsedByAgentIDTx(tx, req.Code, existingAgent.ID)

			result = &RegisterAgentResult{
				AgentID: existingAgent.ID,
				Token:   token,
			}
			return nil
		}

		// 创建新 Agent
		agent := &model.Agent{
			Token:            token,
			Hostname:         req.Hostname,
			DisplayName:      rc.DisplayName, // 使用注册码中的显示名称
			OS:               req.OS,
			Arch:             req.Arch,
			AgentVersion:     req.AgentVersion,
			HostFingerprint:  req.HostFingerprint,
			Online:           false,
		}

		if err := s.agentRepo.CreateTx(tx, agent); err != nil {
			return fmt.Errorf("创建 Agent 失败: %w", err)
		}

		// 回填注册码的 used_by_agent_id
		_ = s.registerRepo.UpdateUsedByAgentIDTx(tx, req.Code, agent.ID)

		log.Printf("Agent 注册成功: ID=%d, Hostname=%s", agent.ID, agent.Hostname)

		result = &RegisterAgentResult{
			AgentID: agent.ID,
			Token:   token,
		}
		return nil
	})

	if txErr != nil {
		return nil, txErr
	}

	return result, nil
}

// ValidateToken 验证 Agent Token
func (s *AgentRegistryService) ValidateToken(token string) (*model.Agent, error) {
	return s.agentRepo.GetByToken(token)
}

// ListRegisterCodes 列出所有注册码
func (s *AgentRegistryService) ListRegisterCodes() ([]model.RegisterCode, error) {
	return s.registerRepo.ListUnused()
}

// DeleteRegisterCode 删除注册码
func (s *AgentRegistryService) DeleteRegisterCode(code string) error {
	return s.registerRepo.Delete(code)
}

// CleanupExpiredCodes 清理过期注册码
func (s *AgentRegistryService) CleanupExpiredCodes() error {
	return s.registerRepo.DeleteExpired()
}

// RegisterAgentRequest 注册请求
type RegisterAgentRequest struct {
	Code            string `json:"code"`
	Hostname        string `json:"hostname"`
	OS              string `json:"os"`
	Arch            string `json:"arch"`
	AgentVersion    string `json:"agent_version"`
	HostFingerprint string `json:"host_fingerprint"`
}

// RegisterAgentResult 注册结果
type RegisterAgentResult struct {
	AgentID int64  `json:"agent_id"`
	Token   string `json:"token"`
}

// generateRandomCode 生成随机注册码（大写字母+数字）
func generateRandomCode(length int) (string, error) {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	for i := range bytes {
		bytes[i] = charset[bytes[i]%byte(len(charset))]
	}
	return string(bytes), nil
}

// generateRandomToken 生成随机 Token（十六进制）
func generateRandomToken(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
