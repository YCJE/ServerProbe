package service

import (
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"
)

// LogLevel 日志级别
type LogLevel string

const (
	LogLevelInfo    LogLevel = "INFO"
	LogLevelWarning LogLevel = "WARNING"
	LogLevelError   LogLevel = "ERROR"
	LogLevelDebug   LogLevel = "DEBUG"
	LogLevelAll     LogLevel = "ALL"
)

// LogEntry 日志条目
type LogEntry struct {
	Timestamp time.Time  `json:"timestamp"`
	Level     LogLevel   `json:"level"`
	Message   string     `json:"message"`
}

// LogCapture 日志捕获服务
// 通过 io.Writer 接口拦截标准 log 包的输出，存储到环形缓冲区
type LogCapture struct {
	mu       sync.RWMutex
	entries  []LogEntry
	head     int // 下一个写入位置
	size     int // 当前条目数
	capacity int // 最大容量
}

// NewLogCapture 创建日志捕获服务
func NewLogCapture(capacity int) *LogCapture {
	return &LogCapture{
		entries:  make([]LogEntry, capacity),
		capacity: capacity,
	}
}

// Write 实现 io.Writer 接口，解析日志级别并存储
func (lc *LogCapture) Write(p []byte) (int, error) {
	msg := strings.TrimSpace(string(p))
	if msg == "" {
		return len(p), nil
	}

	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     lc.parseLevel(msg),
		Message:   msg,
	}

	lc.mu.Lock()
	lc.entries[lc.head] = entry
	lc.head = (lc.head + 1) % lc.capacity
	if lc.size < lc.capacity {
		lc.size++
	}
	lc.mu.Unlock()

	return len(p), nil
}

// parseLevel 从日志消息中推断级别
// 标准 log 包不自带级别，通过关键词推断
func (lc *LogCapture) parseLevel(msg string) LogLevel {
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "error") || strings.Contains(lower, "失败") || strings.Contains(lower, "failed"):
		return LogLevelError
	case strings.Contains(lower, "warn") || strings.Contains(lower, "警告"):
		return LogLevelWarning
	case strings.Contains(lower, "debug"):
		return LogLevelDebug
	default:
		return LogLevelInfo
	}
}

// GetLogs 获取日志条目（按时间倒序，最新的在前）
// level: 过滤级别 ("ALL" 表示全部)
// limit: 返回条数上限
// search: 关键词过滤（空字符串表示不过滤）
func (lc *LogCapture) GetLogs(level string, limit int, search string) []LogEntry {
	lc.mu.RLock()
	defer lc.mu.RUnlock()

	if limit <= 0 || limit > lc.size {
		limit = lc.size
	}

	var result []LogEntry
	searchLower := strings.ToLower(search)

	// 从最新到最旧遍历
	for i := 0; i < lc.size && len(result) < limit; i++ {
		idx := (lc.head - 1 - i + lc.capacity) % lc.capacity
		entry := lc.entries[idx]

		// 级别过滤
		if level != "" && level != string(LogLevelAll) && string(entry.Level) != level {
			continue
		}

		// 关键词过滤
		if searchLower != "" && !strings.Contains(strings.ToLower(entry.Message), searchLower) {
			continue
		}

		result = append(result, entry)
	}

	if result == nil {
		result = []LogEntry{}
	}
	return result
}

// GetLogCount 返回当前存储的日志条目数
func (lc *LogCapture) GetLogCount() int {
	lc.mu.RLock()
	defer lc.mu.RUnlock()
	return lc.size
}

// Install 安装日志捕获：将标准 log 输出重定向到 MultiWriter
// 同时输出到原始目标（stdout）和日志捕获服务
func (lc *LogCapture) Install() {
	originalOutput := logWriterOutput
	log.SetOutput(io.MultiWriter(originalOutput, lc))
}

// logWriterOutput 保存 log 包的原始输出目标
var logWriterOutput io.Writer = os.Stderr
