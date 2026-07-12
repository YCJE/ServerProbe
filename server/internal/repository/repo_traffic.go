package repository

import (
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/server-probe/server/internal/model"
)

// TrafficRepository 每日流量统计 CRUD
type TrafficRepository struct {
	db *gorm.DB
}

// NewTrafficRepository 创建流量统计 repository
func NewTrafficRepository(db *gorm.DB) *TrafficRepository {
	return &TrafficRepository{db: db}
}

// UpsertDailyTraffic 累加指定 Agent 当日流量增量（SQLite ON CONFLICT 语义）
// 由于 GORM 的 clause.OnConflict 对 SQLite UPSERT 支持有限，这里直接使用原生 SQL
func (r *TrafficRepository) UpsertDailyTraffic(agentID int64, date string, rxDelta, txDelta uint64) error {
	sql := `INSERT INTO traffic_records (agent_id, date, rx_bytes, tx_bytes, updated_at)
VALUES (?, ?, ?, ?, datetime('now'))
ON CONFLICT(agent_id, date) DO UPDATE SET
	rx_bytes = rx_bytes + excluded.rx_bytes,
	tx_bytes = tx_bytes + excluded.tx_bytes,
	updated_at = datetime('now')`
	return r.db.Exec(sql, agentID, date, rxDelta, txDelta).Error
}

// GetDailyTraffic 查询指定 Agent 某日的流量记录
// 如果记录不存在，返回零值记录（非 error），便于上层直接展示
func (r *TrafficRepository) GetDailyTraffic(agentID int64, date string) (*model.TrafficRecord, error) {
	var record model.TrafficRecord
	if err := r.db.Where("agent_id = ? AND date = ?", agentID, date).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &model.TrafficRecord{
				AgentID: agentID,
				Date:    date,
			}, nil
		}
		return nil, err
	}
	return &record, nil
}

// GetMonthlyTraffic 查询指定 Agent 某月所有日记录
// date 字段为 "2006-01-02" 格式，利用字符串前缀比较实现日期范围查询
func (r *TrafficRepository) GetMonthlyTraffic(agentID int64, year, month int) ([]model.TrafficRecord, error) {
	// 计算月份起止日期（字符串比较）
	startDate := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	endDate := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC).AddDate(0, 1, 0).Format("2006-01-02")

	var records []model.TrafficRecord
	if err := r.db.Where("agent_id = ? AND date >= ? AND date < ?", agentID, startDate, endDate).
		Order("date ASC").Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

// GetTrafficByDateRange 查询指定 Agent 在日期范围内的流量记录
func (r *TrafficRepository) GetTrafficByDateRange(agentID int64, startDate, endDate string) ([]model.TrafficRecord, error) {
	var records []model.TrafficRecord
	if err := r.db.Where("agent_id = ? AND date >= ? AND date <= ?", agentID, startDate, endDate).
		Order("date ASC").Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

// GetAllTrafficForDate 查询所有 Agent 某日的流量记录
func (r *TrafficRepository) GetAllTrafficForDate(date string) ([]model.TrafficRecord, error) {
	var records []model.TrafficRecord
	if err := r.db.Where("date = ?", date).Order("agent_id ASC").Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

// CleanupExpired 清理指定天数之前的流量记录，返回删除的行数
func (r *TrafficRepository) CleanupExpired(days int) (int64, error) {
	cutoffDate := time.Now().AddDate(0, 0, -days).Format("2006-01-02")
	result := r.db.Where("date < ?", cutoffDate).Delete(&model.TrafficRecord{})
	return result.RowsAffected, result.Error
}
