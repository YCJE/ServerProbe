package repository

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/server-probe/server/internal/model"
)

// setupTestDB 创建测试用 SQLite 数据库
func setupTestDB(t *testing.T) *SQLiteDB {
	t.Helper()
	tmpDir := t.TempDir()
	db, err := NewSQLiteDB(tmpDir)
	if err != nil {
		t.Fatalf("创建测试数据库失败: %v", err)
	}
	t.Cleanup(func() {
		db.Close()
		os.RemoveAll(tmpDir)
	})
	return db
}

func TestNewSQLiteDB(t *testing.T) {
	tmpDir := t.TempDir()
	defer os.RemoveAll(tmpDir)

	db, err := NewSQLiteDB(tmpDir)
	if err != nil {
		t.Fatalf("创建数据库失败: %v", err)
	}
	defer db.Close()

	// 验证数据库文件存在
	dbPath := filepath.Join(tmpDir, "probe.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("数据库文件未创建")
	}

	// 验证表已创建
	tables := []string{"agents", "register_codes", "alert_rules", "notify_channels",
		"ping_targets", "metric_records", "admin", "share_pages"}

	for _, table := range tables {
		var count int64
		result := db.DB().Table(table).Count(&count)
		if result.Error != nil {
			t.Errorf("表 %s 不存在或查询失败: %v", table, result.Error)
		}
	}
}

func TestNewSQLiteDB_CreatesDataDir(t *testing.T) {
	tmpDir := t.TempDir()
	defer os.RemoveAll(tmpDir)

	dataDir := filepath.Join(tmpDir, "nested", "data")
	db, err := NewSQLiteDB(dataDir)
	if err != nil {
		t.Fatalf("创建数据库失败: %v", err)
	}
	defer db.Close()

	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		t.Error("数据目录未自动创建")
	}
}

func TestAgentRepository_CRUD(t *testing.T) {
	db := setupTestDB(t)
	repo := NewAgentRepository(db.DB())

	// 创建
	agent := &model.Agent{
		Token:            "test-token-123",
		Hostname:         "web-server-01",
		OS:               "linux",
		Arch:             "amd64",
		AgentVersion:     "v1.0.0",
		HostFingerprint:  "fingerprint-abc",
		Online:           true,
	}
	err := repo.Create(agent)
	if err != nil {
		t.Fatalf("创建 Agent 失败: %v", err)
	}
	if agent.ID == 0 {
		t.Error("Agent ID 未自动生成")
	}

	// 按 ID 查询
	found, err := repo.GetByID(agent.ID)
	if err != nil {
		t.Fatalf("按 ID 查询失败: %v", err)
	}
	if found.Hostname != "web-server-01" {
		t.Errorf("Hostname 错误: 期望 web-server-01, 得到 %s", found.Hostname)
	}

	// 按 Token 查询
	foundByToken, err := repo.GetByToken("test-token-123")
	if err != nil {
		t.Fatalf("按 Token 查询失败: %v", err)
	}
	if foundByToken.ID != agent.ID {
		t.Errorf("Token 查询 ID 不匹配")
	}

	// 按指纹查询
	foundByFP, err := repo.GetByFingerprint("fingerprint-abc")
	if err != nil {
		t.Fatalf("按指纹查询失败: %v", err)
	}
	if foundByFP.ID != agent.ID {
		t.Errorf("指纹查询 ID 不匹配")
	}

	// 列表
	agents, err := repo.List()
	if err != nil {
		t.Fatalf("列表查询失败: %v", err)
	}
	if len(agents) != 1 {
		t.Errorf("列表数量错误: 期望 1, 得到 %d", len(agents))
	}

	// 更新在线状态
	err = repo.UpdateOnlineStatus(agent.ID, false)
	if err != nil {
		t.Fatalf("更新在线状态失败: %v", err)
	}
	found, _ = repo.GetByID(agent.ID)
	if found.Online != false {
		t.Error("在线状态未更新")
	}

	// 删除
	err = repo.Delete(agent.ID)
	if err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	_, err = repo.GetByID(agent.ID)
	if err == nil {
		t.Error("删除后仍能查询到")
	}
}

func TestRegisterCodeRepository_CRUD(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRegisterCodeRepository(db.DB())

	// 创建注册码
	code := &model.RegisterCode{
		Code:      "ABC123XY",
		ExpiresAt: time.Now().Add(15 * time.Minute),
	}
	err := repo.Create(code)
	if err != nil {
		t.Fatalf("创建注册码失败: %v", err)
	}

	// 查询
	found, err := repo.GetByCode("ABC123XY")
	if err != nil {
		t.Fatalf("查询注册码失败: %v", err)
	}
	if found.Used != false {
		t.Error("新注册码应为未使用")
	}

	// 统计未使用
	count, err := repo.CountUnused()
	if err != nil {
		t.Fatalf("统计失败: %v", err)
	}
	if count != 1 {
		t.Errorf("未使用数量错误: 期望 1, 得到 %d", count)
	}

	// 标记已使用
	err = repo.MarkUsed("ABC123XY", 1)
	if err != nil {
		t.Fatalf("标记已使用失败: %v", err)
	}

	// 验证已标记
	found, _ = repo.GetByCode("ABC123XY")
	if found.Used != true {
		t.Error("注册码未标记为已使用")
	}
	if found.UsedByAgentID != 1 {
		t.Errorf("使用 Agent ID 错误: 期望 1, 得到 %d", found.UsedByAgentID)
	}

	// 重复标记应失败
	err = repo.MarkUsed("ABC123XY", 2)
	if err == nil {
		t.Error("重复标记应失败")
	}

	// 统计未使用
	count, _ = repo.CountUnused()
	if count != 0 {
		t.Errorf("未使用数量错误: 期望 0, 得到 %d", count)
	}
}

