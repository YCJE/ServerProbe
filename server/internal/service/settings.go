package service

import (
	"log"
	"strconv"
	"sync"

	"github.com/server-probe/server/internal/repository"
)

// 系统设置键定义（持久化于 system_settings 表，后台「站点设置」页可修改）
const (
	SettingSiteTitle            = "site_title"             // 站点标题
	SettingSiteDescription      = "site_description"       // 站点描述
	SettingAnnouncement         = "announcement"           // 公告（支持换行，公开页与管理端展示）
	SettingCustomFooter         = "custom_footer"          // 自定义页脚文本
	SettingDefaultHistoryRange  = "default_history_range"  // 详情页默认历史范围
	SettingOfflineGraceSeconds  = "offline_grace_seconds"  // 离线判定宽限期（秒）
	SettingRetentionDays        = "retention_days"         // 历史数据保留天数（5 分钟层）
	SettingRetentionDaysHourly  = "retention_days_hourly"  // 历史数据保留天数（小时聚合层）
	SettingMaxChartPoints       = "max_chart_points"       // 图表单次加载最大数据点数
	// 到期提前通知（P2：每日汇总一条通知，防重发日期记录见 backup.go SettingExpireLastSent）
	SettingExpireNotifyEnabled   = "expire_notify_enabled"   // 是否启用到期提醒
	SettingExpireNotifyLeadDays  = "expire_notify_lead_days"  // 提前天数（剩余天数 ≤ 此值时提醒）
	SettingExpireNotifyChannelID = "expire_notify_channel_id" // 通知渠道 ID（0=未配置，不发送）
)

// 默认值常量
const (
	DefaultSiteTitle           = "Server Probe"
	DefaultSiteDescription     = "安全优先、只读架构的服务器监控探针系统"
	DefaultHistoryRange        = "1h"
	DefaultOfflineGraceSeconds = 90
	DefaultRetentionDays       = 30
    DefaultRetentionDaysHourly = 730
	DefaultMaxChartPoints      = 800

	DefaultExpireNotifyEnabled   = false
	DefaultExpireNotifyLeadDays  = 7
	DefaultExpireNotifyChannelID = 0

	minExpireNotifyLeadDays = 1
	maxExpireNotifyLeadDays = 90

	minOfflineGraceSeconds = 30
	maxOfflineGraceSeconds = 86400
	minRetentionDays       = 1
	maxRetentionDays       = 3650
	minRetentionDaysHourly = 30
	maxRetentionDaysHourly = 3650
	minMaxChartPoints      = 100
	maxMaxChartPoints      = 2000
)

// validHistoryRanges 详情页支持的历史范围
var validHistoryRanges = map[string]bool{
	"1h": true, "6h": true, "12h": true, "1d": true, "2d": true, "3d": true,
	"7d": true, "30d": true, "90d": true, "1y": true,
}

// IsValidHistoryRange 判断历史范围值是否有效
func IsValidHistoryRange(v string) bool { return validHistoryRanges[v] }

// SettingsChangeCallback 设置变更回调（运行时应用：心跳宽限期等）
type SettingsChangeCallback func(s *SettingsService)

// SettingsService 系统设置服务（内存缓存 + 落盘）
type SettingsService struct {
	repo      *repository.SettingRepository
	mu        sync.RWMutex
	cache     map[string]string
	callbacks []SettingsChangeCallback
}

// NewSettingsService 创建设置服务并加载缓存（repo 为 nil 时使用空缓存，全部走默认值）
func NewSettingsService(repo *repository.SettingRepository) (*SettingsService, error) {
	s := &SettingsService{repo: repo, cache: map[string]string{}}
	if repo == nil {
		log.Println("系统设置服务初始化（无仓储，使用默认值）")
		return s, nil
	}
	kv, err := repo.GetAll()
	if err != nil {
		return nil, err
	}
	s.cache = kv
	log.Printf("系统设置已加载（%d 项）", len(kv))
	return s, nil
}

// OnChange 注册设置变更回调
func (s *SettingsService) OnChange(cb SettingsChangeCallback) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.callbacks = append(s.callbacks, cb)
}

// notifyLocked 通知所有回调（调用方需持有写锁之外调用）
func (s *SettingsService) notify() {
	var callbacks []SettingsChangeCallback
	s.mu.RLock()
	callbacks = append(callbacks, s.callbacks...)
	s.mu.RUnlock()
	for _, cb := range callbacks {
		cb(s)
	}
}

