package repository

import (
	"testing"
	"time"

	"github.com/server-probe/server/internal/model"
)

func newSession(adminID int64, expiresAt time.Time) *model.Session {
	sessionID, err := GenerateSessionID()
	if err != nil {
		panic(err)
	}
	return &model.Session{
		SessionID:  sessionID,
		AdminID:    adminID,
		IP:         "127.0.0.1",
		UserAgent:  "test-agent",
		LastSeenAt: time.Now(),
		ExpiresAt:  expiresAt,
	}
}

func TestSessionRepository_CreateAndGet(t *testing.T) {
	db := setupTestDB(t)
	repo := NewSessionRepository(db.DB())

	s := newSession(1, time.Now().Add(12*time.Hour))
	if err := repo.Create(s); err != nil {
		t.Fatalf("创建会话失败: %v", err)
	}

	got, err := repo.GetBySessionID(s.SessionID)
	if err != nil {
		t.Fatalf("查询会话失败: %v", err)
	}
	if got.AdminID != 1 || got.IP != "127.0.0.1" {
		t.Errorf("会话字段错误: admin_id=%d ip=%s", got.AdminID, got.IP)
	}
	if got.RevokedAt != nil {
		t.Errorf("新会话不应处于撤销状态")
	}
}

func TestSessionRepository_Revoke(t *testing.T) {
	db := setupTestDB(t)
	repo := NewSessionRepository(db.DB())

	s := newSession(1, time.Now().Add(12*time.Hour))
	if err := repo.Create(s); err != nil {
		t.Fatalf("创建会话失败: %v", err)
	}

	// 撤销后 RevokedAt 非空；重复撤销幂等
	if err := repo.Revoke(s.SessionID); err != nil {
		t.Fatalf("撤销会话失败: %v", err)
	}
	if err := repo.Revoke(s.SessionID); err != nil {
		t.Fatalf("重复撤销失败: %v", err)
	}

	got, err := repo.GetBySessionID(s.SessionID)
	if err != nil {
		t.Fatalf("查询会话失败: %v", err)
	}
	if got.RevokedAt == nil {
		t.Errorf("会话应已撤销")
	}
}

func TestSessionRepository_RevokeAllOther(t *testing.T) {
	db := setupTestDB(t)
	repo := NewSessionRepository(db.DB())

	keep := newSession(1, time.Now().Add(12*time.Hour))
	other1 := newSession(1, time.Now().Add(12*time.Hour))
	other2 := newSession(1, time.Now().Add(12*time.Hour))
	otherAdmin := newSession(2, time.Now().Add(12*time.Hour))
	for _, s := range []*model.Session{keep, other1, other2, otherAdmin} {
		if err := repo.Create(s); err != nil {
			t.Fatalf("创建会话失败: %v", err)
		}
	}

	n, err := repo.RevokeAllOther(1, keep.SessionID)
	if err != nil {
		t.Fatalf("撤销其他会话失败: %v", err)
	}
	if n != 2 {
		t.Errorf("期望撤销 2 个会话, 实际撤销 %d", n)
	}

	// 保留会话未被撤销
	gotKeep, _ := repo.GetBySessionID(keep.SessionID)
	if gotKeep.RevokedAt != nil {
		t.Errorf("当前会话不应被撤销")
	}
	// 其他管理员会话不受影响
	gotOtherAdmin, _ := repo.GetBySessionID(otherAdmin.SessionID)
	if gotOtherAdmin.RevokedAt != nil {
		t.Errorf("其他管理员的会话不应被撤销")
	}
}

func TestSessionRepository_ListByAdmin(t *testing.T) {
	db := setupTestDB(t)
	repo := NewSessionRepository(db.DB())

	// 活跃、已撤销、过期超过 7 天、其他管理员
	active := newSession(1, time.Now().Add(12*time.Hour))
	revoked := newSession(1, time.Now().Add(12*time.Hour))
	expiredOld := newSession(1, time.Now().Add(-8*24*time.Hour))
	otherAdmin := newSession(2, time.Now().Add(12*time.Hour))
	for _, s := range []*model.Session{active, revoked, expiredOld, otherAdmin} {
		if err := repo.Create(s); err != nil {
			t.Fatalf("创建会话失败: %v", err)
		}
	}
	if err := repo.Revoke(revoked.SessionID); err != nil {
		t.Fatalf("撤销失败: %v", err)
	}

	sessions, err := repo.ListByAdmin(1)
	if err != nil {
		t.Fatalf("查询会话列表失败: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("期望 2 条会话（活跃+近期撤销）, 得到 %d", len(sessions))
	}
}

func TestSessionRepository_CleanupExpired(t *testing.T) {
	db := setupTestDB(t)
	repo := NewSessionRepository(db.DB())

	// 应清理：已撤销超 7 天、已过期超 7 天；应保留：活跃、刚撤销
	revokedOld := newSession(1, time.Now().Add(12*time.Hour))
	expiredOld := newSession(1, time.Now().Add(-8*24*time.Hour))
	active := newSession(1, time.Now().Add(12*time.Hour))
	revokedRecent := newSession(1, time.Now().Add(12*time.Hour))
	for _, s := range []*model.Session{revokedOld, expiredOld, active, revokedRecent} {
		if err := repo.Create(s); err != nil {
			t.Fatalf("创建会话失败: %v", err)
		}
	}
	if err := repo.Revoke(revokedOld.SessionID); err != nil {
		t.Fatalf("撤销失败: %v", err)
	}
	if err := repo.Revoke(revokedRecent.SessionID); err != nil {
		t.Fatalf("撤销失败: %v", err)
	}
	// 将 revokedOld 的撤销时间回拨到 8 天前（Revoke 只能写当前时间，测试用直连 DB 模拟历史数据）
	db.DB().Exec("UPDATE sessions SET revoked_at = ? WHERE session_id = ?",
		time.Now().Add(-8*24*time.Hour), revokedOld.SessionID)

	n, err := repo.CleanupExpired()
	if err != nil {
		t.Fatalf("清理失败: %v", err)
	}
	if n != 2 {
		t.Errorf("期望清理 2 条, 实际清理 %d", n)
	}

	// 活跃与刚撤销的会话仍在
	if _, err := repo.GetBySessionID(active.SessionID); err != nil {
		t.Errorf("活跃会话不应被清理")
	}
	if _, err := repo.GetBySessionID(revokedRecent.SessionID); err != nil {
		t.Errorf("刚撤销的会话不应被清理（保留 7 天供展示）")
	}
}
