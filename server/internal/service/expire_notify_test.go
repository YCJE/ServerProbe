package service

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/server-probe/server/internal/model"
	"github.com/server-probe/server/internal/repository"
)

// fakeSender 捕获通知调用（替代真实 NotifyService，避免测试依赖外发 HTTP）
type fakeSender struct {
	calls    []fakeSend
	err      error
}

type fakeSend struct {
	channelID int64
	title     string
	content   string
}

func (f *fakeSender) SendNotification(channelID int64, title, content string) error {
	f.calls = append(f.calls, fakeSend{channelID: channelID, title: title, content: content})
	return f.err
}

type expireNotifyEnv struct {
	svc     *ExpireNotifyService
	sender  *fakeSender
	agents  *repository.AgentRepository
	settings *SettingsService
}

func setupExpireNotify(t *testing.T) *expireNotifyEnv {
	t.Helper()
	db, err := repository.NewSQLiteDB(t.TempDir())
	if err != nil {
		t.Fatalf("创建测试数据库失败: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	settingRepo := repository.NewSettingRepository(db.DB())
	settingsSvc, err := NewSettingsService(settingRepo)
	if err != nil {
		t.Fatalf("初始化设置服务失败: %v", err)
	}
	agentRepo := repository.NewAgentRepository(db.DB())
	sender := &fakeSender{}
	svc := NewExpireNotifyService(agentRepo, settingsSvc, sender)
	return &expireNotifyEnv{svc: svc, sender: sender, agents: agentRepo, settings: settingsSvc}
}

func (e *expireNotifyEnv) enableNotify(t *testing.T, leadDays int, channelID int64) {
	t.Helper()
	if err := e.settings.Update(map[string]string{
		SettingExpireNotifyEnabled:   "true",
		SettingExpireNotifyLeadDays:  strconv.Itoa(leadDays),
		SettingExpireNotifyChannelID: strconv.FormatInt(channelID, 10),
	}); err != nil {
		t.Fatalf("写入设置失败: %v", err)
	}
}

func (e *expireNotifyEnv) createAgent(t *testing.T, hostname string, expiresAt *time.Time) {
	t.Helper()
	if err := e.agents.Create(&model.Agent{
		Token:            "tok-" + hostname,
		Hostname:         hostname,
		DisplayName:      hostname,
		HostFingerprint:  "fp-" + hostname,
		ExpiresAt:        expiresAt,
	}); err != nil {
		t.Fatalf("创建 Agent %s 失败: %v", hostname, err)
	}
}

func TestExpireNotify_SendsDailySummary(t *testing.T) {
	e := setupExpireNotify(t)
	e.enableNotify(t, 7, 42)

	now := time.Now()
	in3Days := now.Add(72 * time.Hour)
	in2Days := now.Add(48 * time.Hour)
	expiredYesterday := now.Add(-30 * time.Hour)
	farFuture := now.Add(365 * 24 * time.Hour)

	e.createAgent(t, "vps-3d", &in3Days)
	e.createAgent(t, "vps-2d", &in2Days)
	e.createAgent(t, "vps-expired", &expiredYesterday)
	e.createAgent(t, "vps-far", &farFuture)
	e.createAgent(t, "vps-forever", nil) // 永不过期

	if !e.svc.CheckAndNotify() {
		t.Fatalf("应发送通知")
	}
	if len(e.sender.calls) != 1 {
		t.Fatalf("应恰好一条汇总通知, 得到 %d 条", len(e.sender.calls))
	}
	call := e.sender.calls[0]
	if call.channelID != 42 {
		t.Errorf("渠道 ID 错误: %d", call.channelID)
	}
	// 汇总应含 3 台（剩余≤7 天 + 已过期），不含远期与永不过期机器
	for _, name := range []string{"vps-3d", "vps-2d", "vps-expired"} {
		if !strings.Contains(call.content, name) {
			t.Errorf("通知缺少 %s: %q", name, call.content)
		}
	}
	for _, name := range []string{"vps-far", "vps-forever"} {
		if strings.Contains(call.content, name) {
			t.Errorf("通知不应包含 %s: %q", name, call.content)
		}
	}
	if !strings.Contains(call.content, "已过期") || !strings.Contains(call.content, "剩余") {
		t.Errorf("通知应包含已过期与剩余天数描述: %q", call.content)
	}

	// 防重发：同日第二次检查不再发送
	if e.svc.CheckAndNotify() {
		t.Errorf("同日重复检查不应再次发送")
	}
	if len(e.sender.calls) != 1 {
		t.Errorf("防重发失败: %d 条", len(e.sender.calls))
	}

	// 重启场景：记录仍在 settings 中，重建服务后依然不重发
	svc2 := NewExpireNotifyService(e.agents, e.settings, e.sender)
	if svc2.CheckAndNotify() {
		t.Errorf("重启后不应重发当日通知")
	}
}

func TestExpireNotify_SkipConditions(t *testing.T) {
	t.Run("未启用", func(t *testing.T) {
		e := setupExpireNotify(t)
		exp := time.Now().Add(24 * time.Hour)
		e.createAgent(t, "vps", &exp)
		if e.svc.CheckAndNotify() {
			t.Errorf("未启用时不应发送")
		}
	})

	t.Run("未配置渠道", func(t *testing.T) {
		e := setupExpireNotify(t)
		e.enableNotify(t, 7, 0)
		exp := time.Now().Add(24 * time.Hour)
		e.createAgent(t, "vps", &exp)
		if e.svc.CheckAndNotify() {
			t.Errorf("channel_id=0 时不应发送")
		}
	})

	t.Run("无到期机器记录检查日期", func(t *testing.T) {
		e := setupExpireNotify(t)
		e.enableNotify(t, 7, 1)
		far := time.Now().Add(200 * 24 * time.Hour)
		e.createAgent(t, "vps-far", &far)
		e.createAgent(t, "vps-forever", nil)
		if e.svc.CheckAndNotify() {
			t.Errorf("无到期机器不应发送")
		}
		if got := e.settings.GetString(SettingExpireLastSent, ""); got != time.Now().Format("2006-01-02") {
			t.Errorf("应记录检查日期, 得到 %q", got)
		}
	})

	t.Run("发送失败不记录检查日期", func(t *testing.T) {
		e := setupExpireNotify(t)
		e.enableNotify(t, 7, 1)
		e.sender.err = errors.New("渠道故障")
		exp := time.Now().Add(24 * time.Hour)
		e.createAgent(t, "vps", &exp)
		if e.svc.CheckAndNotify() {
			t.Errorf("发送失败应返回 false")
		}
		if got := e.settings.GetString(SettingExpireLastSent, ""); got != "" {
			t.Errorf("发送失败不应记录检查日期, 得到 %q", got)
		}
		// 故障恢复后次日（同日重试亦允许）可再发
		e.sender.err = nil
		if !e.svc.CheckAndNotify() {
			t.Errorf("故障恢复后应可发送")
		}
	})
}

func TestExpireNotify_LeadDaysBoundary(t *testing.T) {
	e := setupExpireNotify(t)
	e.enableNotify(t, 7, 1)

	// 剩余 8 天 > 提前 7 天 → 不提醒；剩余 6.5 天 → 提醒
	outside := time.Now().Add(8 * 24 * time.Hour)
	inside := time.Now().Add(156 * time.Hour) // 6.5 天
	e.createAgent(t, "vps-out", &outside)
	e.createAgent(t, "vps-in", &inside)

	if !e.svc.CheckAndNotify() {
		t.Fatalf("应发送通知")
	}
	content := e.sender.calls[0].content
	if strings.Contains(content, "vps-out") {
		t.Errorf("剩余 8 天（> 提前 7 天）不应提醒: %q", content)
	}
	if !strings.Contains(content, "vps-in") {
		t.Errorf("剩余 6.5 天应提醒: %q", content)
	}
}

func TestNextExpireNotifyRun(t *testing.T) {
	loc := time.Local
	base := time.Date(2026, 8, 29, 8, 0, 0, 0, loc)
	if got := nextExpireNotifyRun(base); got.Hour() != 9 || got.Day() != 29 {
		t.Errorf("08:00 → 当日 09:00, 得到 %v", got)
	}
	at9 := time.Date(2026, 8, 29, 9, 0, 0, 0, loc)
	if got := nextExpireNotifyRun(at9); got.Day() != 30 {
		t.Errorf("09:00 整 → 次日 09:00, 得到 %v", got)
	}
	after := time.Date(2026, 8, 29, 15, 30, 0, 0, loc)
	if got := nextExpireNotifyRun(after); got.Day() != 30 || got.Hour() != 9 {
		t.Errorf("15:30 → 次日 09:00, 得到 %v", got)
	}
}
