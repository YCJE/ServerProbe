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
	// 确保数据目录存在
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("创建数据目录失败: %w", err)
	}

	dbPath := filepath.Join(dataDir, "probe.db")

	// 打开 SQLite 连接
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("打开 SQLite 失败: %w", err)
	}

	// 启用 WAL 模式，提高并发读写性能
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("获取 SQL DB 失败: %w", err)
	}
	if _, err := sqlDB.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		log.Printf("警告: 设置 WAL 模式失败: %v", err)
	}
	if _, err := sqlDB.Exec("PRAGMA busy_timeout=5000;"); err != nil {
		log.Printf("警告: 设置 busy_timeout 失败: %v", err)
	}
	// 设置连接池
	sqlDB.SetMaxOpenConns(1) // SQLite 单写多读，限制连接数避免锁冲突
	sqlDB.SetMaxIdleConns(1)

	// 自动迁移表结构
	if err := db.AutoMigrate(
		&model.Agent{},
		&model.RegisterCode{},
		&model.AlertRule{},
		&model.NotifyChannel{},
		&model.PingTarget{},
		&model.MetricRecord{},
		&model.Admin{},
		&model.SharePage{},
		&model.SystemSetting{},
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
