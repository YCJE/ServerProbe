package repository

import (
	"fmt"
	"log"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/server-probe/server/internal/model"
)

// RecordRepository 历史聚合数据 CRUD
type RecordRepository struct {
	db          *gorm.DB
	batchWriter *BatchWriter // 批量写入缓冲器（可选，设置后 CreateRecord 走异步通道）
}

// NewRecordRepository 创建历史数据 repository
func NewRecordRepository(db *gorm.DB) *RecordRepository {
	return &RecordRepository{db: db}
}

// SetBatchWriter 设置批量写入缓冲器
// 设置后 CreateRecord 将数据推入 BatchWriter 的 channel，由后台 goroutine 异步批量写入
func (r *RecordRepository) SetBatchWriter(bw *BatchWriter) {
	r.batchWriter = bw
}

// Create 创建历史记录（直接写入数据库）
func (r *RecordRepository) Create(record *model.MetricRecord) error {
	return r.db.Create(record).Error
}

// CreateRecord 创建单条历史记录
// 如果已设置 BatchWriter，则推入异步缓冲 channel；否则直接写入数据库
func (r *RecordRepository) CreateRecord(record *model.MetricRecord) error {
	if r.batchWriter != nil {
		r.batchWriter.Submit(*record)
		return nil
	}
	return r.db.Create(record).Error
}

// FlushBatch 执行实际批量 INSERT（由 BatchWriter 调用）
// 使用多值 INSERT 语法（INSERT INTO ... VALUES (...), (...), ...）
// 注意 SQLite 单条 SQL 参数限制（999 个），每批最多 50 行（50 × 19 = 950 < 999）
func (r *RecordRepository) FlushBatch(records []model.MetricRecord) error {
	if len(records) == 0 {
		return nil
	}

	const fieldsPerRow = 19
	const maxRowsPerInsert = 50 // 50 × 19 = 950 < 999 SQLite 参数限制

	for start := 0; start < len(records); start += maxRowsPerInsert {
		end := start + maxRowsPerInsert
		if end > len(records) {
			end = len(records)
		}
		batch := records[start:end]

		// 构建多值 INSERT SQL
		var sqlBuilder strings.Builder
		sqlBuilder.WriteString("INSERT INTO metric_records (agent_id, timestamp, cpu_usage, mem_usage, mem_total, mem_used, swap_total, swap_used, disk_usage, net_rx, net_tx, tcp_connections, udp_connections, load_1, load_5, load_15, uptime, process_count, ping_data) VALUES ")

		args := make([]interface{}, 0, len(batch)*fieldsPerRow)
		for i, rec := range batch {
			if i > 0 {
				sqlBuilder.WriteString(", ")
			}
			sqlBuilder.WriteString("(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")
			args = append(args,
				rec.AgentID, rec.Timestamp, rec.CPUUsage, rec.MemUsage,
				rec.MemTotal, rec.MemUsed, rec.SwapTotal, rec.SwapUsed,
				rec.DiskUsage, rec.NetRx, rec.NetTx, rec.TCPConns, rec.UDPConns,
				rec.Load1, rec.Load5, rec.Load15, rec.Uptime,
				rec.ProcessCount, rec.PingData,
			)
		}

		if err := r.db.Exec(sqlBuilder.String(), args...).Error; err != nil {
			return fmt.Errorf("批量插入失败 (%d 条): %w", len(batch), err)
		}
	}

	return nil
}

// GetByAgentAndTimeRange 根据 Agent ID 和时间范围查询历史数据
func (r *RecordRepository) GetByAgentAndTimeRange(agentID int64, startTime, endTime int64) ([]model.MetricRecord, error) {
	var records []model.MetricRecord
	err := r.db.Where("agent_id = ? AND timestamp >= ? AND timestamp <= ?", agentID, startTime, endTime).
		Order("timestamp ASC").
		Find(&records).Error
	if err != nil {
		return nil, err
	}
	return records, nil
}

// DeleteOlderThan 删除指定时间之前的数据
func (r *RecordRepository) DeleteOlderThan(before int64) (int64, error) {
	result := r.db.Where("timestamp < ?", before).Delete(&model.MetricRecord{})
	return result.RowsAffected, result.Error
}

