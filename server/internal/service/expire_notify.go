package service

import (
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/server-probe/server/internal/repository"
)

// 到期提醒每日检查时刻（本地时间 09:00，与服务进程时区一致）
const expireNotifyHour = 9

// NotificationSender 到期提醒所需的最小通知发送能力（NotifyService 天然满足，测试可注入假实现）
type NotificationSender interface {
	SendNotification(channelID int64, title, content string) error
}

// ExpireNotifyService 到期提前通知服务（P2：每日一条汇总通知）
// 每日 09:00 检查全部 Agent 的 expires_at，剩余天数 ≤ 提前天数（含已过期）的机器
// 汇总为一条通知发送到管理员配置的渠道；发送日期记入 settings，重启不重发
type ExpireNotifyService struct {
	agentRepo *repository.AgentRepository
	settings  *SettingsService
	sender    NotificationSender
	wg        sync.WaitGroup
	stop      chan struct{}
	once      sync.Once
}

// NewExpireNotifyService 创建到期提醒服务（依赖为 nil 时 Start 不生效，便于测试）
func NewExpireNotifyService(agentRepo *repository.AgentRepository, settings *SettingsService, sender NotificationSender) *ExpireNotifyService {
	return &ExpireNotifyService{
		agentRepo: agentRepo,
		settings:  settings,
		sender:    sender,
		stop:      make(chan struct{}),
	}
}

// Start 启动每日检查 goroutine
func (s *ExpireNotifyService) Start() {
	if s.agentRepo == nil || s.settings == nil || s.sender == nil {
		return
	}
	s.wg.Add(1)
	go s.loop()
	log.Printf("到期提醒服务已启动（每日 %02d:00 检查，提前 %d 天）", expireNotifyHour, s.settings.ExpireNotifyLeadDays())
}

// Stop 停止服务
func (s *ExpireNotifyService) Stop() {
	s.once.Do(func() { close(s.stop) })
	s.wg.Wait()
}

func (s *ExpireNotifyService) loop() {
	defer s.wg.Done()

	// 启动补发：服务在今日 09:00 之后启动且今日尚未发送时立即检查一次
	// （进程在 09:00 前重启的场景由 CheckAndNotify 的防重发记录兜底）
	if now := time.Now(); now.Hour() >= expireNotifyHour {
		s.CheckAndNotify()
	}

	for {
		timer := time.NewTimer(time.Until(nextExpireNotifyRun(time.Now())))
		select {
		case <-timer.C:
			s.CheckAndNotify()
		case <-s.stop:
			timer.Stop()
			return
		}
	}
}

// nextExpireNotifyRun 计算下一次检查时刻（本地时间每日 09:00）
func nextExpireNotifyRun(now time.Time) time.Time {
	next := time.Date(now.Year(), now.Month(), now.Day(), expireNotifyHour, 0, 0, 0, now.Location())
	if !next.After(now) {
		next = next.Add(24 * time.Hour)
	}
	return next
}

// CheckAndNotify 执行一次到期检查与汇总通知，返回是否实际发送
// 未启用 / 未配置渠道 / 今日已发送 / 无到期机器 → 跳过（后两种会记录"今日已检查"）
// 发送失败不记录检查日期，次日自动重试
func (s *ExpireNotifyService) CheckAndNotify() bool {
	if s.agentRepo == nil || s.settings == nil || s.sender == nil {
		return false
	}
	if !s.settings.ExpireNotifyEnabled() {
		return false
	}
	channelID := s.settings.ExpireNotifyChannelID()
	if channelID == 0 {
		return false
	}

	today := time.Now().Format("2006-01-02")
	if s.settings.GetString(SettingExpireLastSent, "") == today {
		return false
	}

	agents, err := s.agentRepo.List()
	if err != nil {
		log.Printf("到期提醒: 读取 Agent 列表失败: %v", err)
		return false
	}

	lead := s.settings.ExpireNotifyLeadDays()
	now := time.Now()
	type expiring struct {
		name string
		days float64 // 剩余天数（负数=已过期）
	}
	var list []expiring
	for _, a := range agents {
		if a.ExpiresAt == nil {
			continue // 永不过期
		}
		days := a.ExpiresAt.Sub(now).Hours() / 24
		if days <= float64(lead) {
			name := a.DisplayName
			if name == "" {
				name = a.Hostname
			}
			list = append(list, expiring{name: name, days: days})
		}
	}

	if len(list) == 0 {
		// 今日无到期机器：记录已检查，避免重启后重复扫描
		_ = s.settings.Update(map[string]string{SettingExpireLastSent: today})
		return false
	}

	// 已过期在前，其余按剩余天数升序
	sort.Slice(list, func(i, j int) bool { return list[i].days < list[j].days })

	var b strings.Builder
	for _, e := range list {
		switch {
		case e.days < 0:
			fmt.Fprintf(&b, "%s：已过期 %.0f 天\n", e.name, math.Ceil(-e.days))
		case e.days < 1:
			fmt.Fprintf(&b, "%s：今天到期\n", e.name)
		default:
			fmt.Fprintf(&b, "%s：剩余 %.0f 天\n", e.name, math.Floor(e.days))
		}
	}
	title := fmt.Sprintf("服务器到期提醒（共 %d 台）", len(list))
	if err := s.sender.SendNotification(channelID, title, b.String()); err != nil {
		log.Printf("到期提醒发送失败（明日重试）: %v", err)
		return false
	}

	_ = s.settings.Update(map[string]string{SettingExpireLastSent: today})
	log.Printf("到期提醒已发送（%d 台，渠道 %d）", len(list), channelID)
	return true
}
