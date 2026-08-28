package repository

import (
	"testing"
	"time"

	"github.com/server-probe/server/internal/model"
)

func TestAuditLogRepository_CreateAndList(t *testing.T) {
	db := setupTestDB(t)
	repo := NewAuditLogRepository(db.DB())

	entries := []model.AuditLog{
		{AdminID: 1, Username: "admin", Action: "auth.login", Target: "admin", Success: true, IP: "1.1.1.1"},
		{AdminID: 1, Username: "admin", Action: "DELETE /api/v1/agents/:id", Target: "/api/v1/agents/42", Success: true, IP: "1.1.1.1"},
		{AdminID: 0, Username: "attacker", Action: "auth.login", Target: "attacker", Success: false, IP: "2.2.2.2"},
	}
	for i := range entries {
		if err := repo.Create(&entries[i]); err != nil {
			t.Fatalf("写入审计日志失败: %v", err)
		}
	}

	t.Run("无过滤按时间倒序", func(t *testing.T) {
		logs, total, err := repo.List(AuditLogQuery{Page: 1, PageSize: 10})
		if err != nil {
			t.Fatalf("查询失败: %v", err)
		}
		if total != 3 || len(logs) != 3 {
			t.Fatalf("total=%d len=%d, want 3/3", total, len(logs))
		}
		if logs[0].Action != "auth.login" || logs[0].Username != "attacker" {
			t.Errorf("倒序错误: 首条应为最后写入的 attacker 记录")
		}
	})

	t.Run("按用户名过滤", func(t *testing.T) {
		logs, total, err := repo.List(AuditLogQuery{Username: "admin", Page: 1, PageSize: 10})
		if err != nil {
			t.Fatalf("查询失败: %v", err)
		}
		if total != 2 {
			t.Errorf("username=admin total=%d, want 2", total)
		}
		for _, l := range logs {
			if l.Username != "admin" {
				t.Errorf("过滤泄漏: %s", l.Username)
			}
		}
	})

	t.Run("按操作模糊匹配", func(t *testing.T) {
		_, total, err := repo.List(AuditLogQuery{Action: "DELETE", Page: 1, PageSize: 10})
		if err != nil {
			t.Fatalf("查询失败: %v", err)
		}
		if total != 1 {
			t.Errorf("action~DELETE total=%d, want 1", total)
		}
	})

	t.Run("按成功状态过滤", func(t *testing.T) {
		fail := false
		_, total, err := repo.List(AuditLogQuery{Success: &fail, Page: 1, PageSize: 10})
		if err != nil {
			t.Fatalf("查询失败: %v", err)
		}
		if total != 1 {
			t.Errorf("success=false total=%d, want 1", total)
		}
	})

	t.Run("分页", func(t *testing.T) {
		logs, total, err := repo.List(AuditLogQuery{Page: 2, PageSize: 2})
		if err != nil {
			t.Fatalf("查询失败: %v", err)
		}
		if total != 3 || len(logs) != 1 {
			t.Errorf("page=2 size=2: total=%d len=%d, want 3/1", total, len(logs))
		}
	})
}

func TestAuditLogRepository_CleanupOlderThan(t *testing.T) {
	db := setupTestDB(t)
	repo := NewAuditLogRepository(db.DB())

	old := model.AuditLog{AdminID: 1, Username: "admin", Action: "auth.login", Success: true}
	if err := repo.Create(&old); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	// 手动回溯创建时间（绕过 autoCreateTime）
	if err := db.DB().Model(&model.AuditLog{}).Where("id = ?", old.ID).
		Update("created_at", time.Now().AddDate(0, 0, -200)).Error; err != nil {
		t.Fatalf("回溯时间失败: %v", err)
	}

	recent := model.AuditLog{AdminID: 1, Username: "admin", Action: "auth.login", Success: true}
	if err := repo.Create(&recent); err != nil {
		t.Fatalf("写入失败: %v", err)
	}

	deleted, err := repo.CleanupOlderThan(time.Now().AddDate(0, 0, -180))
	if err != nil {
		t.Fatalf("清理失败: %v", err)
	}
	if deleted != 1 {
		t.Errorf("deleted=%d, want 1", deleted)
	}

	_, total, err := repo.List(AuditLogQuery{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if total != 1 {
		t.Errorf("清理后 total=%d, want 1", total)
	}
}
