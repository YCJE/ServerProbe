package service

import (
	"github.com/server-probe/server/internal/repository"
	sharedmodel "github.com/server-probe/shared/model"
)

// ConfigSyncService 配置同步服务
type ConfigSyncService struct {
	pingTargetRepo *repository.PingTargetRepository
}

// NewConfigSyncService 创建配置同步服务
func NewConfigSyncService(pingTargetRepo *repository.PingTargetRepository) *ConfigSyncService {
	return &ConfigSyncService{
		pingTargetRepo: pingTargetRepo,
	}
}

// GetAgentConfig 获取 Agent 配置（探测目标）
func (s *ConfigSyncService) GetAgentConfig() (*sharedmodel.AgentConfig, error) {
	targets, err := s.pingTargetRepo.ListEnabled()
	if err != nil {
		return nil, err
	}

	pingTargets := make([]sharedmodel.PingTarget, 0, len(targets))
	for _, t := range targets {
		pingTargets = append(pingTargets, sharedmodel.PingTarget{
			ID:      t.ID,
			Target:  t.Target,
			Name:    t.Name,
			Enabled: t.Enabled,
		})
	}

	return &sharedmodel.AgentConfig{
		PingTargets:  pingTargets,
		PingInterval: 60, // 默认 60 秒
	}, nil
}
