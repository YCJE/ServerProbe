package service

import (
	"testing"
	"time"

	"github.com/server-probe/server/internal/model"
	"github.com/server-probe/server/internal/repository"
)

func setupAuditTest(t *testing.T) (*AuditService, *repository.AuditLogRepository) {
	t.Helper()
	tmpDir := t.TempDir()
	db, err := repository.NewSQLiteDB(tmpDir)
	if err != nil {
		t.Fatalf("创建测试数据库失败: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	repo := repository.NewAuditLogRepository(db.DB())
	return NewAuditService(repo), repo
}

// waitForRecords 轮询等待审计日志异步落库
func waitForRecords(t *testing.T, repo *repository.AuditLogRepository, want int64) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		_, total, err := repo.List(repository.AuditLogQuery{Page: 1, PageSize: 100})
		if err == nil && total >= want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("等待审计日志落库超时（期望 %d 条）", want)
}

func TestAuditService_AsyncWrite(t *testing.T) {
	svc, repo := setupAuditTest(t)
	svc.Start()
	defer svc.Stop()

	svc.Record(model.AuditLog{AdminID: 1, Username: "admin", Action: "auth.login", Success: true})
	svc.Record(model.AuditLog{AdminID: 1, Username: "admin", Action: "DELETE /api/v1/agents/:id", Success: false})

	waitForRecords(t, repo, 2)

	logs, total, err := repo.List(repository.AuditLogQuery{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if total != 2 || len(logs) != 2 {
		t.Fatalf("total=%d len=%d, want 2/2", total, len(logs))
	}
	// 倒序：最新在前
	if logs[0].Action != "DELETE /api/v1/agents/:id" {
		t.Errorf("顺序错误: %s", logs[0].Action)
	}
}

func TestAuditService_BufferOverflowDrops(t *testing.T) {
	svc, repo := setupAuditTest(t)
	// 不启动 writeLoop，缓冲固定不消费
	const overflow = 300 // 超过 auditBufferSize=256
	for i := 0; i < overflow; i++ {
		svc.Record(model.AuditLog{AdminID: 1, Action: "auth.login"})
	}

	// 启动后 drain（Stop 会 flush 缓冲剩余条目）
	svc.Start()
	svc.Stop()

	_, total, err := repo.List(repository.AuditLogQuery{Page: 1, PageSize: 500})
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	// 缓冲容量 256：超出部分被丢弃，落库恰好 256 条（Record 不阻塞、不 panic）
	if total != 256 {
		t.Errorf("total=%d, want 256（缓冲容量）", total)
	}
}

func TestAuditService_StopFlushesBuffer(t *testing.T) {
	svc, repo := setupAuditTest(t)
	// 先 Record 再 Start：Stop 的 drain 逻辑应把缓冲内条目全部写库
	svc.Record(model.AuditLog{AdminID: 1, Action: "auth.login"})
	svc.Record(model.AuditLog{AdminID: 1, Action: "auth.setup"})
	svc.Record(model.AuditLog{AdminID: 1, Action: "PUT /api/v1/settings"})

	svc.Start()
	svc.Stop()

	_, total, err := repo.List(repository.AuditLogQuery{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if total != 3 {
		t.Errorf("Stop 后应 flush 全部缓冲: total=%d, want 3", total)
	}
}

func TestAuditService_NilRepoNoop(t *testing.T) {
	// repo 为 nil 时（测试/降级场景）Record 不应 panic
	svc := NewAuditService(nil)
	svc.Start()
	defer svc.Stop()
	svc.Record(model.AuditLog{AdminID: 1, Action: "auth.login"})
}
