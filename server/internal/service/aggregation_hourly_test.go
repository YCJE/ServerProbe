package service

import (
	"encoding/json"
	"testing"

	"github.com/server-probe/server/internal/model"
	"github.com/server-probe/server/internal/repository"
	sharedmodel "github.com/server-probe/shared/model"
)

// setupAggTestDB 创建测试用 SQLite 数据库（service 包内无法复用 repository 的私有 helper）
func setupAggTestDB(t *testing.T) *repository.SQLiteDB {
	t.Helper()
	db, err := repository.NewSQLiteDB(t.TempDir())
	if err != nil {
		t.Fatalf("创建测试数据库失败: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// newAggService 构造仅用于 computeHourly 测试的聚合服务（monitor 等依赖不参与）
func newAggService(t *testing.T) (*AggregationService, *repository.RecordRepository, *repository.HourlyRepository) {
	t.Helper()
	db := setupAggTestDB(t)
	recordRepo := repository.NewRecordRepository(db.DB())
	hourlyRepo := repository.NewHourlyRepository(db.DB())
	svc := NewAggregationService(nil, recordRepo, hourlyRepo, nil, nil)
	return svc, recordRepo, hourlyRepo
}

// mkRow 构造一条 5 分钟层记录（CPU/Load 为 ×10 整数）
func mkRow(agentID, ts int64, cpu int, mem float64, netRx int64, offline int) model.MetricRecord {
	return model.MetricRecord{
		AgentID:   agentID,
		Timestamp: ts,
		CPUUsage:  cpu,
		MemUsage:  mem,
		NetRx:     netRx,
		NetTx:     netRx / 2,
		Load1:     cpu / 10,
		Offline:   offline,
	}
}

func TestComputeHourly_MeanMinMax(t *testing.T) {
	svc, recordRepo, _ := newAggService(t)

	hour := int64(1700000000)
	hour -= hour % 3600

	// 12 条在线记录: CPU ×10 = 100, 300, ..., 2300（等差步长 200），
	// 均值 ×10 = 1200；mem 40.0..45.5（步长 0.5），均值 42.75；net_rx 1000..12000（步长 1000），均值 6500
	for i := 0; i < 12; i++ {
		row := mkRow(1, hour+int64(i+1)*300, 100+i*200, 40.0+float64(i)*0.5, int64(1000+i*1000), 0)
		if err := recordRepo.Create(&row); err != nil {
			t.Fatalf("写入失败: %v", err)
		}
	}

	rec, err := svc.computeHourly(1, hour)
	if err != nil {
		t.Fatalf("computeHourly 失败: %v", err)
	}

	if rec.SampleCount != 12 || rec.OfflineSamples != 0 || rec.Offline != 0 {
		t.Errorf("样本统计错误: sample=%d offline_samples=%d offline=%d", rec.SampleCount, rec.OfflineSamples, rec.Offline)
	}
	if rec.CPUUsage != 1200 {
		t.Errorf("CPU 均值错误: 期望 ×10=1200, 得到 %d", rec.CPUUsage)
	}
	if rec.CPUMin != 100 || rec.CPUMax != 2300 {
		t.Errorf("CPU 极值错误: min=%d max=%d (期望 100/2300)", rec.CPUMin, rec.CPUMax)
	}
	// mem 均值 = (40+45.5)/2 = 42.75
	if rec.MemUsage < 42.74 || rec.MemUsage > 42.76 {
		t.Errorf("内存均值错误: 期望 42.75, 得到 %f", rec.MemUsage)
	}
	if rec.MemMin != 40.0 || rec.MemMax != 45.5 {
		t.Errorf("内存极值错误: min=%f max=%f", rec.MemMin, rec.MemMax)
	}
	if rec.NetRx != 6500 {
		t.Errorf("下行均值错误: 期望 6500, 得到 %d", rec.NetRx)
	}
}

func TestComputeHourly_OfflineMajority(t *testing.T) {
	svc, recordRepo, _ := newAggService(t)

	hour := int64(1700000000)
	hour -= hour % 3600

	// 12 行中 6 行离线占位（指标零值）→ 多数规则判离线，均值仅用 6 行在线数据
	for i := 0; i < 12; i++ {
		offline := 1
		cpu := 0
		mem := 0.0
		netRx := int64(0)
		if i%2 == 0 {
			offline = 0
			cpu = 500 // 50%
			mem = 60.0
			netRx = 3000
		}
		row := mkRow(1, hour+int64(i+1)*300, cpu, mem, netRx, offline)
		if err := recordRepo.Create(&row); err != nil {
			t.Fatalf("写入失败: %v", err)
		}
	}

	rec, err := svc.computeHourly(1, hour)
	if err != nil {
		t.Fatalf("computeHourly 失败: %v", err)
	}

	if rec.Offline != 1 {
		t.Errorf("离线样本过半应判离线: offline=%d", rec.Offline)
	}
	if rec.SampleCount != 12 || rec.OfflineSamples != 6 {
		t.Errorf("样本统计错误: sample=%d offline_samples=%d", rec.SampleCount, rec.OfflineSamples)
	}
	// 均值仅统计在线行: CPU=500, mem=60, netRx=3000
	if rec.CPUUsage != 500 || rec.MemUsage != 60.0 || rec.NetRx != 3000 {
		t.Errorf("均值未排除离线行: cpu=%d mem=%f net_rx=%d", rec.CPUUsage, rec.MemUsage, rec.NetRx)
	}
}

func TestComputeHourly_AllOffline(t *testing.T) {
	svc, recordRepo, _ := newAggService(t)

	hour := int64(1700000000)
	hour -= hour % 3600

	for i := 0; i < 12; i++ {
		row := mkRow(1, hour+int64(i+1)*300, 0, 0, 0, 1)
		if err := recordRepo.Create(&row); err != nil {
			t.Fatalf("写入失败: %v", err)
		}
	}

	rec, err := svc.computeHourly(1, hour)
	if err != nil {
		t.Fatalf("computeHourly 失败: %v", err)
	}
	if rec.Offline != 1 || rec.SampleCount != 12 || rec.OfflineSamples != 12 {
		t.Errorf("全离线统计错误: offline=%d sample=%d offline_samples=%d", rec.Offline, rec.SampleCount, rec.OfflineSamples)
	}
	if rec.CPUUsage != 0 || rec.MemUsage != 0 {
		t.Errorf("全离线小时指标应为零值: cpu=%d mem=%f", rec.CPUUsage, rec.MemUsage)
	}
}

func TestComputeHourly_EmptyHour(t *testing.T) {
	svc, _, _ := newAggService(t)

	hour := int64(1700000000)
	hour -= hour % 3600

	// 无任何 5 分钟行（服务停机窗口）→ offline 占位，时间线连续
	rec, err := svc.computeHourly(1, hour)
	if err != nil {
		t.Fatalf("computeHourly 失败: %v", err)
	}
	if rec.Offline != 1 || rec.SampleCount != 0 {
		t.Errorf("空洞占位错误: offline=%d sample=%d", rec.Offline, rec.SampleCount)
	}
	if rec.Timestamp != hour {
		t.Errorf("时间戳未对齐整点: %d", rec.Timestamp)
	}
}

func TestComputeHourly_PingAggregation(t *testing.T) {
	svc, recordRepo, _ := newAggService(t)

	hour := int64(1700000000)
	hour -= hour % 3600

	// 两个目标交错出现，各自平均
	pingA1 := []sharedmodel.PingResult{{Target: "1.1.1.1", Name: "CF", Method: "icmp", AvgLatency: 10, MinLatency: 8, MaxLatency: 12, Jitter: 1, Loss: 0}}
	pingA2 := []sharedmodel.PingResult{{Target: "1.1.1.1", Name: "CF", Method: "icmp", AvgLatency: 20, MinLatency: 18, MaxLatency: 22, Jitter: 2, Loss: 0}}
	pingB1 := []sharedmodel.PingResult{{Target: "8.8.8.8", Name: "Google", Method: "icmp", AvgLatency: 40, MinLatency: 38, MaxLatency: 42, Jitter: 3, Loss: 0}}

	rows := []struct {
		ts    int64
		pings []sharedmodel.PingResult
	}{
		{hour + 300, pingA1},
		{hour + 600, pingB1},
		{hour + 900, pingA2},
	}
	for _, r := range rows {
		b, _ := json.Marshal(r.pings)
		row := mkRow(1, r.ts, 100, 50, 1000, 0)
		row.PingData = string(b)
		if err := recordRepo.Create(&row); err != nil {
			t.Fatalf("写入失败: %v", err)
		}
	}

	rec, err := svc.computeHourly(1, hour)
	if err != nil {
		t.Fatalf("computeHourly 失败: %v", err)
	}

	var pings []sharedmodel.PingResult
	if err := json.Unmarshal([]byte(rec.PingData), &pings); err != nil {
		t.Fatalf("解析聚合 ping 数据失败: %v", err)
	}
	if len(pings) != 2 {
		t.Fatalf("期望按目标分组为 2 条, 得到 %d", len(pings))
	}

	byTarget := map[string]sharedmodel.PingResult{}
	for _, p := range pings {
		byTarget[p.Target] = p
	}
	a := byTarget["1.1.1.1"]
	if a.AvgLatency != 15 || a.MinLatency != 13 || a.MaxLatency != 17 || a.Jitter != 1.5 {
		t.Errorf("目标 A 聚合错误: avg=%f min=%f max=%f jitter=%f", a.AvgLatency, a.MinLatency, a.MaxLatency, a.Jitter)
	}
	b := byTarget["8.8.8.8"]
	if b.AvgLatency != 40 {
		t.Errorf("目标 B 聚合错误: avg=%f", b.AvgLatency)
	}
}

func TestRollupHourly_BackfillAndIncremental(t *testing.T) {
	svc, recordRepo, hourlyRepo := newAggService(t)

	base := int64(1700000000)
	base -= base % 3600

	// 写入 3 个小时的 5 分钟数据（每小时 4 条，模拟稀疏采样）
	for h := int64(0); h < 3; h++ {
		for i := 0; i < 4; i++ {
			row := mkRow(1, base+h*3600+int64(i+1)*300, 100, 50, 1000, 0)
			if err := recordRepo.Create(&row); err != nil {
				t.Fatalf("写入失败: %v", err)
			}
		}
	}

	agent := model.Agent{ID: 1}
	// now 足够晚：base+3h 已完整结束并超过一个聚合周期。
	// lastReady = base+3h，故除 3 个数据小时外还会为 base+3h 写空洞离线占位（时间线连续）
	now := base + 4*3600 + 600
	svc.rollupHourly([]model.Agent{agent}, now)

	// 首次回填：base..base+2h 共 3 条数据小时 + base+3h 空洞占位 = 4 条
	records, err := hourlyRepo.GetByAgentAndTimeRange(1, base, base+3*3600)
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if len(records) != 4 {
		t.Fatalf("期望回填 3 条数据小时 + 1 条空洞占位, 得到 %d", len(records))
	}
	for i, rec := range records {
		if rec.Timestamp != base+int64(i)*3600 {
			t.Errorf("小时 %d 时间戳错误: %d", i, rec.Timestamp)
		}
		if i < 3 {
			if rec.SampleCount != 4 {
				t.Errorf("小时 %d 样本数错误: %d", i, rec.SampleCount)
			}
			if rec.Offline != 0 {
				t.Errorf("小时 %d 不应判离线: offline=%d", i, rec.Offline)
			}
		} else if rec.SampleCount != 0 || rec.Offline != 1 {
			t.Errorf("空洞小时应写离线占位: sample=%d offline=%d", rec.SampleCount, rec.Offline)
		}
	}

	// 再次执行（无新增数据）：lastRolled 增量跳过，行数不变
	svc.rollupHourly([]model.Agent{agent}, now+600)
	records2, _ := hourlyRepo.GetByAgentAndTimeRange(1, base, base+3*3600)
	if len(records2) != 4 {
		t.Errorf("重复 rollup 应幂等: 期望 4 条, 得到 %d", len(records2))
	}
}
