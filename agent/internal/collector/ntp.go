package collector

import (
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// ntpEpochOffset 是 NTP 纪元（1900-01-01）与 Unix 纪元（1970-01-01）之间的秒数差
const ntpEpochOffset = 2208988800

// ntpPacketSize NTP 协议包大小（48 字节）
const ntpPacketSize = 48

// ntpTimeout NTP 查询超时时间
const ntpTimeout = 3 * time.Second

// defaultNTPServer 默认 NTP 服务器
const defaultNTPServer = "pool.ntp.org:123"

// NTPCollector NTP 时间偏移采集器
// 启动时向 NTP 服务器查询时间偏移，之后缓存结果
// 查询失败时会定期重试（后台 ticker 每 5 分钟一次，Collect 时也会触发异步重试）
type NTPCollector struct {
	ntpServer string
	offset    atomic.Int64 // 缓存的时间偏移（毫秒）
	queried   atomic.Bool  // 是否已成功查询
	querying  atomic.Bool  // 是否正在查询中（防止并发查询）
	stopCh    chan struct{}
	stopOnce  sync.Once
}

// NewNTPCollector 创建 NTP 采集器
// ntpServer 为空时使用默认服务器 "pool.ntp.org:123"
// 查询在后台异步执行，不阻塞构造函数
func NewNTPCollector(ntpServer string) *NTPCollector {
	if ntpServer == "" {
		ntpServer = defaultNTPServer
	}
	c := &NTPCollector{
		ntpServer: ntpServer,
		stopCh:    make(chan struct{}),
	}
	// 异步查询 NTP，不阻塞 Agent 启动
	go c.queryOnce()
	// 启动后台 ticker，每 5 分钟重试一次（失败时重试，成功时刷新）
	go c.retryLoop()
	return c
}

// Name 返回采集器名称
func (c *NTPCollector) Name() string {
	return "ntp"
}

// Collect 返回缓存的时间偏移（毫秒）
// 直接返回当前 offset，不触发异步查询。
// 初始化时 constructor 已调用 queryOnce，retryLoop 每 5 分钟负责重试/刷新。
func (c *NTPCollector) Collect() (interface{}, error) {
	return c.offset.Load(), nil
}

// queryOnce 执行一次 NTP 查询并缓存结果
// 使用 querying 标志防止并发查询
// 查询失败时 queried 保持 false，以便后续重试
func (c *NTPCollector) queryOnce() {
	// 防止并发查询
	if !c.querying.CompareAndSwap(false, true) {
		return
	}
	defer c.querying.Store(false)

	offset, err := c.queryNTPServer()
	if err != nil {
		// 查询失败时保留上次成功的缓存偏移量，不清零。
		// queried 保持 false（若尚未成功过），以便 retryLoop 后续重试。
		// 若之前已成功查询过（queried 为 true），offset 保留上次的值。
		return
	}
	c.offset.Store(offset)
	c.queried.Store(true)
}

// retryLoop 后台定期重试 NTP 查询
// 每 5 分钟触发一次查询（失败时重试，成功时刷新偏移量）
func (c *NTPCollector) retryLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			go c.queryOnce()
		case <-c.stopCh:
			return
		}
	}
}

// Stop 停止后台重试 goroutine
func (c *NTPCollector) Stop() {
	c.stopOnce.Do(func() { close(c.stopCh) })
}

// queryNTPServer 向 NTP 服务器发送查询并计算时间偏移
// 返回偏移量（毫秒），正数表示本地时钟领先，负数表示落后
func (c *NTPCollector) queryNTPServer() (int64, error) {
	// 建立 UDP 连接
	conn, err := net.DialTimeout("udp", c.ntpServer, ntpTimeout)
	if err != nil {
		return 0, fmt.Errorf("连接 NTP 服务器失败: %w", err)
	}
	defer conn.Close()

	// 设置读写超时
	conn.SetDeadline(time.Now().Add(ntpTimeout))

	// 构造 NTP v4 客户端请求包（48 字节）
	// 第一个字节: LI(2bit)=0 | VN(3bit)=4 | Mode(3bit)=3 = 0x1B
	request := make([]byte, ntpPacketSize)
	request[0] = 0x1B

	// 记录发送时间 T1（客户端时钟）
	t1 := time.Now()

	// 发送请求
	if _, err := conn.Write(request); err != nil {
		return 0, fmt.Errorf("发送 NTP 请求失败: %w", err)
	}

	// 读取响应
	response := make([]byte, ntpPacketSize)
	if _, err := conn.Read(response); err != nil {
		return 0, fmt.Errorf("读取 NTP 响应失败: %w", err)
	}

	// 记录接收时间 T4（客户端时钟）
	t4 := time.Now()

	// 解析服务器响应中的时间戳
	// T2 = Receive Timestamp（服务器接收请求的时间，bytes 32-39）
	// T3 = Transmit Timestamp（服务器发送响应的时间，bytes 40-47）
	t2 := ntpToTime(binary.BigEndian.Uint64(response[32:40]))
	t3 := ntpToTime(binary.BigEndian.Uint64(response[40:48]))

	// 计算时间偏移
	// offset = ((T2 - T1) + (T3 - T4)) / 2
	// 正值表示本地时钟领先于服务器
	offset := ((t2.Sub(t1)) + (t3.Sub(t4))) / 2

	return offset.Milliseconds(), nil
}

// ntpToTime 将 NTP 时间戳（64 位定点数）转换为 time.Time
// NTP 时间戳: 高 32 位为秒数（自 1900-01-01），低 32 位为小数部分
func ntpToTime(ntpTime uint64) time.Time {
	seconds := int64(ntpTime >> 32)
	fraction := float64(ntpTime&0xFFFFFFFF) / float64(1<<32)

	// 转换为 Unix 时间戳
	unixSeconds := seconds - ntpEpochOffset

	return time.Unix(unixSeconds, int64(fraction*1e9))
}