// DeleteByAgentID 删除指定 Agent 的所有历史聚合数据
func (r *RecordRepository) DeleteByAgentID(agentID int64) error {
	return r.db.Where("agent_id = ?", agentID).Delete(&model.MetricRecord{}).Error
}

// CleanupExpired 清理过期数据（默认保留 4 天，覆盖 3 天查询范围）
func (r *RecordRepository) CleanupExpired(retentionDays int) (int64, error) {
	if retentionDays <= 0 {
		retentionDays = 4 // 默认保留 4 天
	}
	cutoff := time.Now().AddDate(0, 0, -retentionDays).Unix()
	deleted, err := r.DeleteOlderThan(cutoff)
	if err != nil {
		return 0, fmt.Errorf("清理过期数据失败: %w", err)
	}
	return deleted, nil
}

// GetDBSize 获取数据库文件大小 (字节)
// 查询失败时返回 -1 并记录日志
func (r *RecordRepository) GetDBSize() int64 {
	var page_count int64
	if err := r.db.Raw("PRAGMA page_count").Scan(&page_count).Error; err != nil {
		log.Printf("查询 page_count 失败: %v", err)
		return -1
	}
	var page_size int64
	if err := r.db.Raw("PRAGMA page_size").Scan(&page_size).Error; err != nil {
		log.Printf("查询 page_size 失败: %v", err)
		return -1
	}
	return page_count * page_size
}

// AdminRepository 管理员账户 CRUD
type AdminRepository struct {
	db *gorm.DB
}

// NewAdminRepository 创建管理员 repository
func NewAdminRepository(db *gorm.DB) *AdminRepository {
	return &AdminRepository{db: db}
}

// Create 创建管理员
func (r *AdminRepository) Create(admin *model.Admin) error {
	return r.db.Create(admin).Error
}

// ErrAdminAlreadyExists 管理员账户已存在
var ErrAdminAlreadyExists = fmt.Errorf("管理员账户已存在")

// CreateFirstAdmin 在事务内检查是否已有管理员，若无则创建
// 防止 TOCTOU 竞态条件导致创建多个管理员
func (r *AdminRepository) CreateFirstAdmin(admin *model.Admin) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&model.Admin{}).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ErrAdminAlreadyExists
		}
		return tx.Create(admin).Error
	})
}

// GetByUsername 根据用户名获取管理员
func (r *AdminRepository) GetByUsername(username string) (*model.Admin, error) {
	var admin model.Admin
	if err := r.db.Where("username = ?", username).First(&admin).Error; err != nil {
		return nil, err
	}
	return &admin, nil
}

// GetByID 根据 ID 获取管理员
func (r *AdminRepository) GetByID(id int64) (*model.Admin, error) {
	var admin model.Admin
	if err := r.db.First(&admin, id).Error; err != nil {
		return nil, err
	}
	return &admin, nil
}

// Update 更新管理员
func (r *AdminRepository) Update(admin *model.Admin) error {
	return r.db.Save(admin).Error
}

// Count 统计管理员数量
func (r *AdminRepository) Count() (int64, error) {
	var count int64
	err := r.db.Model(&model.Admin{}).Count(&count).Error
	return count, err
}

// SharePageRepository 分享页 CRUD
type SharePageRepository struct {
	db *gorm.DB
}

// NewSharePageRepository 创建分享页 repository
func NewSharePageRepository(db *gorm.DB) *SharePageRepository {
	return &SharePageRepository{db: db}
}

// Create 创建分享页
func (r *SharePageRepository) Create(page *model.SharePage) error {
	return r.db.Create(page).Error
}

// GetByShareID 根据 share_id 获取分享页
func (r *SharePageRepository) GetByShareID(shareID string) (*model.SharePage, error) {
	var page model.SharePage
	if err := r.db.Where("share_id = ?", shareID).First(&page).Error; err != nil {
		return nil, err
	}
	return &page, nil
}

// List 获取所有分享页
func (r *SharePageRepository) List() ([]model.SharePage, error) {
	var pages []model.SharePage
	if err := r.db.Order("id ASC").Find(&pages).Error; err != nil {
		return nil, err
	}
	return pages, nil
}

// Delete 删除分享页
func (r *SharePageRepository) Delete(id int64) error {
	return r.db.Delete(&model.SharePage{}, id).Error
}
