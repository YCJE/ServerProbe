package repository

import (
	"database/sql"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/server-probe/server/internal/model"
)

// HourlyRepository 小时级聚合数据 CRUD
// 小时表写入频率极低（每 Agent 每小时 1 行），无需 BatchWriter，直接同步写入
type HourlyRepository struct {
	db *gorm.DB
}

// NewHourlyRepository 创建小时级数据 repository
func NewHourlyRepository(db *gorm.DB) *HourlyRepository {
	return &HourlyRepository{db: db}
}

// upsertHourlySQL 整行覆盖式 upsert：
// 小时记录由该小时全部 5 分钟记录全量重算，冲突时用最新计算结果覆盖全部指标列
const upsertHourlySQL = `INSERT INTO metric_records_hourly
(agent_id, timestamp, cpu_usage, mem_usage, load_1, load_5, load_15, net_rx, net_tx,
 cpu_min, cpu_max, mem_min, mem_max, load_1_max, net_rx_max, net_tx_max,
 mem_total, mem_used, swap_total, swap_used, disk_usage, uptime, process_count,
 tcp_connections, udp_connections, ping_data, offline, sample_count, offline_samples)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(agent_id, timestamp) DO UPDATE SET
 cpu_usage = excluded.cpu_usage, mem_usage = excluded.mem_usage,
 load_1 = excluded.load_1, load_5 = excluded.load_5, load_15 = excluded.load_15,
 net_rx = excluded.net_rx, net_tx = excluded.net_tx,
 cpu_min = excluded.cpu_min, cpu_max = excluded.cpu_max,
 mem_min = excluded.mem_min, mem_max = excluded.mem_max,
 load_1_max = excluded.load_1_max, net_rx_max = excluded.net_rx_max, net_tx_max = excluded.net_tx_max,
 mem_total = excluded.mem_total, mem_used = excluded.mem_used,
 swap_total = excluded.swap_total, swap_used = excluded.swap_used,
 disk_usage = excluded.disk_usage, uptime = excluded.uptime, process_count = excluded.process_count,
 tcp_connections = excluded.tcp_connections, udp_connections = excluded.udp_connections,
 ping_data = excluded.ping_data, offline = excluded.offline,
 sample_count = excluded.sample_count, offline_samples = excluded.offline_samples`

// UpsertHourly 写入/覆盖一条小时聚合记录（幂等）
func (r *HourlyRepository) UpsertHourly(rec *model.MetricRecordHourly) error {
	return r.db.Exec(upsertHourlySQL,
		rec.AgentID, rec.Timestamp, rec.CPUUsage, rec.MemUsage,
		rec.Load1, rec.Load5, rec.Load15, rec.NetRx, rec.NetTx,
		rec.CPUMin, rec.CPUMax, rec.MemMin, rec.MemMax,
		rec.Load1Max, rec.NetRxMax, rec.NetTxMax,
		rec.MemTotal, rec.MemUsed, rec.SwapTotal, rec.SwapUsed,
		rec.DiskUsage, rec.Uptime, rec.ProcessCount,
		rec.TCPConns, rec.UDPConns, rec.PingData,
		rec.Offline, rec.SampleCount, rec.OfflineSamples,
	).Error
}

// GetByAgentAndTimeRange 查询指定 Agent 时间范围内的小时记录
func (r *HourlyRepository) GetByAgentAndTimeRange(agentID int64, startTime, endTime int64) ([]model.MetricRecordHourly, error) {
	var records []model.MetricRecordHourly
	err := r.db.Where("agent_id = ? AND timestamp >= ? AND timestamp <= ?", agentID, startTime, endTime).
		Order("timestamp ASC").
		Find(&records).Error
	if err != nil {
		return nil, err
	}
	return records, nil
}

// GetLastTimestamp 获取 Agent 最新一条小时记录的时间戳
// 用于 rollup 增量起点；无记录时返回 (0, false)
func (r *HourlyRepository) GetLastTimestamp(agentID int64) (int64, bool, error) {
	// MAX() 在无记录时返回 NULL，必须用 NullInt64 承接，否则 Scan 报错
	var ts sql.NullInt64
	err := r.db.Model(&model.MetricRecordHourly{}).
		Where("agent_id = ?", agentID).
		Select("MAX(timestamp)").
		Scan(&ts).Error
	if err != nil {
		return 0, false, err
	}
	return ts.Int64, ts.Valid && ts.Int64 > 0, nil
}

// DeleteOlderThan 删除指定时间之前的小时数据
func (r *HourlyRepository) DeleteOlderThan(before int64) (int64, error) {
	result := r.db.Where("timestamp < ?", before).Delete(&model.MetricRecordHourly{})
	return result.RowsAffected, result.Error
}

// CleanupExpired 清理过期小时数据（retentionDays <= 0 时取默认 730 天）
func (r *HourlyRepository) CleanupExpired(retentionDays int) (int64, error) {
	if retentionDays <= 0 {
		retentionDays = 730
	}
	cutoff := time.Now().AddDate(0, 0, -retentionDays).Unix()
	deleted, err := r.DeleteOlderThan(cutoff)
	if err != nil {
		return 0, fmt.Errorf("清理过期小时数据失败: %w", err)
	}
	return deleted, nil
}

// DeleteByAgentID 删除指定 Agent 的所有小时记录（Agent 删除时联动）
func (r *HourlyRepository) DeleteByAgentID(agentID int64) error {
	return r.db.Where("agent_id = ?", agentID).Delete(&model.MetricRecordHourly{}).Error
}