// GetString 读取字符串设置（不存在返回默认值）
func (s *SettingsService) GetString(key, def string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if v, ok := s.cache[key]; ok && v != "" {
		return v
	}
	return def
}

// GetInt 读取整数设置（无效或不存在返回默认值）
func (s *SettingsService) GetInt(key string, def, min, max int) int {
	s.mu.RLock()
	raw, ok := s.cache[key]
	s.mu.RUnlock()
	if !ok || raw == "" {
		return def
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// GetBool 读取布尔设置（"true"/"1" 视为 true，其余 false；不存在返回默认值）
func (s *SettingsService) GetBool(key string, def bool) bool {
	s.mu.RLock()
	raw, ok := s.cache[key]
	s.mu.RUnlock()
	if !ok || raw == "" {
		return def
	}
	return raw == "true" || raw == "1"
}

// SiteTitle 站点标题
func (s *SettingsService) SiteTitle() string { return s.GetString(SettingSiteTitle, DefaultSiteTitle) }

// SiteDescription 站点描述
func (s *SettingsService) SiteDescription() string { return s.GetString(SettingSiteDescription, DefaultSiteDescription) }

// Announcement 公告
func (s *SettingsService) Announcement() string { return s.GetString(SettingAnnouncement, "") }

// CustomFooter 自定义页脚
func (s *SettingsService) CustomFooter() string { return s.GetString(SettingCustomFooter, "") }

// DefaultHistoryRange 详情页默认历史范围
func (s *SettingsService) DefaultHistoryRange() string {
	v := s.GetString(SettingDefaultHistoryRange, DefaultHistoryRange)
	if !validHistoryRanges[v] {
		return DefaultHistoryRange
	}
	return v
}

// OfflineGraceSeconds 离线判定宽限期
func (s *SettingsService) OfflineGraceSeconds() int {
	return s.GetInt(SettingOfflineGraceSeconds, DefaultOfflineGraceSeconds, minOfflineGraceSeconds, maxOfflineGraceSeconds)
}

// RetentionDays 历史数据保留天数（5 分钟层）
func (s *SettingsService) RetentionDays() int {
	return s.GetInt(SettingRetentionDays, DefaultRetentionDays, minRetentionDays, maxRetentionDays)
}

// RetentionDaysHourly 历史数据保留天数（小时聚合层）
func (s *SettingsService) RetentionDaysHourly() int {
	return s.GetInt(SettingRetentionDaysHourly, DefaultRetentionDaysHourly, minRetentionDaysHourly, maxRetentionDaysHourly)
}

// MaxChartPoints 图表单次加载最大数据点数
func (s *SettingsService) MaxChartPoints() int {
	return s.GetInt(SettingMaxChartPoints, DefaultMaxChartPoints, minMaxChartPoints, maxMaxChartPoints)
}

// ExpireNotifyEnabled 到期提醒是否启用
func (s *SettingsService) ExpireNotifyEnabled() bool {
	return s.GetBool(SettingExpireNotifyEnabled, DefaultExpireNotifyEnabled)
}

// ExpireNotifyLeadDays 到期提醒提前天数
func (s *SettingsService) ExpireNotifyLeadDays() int {
	return s.GetInt(SettingExpireNotifyLeadDays, DefaultExpireNotifyLeadDays, minExpireNotifyLeadDays, maxExpireNotifyLeadDays)
}

// ExpireNotifyChannelID 到期提醒通知渠道 ID（0=未配置）
func (s *SettingsService) ExpireNotifyChannelID() int64 {
	return int64(s.GetInt(SettingExpireNotifyChannelID, DefaultExpireNotifyChannelID, 0, 1<<30))
}

// AllSettings 导出全部设置（管理端展示）
func (s *SettingsService) AllSettings() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]string, len(s.cache))
	for k, v := range s.cache {
		out[k] = v
	}
	return out
}

// Update 批量更新设置并落盘，随后通知运行时回调
func (s *SettingsService) Update(kv map[string]string) error {
	if len(kv) == 0 {
		return nil
	}
	if s.repo != nil {
		if err := s.repo.SetBatch(kv); err != nil {
			return err
		}
	}
	s.mu.Lock()
	for k, v := range kv {
		s.cache[k] = v
	}
	s.mu.Unlock()
	s.notify()
	log.Printf("系统设置已更新（%d 项）", len(kv))
	return nil
}
