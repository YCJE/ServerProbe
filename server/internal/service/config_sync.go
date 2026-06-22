package service

import (
	"strconv"

	"github.com/server-probe/server/internal/model"
	"github.com/server-probe/server/internal/repository"
	sharedmodel "github.com/server-probe/shared/model"
	"gorm.io/gorm"
)

// ConfigSyncService 配置同步服务
type ConfigSyncService struct {
	pingTargetRepo *repository.PingTargetRepository
	db             *gorm.DB
}

// NewConfigSyncService 创建配置同步服务
func NewConfigSyncService(pingTargetRepo *repository.PingTargetRepository, db *gorm.DB) *ConfigSyncService {
	return &ConfigSyncService{
		pingTargetRepo: pingTargetRepo,
		db:             db,
	}
}

// GetPingInterval 从数据库获取 Ping 探测间隔 (默认 60 秒)
func (s *ConfigSyncService) GetPingInterval() int {
	if s.db == nil {
		return 60
	}
	var setting model.SystemSetting
	if err := s.db.Where("key = ?", "ping_interval").First(&setting).Error; err != nil {
		return 60 // 默认 60 秒
	}
	interval, err := strconv.Atoi(setting.Value)
	if err != nil || interval < 1 || interval > 3600 {
		return 60
	}
	return interval
}

// SetPingInterval 设置 Ping 探测间隔
func (s *ConfigSyncService) SetPingInterval(interval int) error {
	if interval < 1 || interval > 3600 {
		interval = 60
	}
	setting := model.SystemSetting{
		Key:   "ping_interval",
		Value: strconv.Itoa(interval),
	}
	// 使用 FirstOrCreate 实现 upsert (Save 在记录不存在时无法首次创建)
	return s.db.Where("key = ?", "ping_interval").
		Assign(model.SystemSetting{Value: strconv.Itoa(interval)}).
		FirstOrCreate(&setting).Error
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
			Method:  t.Method,
			Enabled: t.Enabled,
		})
	}

	return &sharedmodel.AgentConfig{
		PingTargets:  pingTargets,
		PingInterval: s.GetPingInterval(),
	}, nil
}
