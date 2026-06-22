package repository

import (
	"sync"

	sharedmodel "github.com/server-probe/shared/model"
)

// MetricPoint 实时监控数据点
type MetricPoint struct {
	Timestamp    int64                       // Unix 时间戳
	CPU          float64                     // CPU 使用率
	Mem          float64                     // 内存使用率
	MemTotal     uint64                      // 内存总量
	MemUsed      uint64                      // 内存已用
	SwapTotal    uint64                      // Swap 总量
	SwapUsed     uint64                      // Swap 已用
	Disks        []sharedmodel.DiskInfo      // 磁盘信息
	NetRx        uint64                      // 下行速率
	NetTx        uint64                      // 上行速率
	TCPConns     int                         // TCP 连接数
	UDPConns     int                         // UDP 连接数
	Load1        float64                     // 1 分钟负载
	Load5        float64                     // 5 分钟负载
	Load15       float64                     // 15 分钟负载
	Uptime       uint64                      // 运行时间
	ProcessCount int                         // 进程数
	PingData     []sharedmodel.PingResult    // Ping 探测结果
}

// RingBuffer 内存环形缓冲
// 每个 Agent 一个实例，用于存储实时监控数据
type RingBuffer struct {
	mu       sync.RWMutex
	data     []MetricPoint
	capacity int
	size     int
	head     int // 下一个写入位置
}

// NewRingBuffer 创建环形缓冲
func NewRingBuffer(capacity int) *RingBuffer {
	return &RingBuffer{
		data:     make([]MetricPoint, capacity),
		capacity: capacity,
		size:     0,
		head:     0,
	}
}

// Write 写入一个数据点
func (rb *RingBuffer) Write(point MetricPoint) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	rb.data[rb.head] = point
	rb.head = (rb.head + 1) % rb.capacity

	if rb.size < rb.capacity {
		rb.size++
	}
}

// Latest 获取最近 n 个数据点（最新的在前）
func (rb *RingBuffer) Latest(n int) []MetricPoint {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	if n <= 0 || rb.size == 0 {
		return []MetricPoint{}
	}

	if n > rb.size {
		n = rb.size
	}

	result := make([]MetricPoint, n)
	// head 指向下一个写入位置，所以最新的数据在 (head-1) % capacity
	for i := 0; i < n; i++ {
		idx := (rb.head - 1 - i + rb.capacity) % rb.capacity
		result[i] = rb.data[idx]
	}

	return result
}

// GetByTimeRange 获取指定时间范围内的数据点（按时间升序）
func (rb *RingBuffer) GetByTimeRange(startTime, endTime int64) []MetricPoint {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	if rb.size == 0 {
		return []MetricPoint{}
	}

	var result []MetricPoint

	// 从最旧的数据开始遍历
	for i := rb.size - 1; i >= 0; i-- {
		idx := (rb.head - 1 - i + rb.capacity) % rb.capacity
		point := rb.data[idx]
		if point.Timestamp >= startTime && point.Timestamp <= endTime {
			result = append(result, point)
		}
	}

	return result
}

// Size 返回当前数据点数量
func (rb *RingBuffer) Size() int {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	return rb.size
}

// Clear 清空缓冲
func (rb *RingBuffer) Clear() {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	rb.size = 0
	rb.head = 0
	rb.data = make([]MetricPoint, rb.capacity)
}

// GetAll 获取所有数据点（最新的在前）
func (rb *RingBuffer) GetAll() []MetricPoint {
	return rb.Latest(rb.size)
}

// UpdateLastPing 更新最新数据点的 PingData (不创建新数据点)
func (rb *RingBuffer) UpdateLastPing(pingData []sharedmodel.PingResult) bool {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if rb.size == 0 {
		return false
	}

	idx := (rb.head - 1 + rb.capacity) % rb.capacity
	rb.data[idx].PingData = pingData
	return true
}

// GetAvgCPU 获取最近 n 个数据点的平均 CPU 使用率
func (rb *RingBuffer) GetAvgCPU(n int) float64 {
	points := rb.Latest(n)
	if len(points) == 0 {
		return 0
	}

	var sum float64
	for _, p := range points {
		sum += p.CPU
	}
	return sum / float64(len(points))
}

// GetAvgMem 获取最近 n 个数据点的平均内存使用率
func (rb *RingBuffer) GetAvgMem(n int) float64 {
	points := rb.Latest(n)
	if len(points) == 0 {
		return 0
	}

	var sum float64
	for _, p := range points {
		sum += p.Mem
	}
	return sum / float64(len(points))
}
