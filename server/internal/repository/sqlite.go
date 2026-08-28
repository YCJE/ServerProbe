package repository

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/server-probe/server/internal/model"
)

// SQLiteDB SQLite 数据库管理
type SQLiteDB struct {
	db *gorm.DB
}

// NewSQLiteDB 创建 SQLite 连接并自动迁移表结构
func NewSQLiteDB(dataDir string) (*SQLiteDB, error) {
	// 确保数据目录存在（权限 0700，防止其他系统用户读取数据库与密钥）
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, fmt.Errorf("创建数据目录失败: %w", err)
	}
	// 修正已存在目录的权限（若此前以 0755 创建）
	if info, err := os.Stat(dataDir); err == nil && info.Mode().Perm() != 0700 {
		_ = os.Chmod(dataDir, 0700)
	}

	dbPath := filepath.Join(dataDir, "probe.db")

	// 打开 SQLite 连接，通过 DSN 参数设置 PRAGMA（确保重连后也生效）
	dsn := dbPath + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("打开 SQLite 失败: %w", err)
	}

	// 数据库文件权限设为 0600，防止其他系统用户读取敏感数据
	if info, statErr := os.Stat(dbPath); statErr == nil && info.Mode().Perm() != 0600 {
		_ = os.Chmod(dbPath, 0600)
	}

	// 获取 SQL DB 用于连接池配置
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("获取 SQL DB 失败: %w", err)
	}
	// 设置连接池
	sqlDB.SetMaxOpenConns(1) // SQLite 单写多读，限制连接数避免锁冲突
	sqlDB.SetMaxIdleConns(1)

	// 一次性迁移：检测 metric_records 是否使用旧 GORM 默认列名
	// （tcp_conns/udp_conns/load1/load5/load15），若存在则 DROP 重建
	// 以匹配 gorm:"column:xxx" 标签指定的新列名
	var oldColCount int64
	db.Raw("SELECT count(*) FROM pragma_table_info('metric_records') WHERE name IN ('tcp_conns', 'udp_conns', 'load1', 'load5', 'load15')").Scan(&oldColCount)
	if oldColCount > 0 {
		db.Exec("DROP TABLE IF EXISTS metric_records")
	}

	// 自动迁移表结构
	if err := db.AutoMigrate(
		&model.Agent{},
		&model.RegisterCode{},
		&model.AlertRule{},
		&model.AlertHistory{},
		&model.NotifyChannel{},
		&model.PingTarget{},
		&model.MetricRecord{},
		&model.MetricRecordHourly{},
		&model.Admin{},
		&model.AuditLog{},
		&model.SharePage{},
		&model.SystemSetting{},
		&model.TrafficRecord{},
		&model.ServiceMonitor{},
		&model.SSLCertMonitor{},
		&model.Tag{},
	); err != nil {
		return nil, fmt.Errorf("数据库迁移失败: %w", err)
	}

	log.Println("SQLite 数据库初始化完成")

	return &SQLiteDB{db: db}, nil
}

// DB 返回底层 gorm.DB 实例
func (s *SQLiteDB) DB() *gorm.DB {
	return s.db
}

// Close 关闭数据库连接
func (s *SQLiteDB) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
