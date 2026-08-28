package service

import (
	"log"
	"sync"
	"time"

	"github.com/server-probe/server/internal/model"
	"github.com/server-probe/server/internal/repository"
)

// 审计日志保留策略：默认保留 180 天，每日清理一次
const (
	auditRetentionDays = 180
	auditCleanupPeriod = 24 * time.Hour
	auditBufferSize    = 256
)

// AuditService 审计日志服务
// 写入走内存缓冲 channel，由后台 goroutine 批量落库：
// HTTP 处理路径零阻塞，缓冲满时丢弃并记日志（审计尽力而为，不拖垮业务）
type AuditService struct {
	repo  *repository.AuditLogRepository
	ch    chan model.AuditLog
	wg    sync.WaitGroup
	stop  chan struct{}
	once  sync.Once
}

// NewAuditService 创建审计服务（repo 为 nil 时所有写入为空操作，便于测试）
func NewAuditService(repo *repository.AuditLogRepository) *AuditService {
	return &AuditService{
		repo: repo,
		ch:   make(chan model.AuditLog, auditBufferSize),
		stop: make(chan struct{}),
	}
}

// Start 启动后台写入与每日清理 goroutine
func (s *AuditService) Start() {
	s.wg.Add(2)
	go s.writeLoop()
	go s.cleanupLoop()
	log.Printf("审计服务已启动（保留 %d 天，缓冲容量 %d）", auditRetentionDays, auditBufferSize)
}

// Stop 停止服务，flush 缓冲中剩余的日志
func (s *AuditService) Stop() {
	s.once.Do(func() { close(s.stop) })
	s.wg.Wait()
}

// Record 提交一条审计日志（非阻塞；缓冲满时丢弃）
func (s *AuditService) Record(entry model.AuditLog) {
	if s.repo == nil {
		return
	}
	select {
	case s.ch <- entry:
	default:
		log.Printf("警告: 审计日志缓冲已满，丢弃一条（action=%s）", entry.Action)
	}
}

// writeLoop 后台消费缓冲并写入数据库
func (s *AuditService) writeLoop() {
	defer s.wg.Done()
	for {
		select {
		case entry := <-s.ch:
			if err := s.repo.Create(&entry); err != nil {
				log.Printf("审计日志写入失败（action=%s）: %v", entry.Action, err)
			}
		case <-s.stop:
			// 优雅关闭：drain 缓冲中剩余条目
			for {
				select {
				case entry := <-s.ch:
					if err := s.repo.Create(&entry); err != nil {
						log.Printf("审计日志写入失败（action=%s）: %v", entry.Action, err)
					}
				default:
					return
				}
			}
		}
	}
}

// cleanupLoop 每日清理过期审计日志
func (s *AuditService) cleanupLoop() {
	defer s.wg.Done()
	ticker := time.NewTicker(auditCleanupPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			before := time.Now().AddDate(0, 0, -auditRetentionDays)
			if deleted, err := s.repo.CleanupOlderThan(before); err != nil {
				log.Printf("审计日志清理失败: %v", err)
			} else if deleted > 0 {
				log.Printf("已清理 %d 条过期审计日志（早于 %s）", deleted, before.Format(time.RFC3339))
			}
		case <-s.stop:
			return
		}
	}
}