func TestRegisterCodeRepository_DeleteExpired(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRegisterCodeRepository(db.DB())

	// 创建过期注册码
	expiredCode := &model.RegisterCode{
		Code:      "EXPIRED1",
		ExpiresAt: time.Now().Add(-1 * time.Minute),
	}
	_ = repo.Create(expiredCode)

	// 创建有效注册码
	validCode := &model.RegisterCode{
		Code:      "VALID001",
		ExpiresAt: time.Now().Add(15 * time.Minute),
	}
	_ = repo.Create(validCode)

	// 删除过期
	err := repo.DeleteExpired()
	if err != nil {
		t.Fatalf("删除过期注册码失败: %v", err)
	}

	// 验证过期注册码已删除
	_, err = repo.GetByCode("EXPIRED1")
	if err == nil {
		t.Error("过期注册码应已删除")
	}

	// 验证有效注册码仍存在
	_, err = repo.GetByCode("VALID001")
	if err != nil {
		t.Error("有效注册码不应被删除")
	}
}

func TestAlertRepository_CRUD(t *testing.T) {
	db := setupTestDB(t)
	repo := NewAlertRepository(db.DB())

	rule := &model.AlertRule{
		Name:            "CPU高负载",
		Metric:          "cpu_usage",
		Operator:        ">",
		Threshold:       80,
		Duration:        300,
		Enabled:         true,
		NotifyChannelID: 1,
	}
	err := repo.Create(rule)
	if err != nil {
		t.Fatalf("创建告警规则失败: %v", err)
	}

	// 查询已启用
	rules, err := repo.ListEnabled()
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if len(rules) != 1 {
		t.Errorf("已启用规则数量错误: 期望 1, 得到 %d", len(rules))
	}

	// 更新
	rule.Enabled = false
	err = repo.Update(rule)
	if err != nil {
		t.Fatalf("更新失败: %v", err)
	}

	// 验证已禁用
	rules, _ = repo.ListEnabled()
	if len(rules) != 0 {
		t.Error("禁用后不应有已启用规则")
	}
}

func TestRecordRepository_CRUD(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRecordRepository(db.DB())

	now := time.Now().Unix()

	// 创建记录
	record := &model.MetricRecord{
		AgentID:   1,
		Timestamp: now,
		CPUUsage:  45.2,
		MemUsage:  60.0,
		NetRx:     1048576,
		NetTx:     524288,
	}
	err := repo.Create(record)
	if err != nil {
		t.Fatalf("创建记录失败: %v", err)
	}

	// 按时间范围查询
	records, err := repo.GetByAgentAndTimeRange(1, now-60, now+60)
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if len(records) != 1 {
		t.Errorf("记录数量错误: 期望 1, 得到 %d", len(records))
	}
	if records[0].CPUUsage != 45.2 {
		t.Errorf("CPU 使用率错误: 期望 45.2, 得到 %f", records[0].CPUUsage)
	}

	// 清理过期数据
	deleted, err := repo.CleanupExpired(90)
	if err != nil {
		t.Fatalf("清理失败: %v", err)
	}
	// 当前数据不应被清理（在 90 天内）
	if deleted != 0 {
		t.Errorf("不应清理任何数据, 得到 %d", deleted)
	}

	// 创建过期数据并清理
	oldRecord := &model.MetricRecord{
		AgentID:   1,
		Timestamp: now - 91*24*3600, // 91 天前
		CPUUsage:  10.0,
	}
	_ = repo.Create(oldRecord)

	deleted, err = repo.CleanupExpired(90)
	if err != nil {
		t.Fatalf("清理失败: %v", err)
	}
	if deleted != 1 {
		t.Errorf("应清理 1 条数据, 得到 %d", deleted)
	}
}

func TestAdminRepository_CRUD(t *testing.T) {
	db := setupTestDB(t)
	repo := NewAdminRepository(db.DB())

	admin := &model.Admin{
		Username:     "admin",
		PasswordHash: "$2a$12$somehash",
	}
	err := repo.Create(admin)
	if err != nil {
		t.Fatalf("创建管理员失败: %v", err)
	}

	// 按用户名查询
	found, err := repo.GetByUsername("admin")
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if found.PasswordHash != "$2a$12$somehash" {
		t.Error("密码哈希不匹配")
	}

	// 统计
	count, err := repo.Count()
	if err != nil {
		t.Fatalf("统计失败: %v", err)
	}
	if count != 1 {
		t.Errorf("管理员数量错误: 期望 1, 得到 %d", count)
	}
}
