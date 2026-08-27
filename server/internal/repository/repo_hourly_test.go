package repository

import (
	"testing"
	"time"

	"github.com/server-probe/server/internal/model"
)

func newHourlyRecord(agentID, hour int64) *model.MetricRecordHourly {
	return &model.MetricRecordHourly{
		AgentID:      agentID,
		Timestamp:    hour,
		CPUUsage:     350,
		CPUMin:       100,
		CPUMax:       800,
		MemUsage:     50.5,
		MemMin:       40,
		MemMax:       60,
		Load1:        10,
		Load1Max:     30,
		NetRx:        1024,
		NetTx:        2048,
		NetRxMax:     4096,
		NetTxMax:     8192,
		MemTotal:     8 << 30,
		MemUsed:      4 << 30,
		SampleCount:  12,
		Offline:      0,
	}
}

func TestHourlyRepository_UpsertIdempotent(t *testing.T) {
	db := setupTestDB(t)
	repo := NewHourlyRepository(db.DB())

	hour := time.Now().Unix()
	hour -= hour % 3600

	// 第一次写入
	if err := repo.UpsertHourly(newHourlyRecord(1, hour)); err != nil {
		t.Fatalf("首次写入失败: %v", err)
	}
	// 同一小时重算后覆盖写入（值不同）
	updated := newHourlyRecord(1, hour)
	updated.CPUUsage = 500
	updated.SampleCount = 11
	if err := repo.UpsertHourly(updated); err != nil {
		t.Fatalf("覆盖写入失败: %v", err)
	}

	// 幂等：行数不变，指标为最新值
	var count int64
	db.DB().Model(&model.MetricRecordHourly{}).Where("agent_id = ?", 1).Count(&count)
	if count != 1 {
		t.Fatalf("幂等 upsert 失败: 期望 1 行, 得到 %d 行", count)
	}

	records, err := repo.GetByAgentAndTimeRange(1, hour-1, hour+1)
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("期望 1 条记录, 得到 %d", len(records))
	}
	if records[0].CPUUsage != 500 {
		t.Errorf("覆盖后指标错误: 期望 cpu_usage=500, 得到 %d", records[0].CPUUsage)
	}
	if records[0].SampleCount != 11 {
		t.Errorf("覆盖后 sample_count 错误: 期望 11, 得到 %d", records[0].SampleCount)
	}
}

func TestHourlyRepository_GetByAgentAndTimeRange(t *testing.T) {
	db := setupTestDB(t)
	repo := NewHourlyRepository(db.DB())

	base := int64(1700000000)
	base -= base % 3600
	for i := int64(0); i < 24; i++ {
		if err := repo.UpsertHourly(newHourlyRecord(1, base+i*3600)); err != nil {
			t.Fatalf("写入失败: %v", err)
		}
	}
	// 另一 Agent 的数据不应混入
	if err := repo.UpsertHourly(newHourlyRecord(2, base)); err != nil {
		t.Fatalf("写入失败: %v", err)
	}

	records, err := repo.GetByAgentAndTimeRange(1, base+5*3600, base+10*3600)
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	// [base+5h, base+10h] 闭区间共 6 个小时
	if len(records) != 6 {
		t.Fatalf("期望 6 条记录, 得到 %d", len(records))
	}
	if records[0].Timestamp != base+5*3600 || records[len(records)-1].Timestamp != base+10*3600 {
		t.Errorf("时间边界错误: 首=%d 尾=%d", records[0].Timestamp, records[len(records)-1].Timestamp)
	}
}

func TestHourlyRepository_GetLastTimestamp(t *testing.T) {
	db := setupTestDB(t)
	repo := NewHourlyRepository(db.DB())

	// 无记录
	_, has, err := repo.GetLastTimestamp(1)
	if err != nil || has {
		t.Fatalf("无记录时期望 (false, nil), 得到 (%v, %v)", has, err)
	}

	base := int64(1700000000)
	base -= base % 3600
	for i := int64(0); i < 3; i++ {
		if err := repo.UpsertHourly(newHourlyRecord(1, base+i*3600)); err != nil {
			t.Fatalf("写入失败: %v", err)
		}
	}

	last, has, err := repo.GetLastTimestamp(1)
	if err != nil || !has {
		t.Fatalf("查询失败: has=%v err=%v", has, err)
	}
	if last != base+2*3600 {
		t.Errorf("期望最新时间戳 %d, 得到 %d", base+2*3600, last)
	}
}

func TestHourlyRepository_DeleteOlderThan(t *testing.T) {
	db := setupTestDB(t)
	repo := NewHourlyRepository(db.DB())

	base := int64(1700000000)
	base -= base % 3600
	for i := int64(0); i < 10; i++ {
		if err := repo.UpsertHourly(newHourlyRecord(1, base+i*3600)); err != nil {
			t.Fatalf("写入失败: %v", err)
		}
	}

	// 删除 base+5h（含）之前的记录
	deleted, err := repo.DeleteOlderThan(base + 5*3600)
	if err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	if deleted != 5 {
		t.Errorf("期望删除 5 条, 得到 %d", deleted)
	}

	remaining, _ := repo.GetByAgentAndTimeRange(1, 0, base+24*3600)
	if len(remaining) != 5 {
		t.Errorf("期望剩余 5 条, 得到 %d", len(remaining))
	}
}

func TestHourlyRepository_DeleteByAgentID(t *testing.T) {
	db := setupTestDB(t)
	repo := NewHourlyRepository(db.DB())

	base := int64(1700000000)
	base -= base % 3600
	for i := int64(0); i < 3; i++ {
		if err := repo.UpsertHourly(newHourlyRecord(1, base+i*3600)); err != nil {
			t.Fatalf("写入失败: %v", err)
		}
		if err := repo.UpsertHourly(newHourlyRecord(2, base+i*3600)); err != nil {
			t.Fatalf("写入失败: %v", err)
		}
	}

	if err := repo.DeleteByAgentID(1); err != nil {
		t.Fatalf("删除失败: %v", err)
	}

	var count1, count2 int64
	db.DB().Model(&model.MetricRecordHourly{}).Where("agent_id = ?", 1).Count(&count1)
	db.DB().Model(&model.MetricRecordHourly{}).Where("agent_id = ?", 2).Count(&count2)
	if count1 != 0 {
		t.Errorf("Agent 1 的小时记录未删除干净: %d 条残留", count1)
	}
	if count2 != 3 {
		t.Errorf("Agent 2 的记录不应受影响: 期望 3, 得到 %d", count2)
	}
}
