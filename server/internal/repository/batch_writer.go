package repository

import (
	"log"
	"sync"
	"time"

	"github.com/server-probe/server/internal/model"
)

// BatchWriter 批量写入缓冲器
// 使用 channel + 后台 goroutine 实现异步批量写入
// 每 flushInterval 或累积 batchSize 条触发一次批量写入
type BatchWriter struct {
	ch            chan model.MetricRecord // 数据缓冲 channel
	flushFn       func([]model.MetricRecord) error
	flushInterval time.Duration // 定时 flush 间隔
	batchSize     int           // 触发批量写入的条数阈值
	stopCh        chan struct{}
	stopOnce      sync.Once
	wg            sync.WaitGroup
}

// NewBatchWriter 创建批量写入缓冲器
// flushFn 为实际执行批量写入的回调函数
func NewBatchWriter(flushFn func([]model.MetricRecord) error) *BatchWriter {
	return &BatchWriter{
		ch:            make(chan model.MetricRecord, 10000), // channel 容量 10000
		flushFn:       flushFn,
		flushInterval: 500 * time.Millisecond, // 每 500ms 触发一次
		batchSize:     100,                    // 累积 100 条触发一次
		stopCh:        make(chan struct{}),
	}
}

// Start 启动后台写入 goroutine
func (bw *BatchWriter) Start() {
	bw.wg.Add(1)
	go bw.run()
}

// run 后台 goroutine 主循环
func (bw *BatchWriter) run() {
	defer bw.wg.Done()

	ticker := time.NewTicker(bw.flushInterval)
	defer ticker.Stop()

	// 预分配 batch slice，避免频繁扩容
	batch := make([]model.MetricRecord, 0, bw.batchSize)

	// flush 执行批量写入并清空 batch
	// flush 失败时保留数据并重试，最多重试 3 次，每次间隔 100ms
	// 3 次都失败才丢弃并记录日志
	flush := func() {
		if len(batch) == 0 {
			return
		}
		// 深拷贝 records，避免与 batch 共享底层数组
		// batch = batch[:0] 后 records 仍持有独立副本，防止后续 append 覆盖 flushFn 中的数据
		records := make([]model.MetricRecord, len(batch))
		copy(records, batch)
		for attempt := 1; attempt <= 3; attempt++ {
			if err := bw.flushFn(records); err != nil {
				log.Printf("[BatchWriter] 批量写入失败 (第 %d 次, %d 条): %v", attempt, len(records), err)
				if attempt < 3 {
					time.Sleep(100 * time.Millisecond)
					continue
				}
				// 3 次都失败，丢弃数据并记录日志
				log.Printf("[BatchWriter] 批量写入重试 3 次均失败，丢弃 %d 条数据", len(records))
			}
			break
		}
		// 重置 batch，保留底层数组
		batch = batch[:0]
	}

	for {
		select {
		case record := <-bw.ch:
			batch = append(batch, record)
			// 达到批量写入阈值，立即 flush
			if len(batch) >= bw.batchSize {
				flush()
			}
		case <-ticker.C:
			// 定时 flush，确保低频数据也能及时写入
			flush()
		case <-bw.stopCh:
			// 优雅关闭：drain channel 中剩余数据
			for {
				select {
				case record := <-bw.ch:
					batch = append(batch, record)
				default:
					flush()
					log.Printf("[BatchWriter] 已停止，剩余数据已 flush")
					return
				}
			}
		}
	}
}

// Submit 提交一条记录到缓冲 channel
// 非阻塞：channel 满时丢弃并记录日志，避免阻塞调用方
func (bw *BatchWriter) Submit(record model.MetricRecord) {
	select {
	case bw.ch <- record:
	default:
		log.Printf("[BatchWriter] channel 已满，丢弃记录 (agent_id=%d, timestamp=%d)",
			record.AgentID, record.Timestamp)
	}
}

// FlushAndShutdown 优雅关闭：通知后台 goroutine 退出并等待剩余数据 flush 完成
func (bw *BatchWriter) FlushAndShutdown() {
	bw.stopOnce.Do(func() { close(bw.stopCh) })
	bw.wg.Wait()
}
