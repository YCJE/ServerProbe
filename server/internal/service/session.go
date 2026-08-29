package service

import (
	"log"
	"sync"
	"time"

	"github.com/server-probe/server/internal/repository"
)

// 会话清理策略：每日清理过期或已撤销超过 7 天的会话记录
const sessionCleanupPeriod = 24 * time.Hour

// SessionService 会话维护服务（每日清理过期/已撤销会话）
type SessionService struct {
	repo *repository.SessionRepository
	wg   sync.WaitGroup
	stop chan struct{}
	once sync.Once
}

// NewSessionService 创建会话服务（repo 为 nil 时不启动清理，便于测试）
func NewSessionService(repo *repository.SessionRepository) *SessionService {
	return &SessionService{repo: repo, stop: make(chan struct{})}
}

// Start 启动每日清理 goroutine
func (s *SessionService) Start() {
	if s.repo == nil {
		return
	}
	s.wg.Add(1)
	go s.cleanupLoop()
	log.Println("会话清理任务已启动（每日一次，保留 7 天撤销/过期记录）")
}

// Stop 停止清理任务
func (s *SessionService) Stop() {
	s.once.Do(func() { close(s.stop) })
	s.wg.Wait()
}

// CleanupOnce 立即执行一次清理（启动时调用，也供测试使用）
func (s *SessionService) CleanupOnce() {
	if s.repo == nil {
		return
	}
	if deleted, err := s.repo.CleanupExpired(); err != nil {
		log.Printf("会话清理失败: %v", err)
	} else if deleted > 0 {
		log.Printf("已清理 %d 条过期会话记录", deleted)
	}
}

func (s *SessionService) cleanupLoop() {
	defer s.wg.Done()
	ticker := time.NewTicker(sessionCleanupPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.CleanupOnce()
		case <-s.stop:
			return
		}
	}
}
