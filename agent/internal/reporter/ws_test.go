package reporter

import (
	"sync"
	"testing"
	"time"
)

// TestGetReconnectInterval_JitterBounds 验证每次重连间隔都在
// [base×0.8, base×1.2] 范围内且不超过上限（封顶在抖动之后应用）
func TestGetReconnectInterval_JitterBounds(t *testing.T) {
	for i := 0; i < 200; i++ {
		// 每次 getReconnectInterval 调用都会递增 attempts，
		// 因此每个 attempt 级别用独立客户端验证边界
		cc := &WSClient{maxReconnectInterval: 60 * time.Second, stopCh: make(chan struct{})}
		cc.mu.Lock()
		cc.reconnectAttempts = i % 10 // 限制在移位上限内
		cc.mu.Unlock()
		interval := cc.getReconnectInterval()

		attempts := i%10 + 1
		expectedBase := time.Duration(5*(1<<(attempts-1))) * time.Second
		lo := time.Duration(float64(expectedBase) * 0.8)
		hi := time.Duration(float64(expectedBase) * 1.2)
		capInterval := cc.maxReconnectInterval

		// 封顶在抖动之后应用：
		// - hi <= cap：间隔 ∈ [lo, hi]
		// - lo <= cap < hi：间隔 ∈ [lo, cap]
		// - cap < lo：间隔恒为 cap
		if hi > capInterval {
			hi = capInterval
		}
		if lo > capInterval {
			lo = capInterval
		}

		if interval < lo {
			t.Fatalf("attempt %d: interval %v < 下界 %v", attempts, interval, lo)
		}
		if interval > hi {
			t.Fatalf("attempt %d: interval %v > 上界 %v", attempts, interval, hi)
		}
	}
}

// TestGetReconnectInterval_CapAt60s 验证退避指数增长后间隔封顶在 maxReconnectInterval
func TestGetReconnectInterval_CapAt60s(t *testing.T) {
	c := &WSClient{maxReconnectInterval: 60 * time.Second, stopCh: make(chan struct{})}

	// 连续调用 20 次（attempts 5 起理论 base ≥ 80s > 60s）
	for i := 0; i < 20; i++ {
		interval := c.getReconnectInterval()
		if interval > 60*time.Second {
			t.Fatalf("第 %d 次重连间隔 %v 超过封顶 60s", i+1, interval)
		}
		if interval <= 0 {
			t.Fatalf("第 %d 次重连间隔非法: %v", i+1, interval)
		}
	}
}

// TestGetReconnectInterval_ExponentialGrowth 验证前几次退避按 5/10/20/40s 指数增长
func TestGetReconnectInterval_ExponentialGrowth(t *testing.T) {
	// 多次采样取最小值：采样最小值仍应落在 [base×0.8, base×1.2] 内，
	// 且 attempt N+1 的采样最小值应大于 attempt N 的采样最小值减去抖动余量
	sampleMin := func(attempt int) time.Duration {
		minVal := time.Duration(1<<62)
		for i := 0; i < 50; i++ {
			c := &WSClient{maxReconnectInterval: 60 * time.Second, stopCh: make(chan struct{})}
			c.mu.Lock()
			c.reconnectAttempts = attempt - 1
			c.mu.Unlock()
			v := c.getReconnectInterval()
			if v < minVal {
				minVal = v
			}
		}
		return minVal
	}

	// attempt 1: base 5s → jitter 后最低 4s
	// attempt 2: base 10s → jitter 后最低 8s
	if got := sampleMin(1); got > 6*time.Second {
		t.Errorf("attempt 1 采样最小值 %v，明显偏离 5s 基准", got)
	}
	if got := sampleMin(2); got > 12*time.Second {
		t.Errorf("attempt 2 采样最小值 %v，明显偏离 10s 基准", got)
	}
	if got := sampleMin(3); got > 24*time.Second {
		t.Errorf("attempt 3 采样最小值 %v，明显偏离 20s 基准", got)
	}
}

// TestGetReconnectInterval_AttemptsIncrement 验证每次调用递增重试计数
func TestGetReconnectInterval_AttemptsIncrement(t *testing.T) {
	c := &WSClient{maxReconnectInterval: 60 * time.Second, stopCh: make(chan struct{})}

	c.getReconnectInterval()
	c.getReconnectInterval()
	c.getReconnectInterval()

	c.mu.Lock()
	attempts := c.reconnectAttempts
	c.mu.Unlock()

	if attempts != 3 {
		t.Errorf("reconnectAttempts = %d, want 3", attempts)
	}
}

// TestGetReconnectInterval_NoOverflow 验证大量重试后不发生整数溢出（间隔恒为正）
func TestGetReconnectInterval_NoOverflow(t *testing.T) {
	c := &WSClient{maxReconnectInterval: 60 * time.Second, stopCh: make(chan struct{})}

	// 模拟 1000 次连续失败重连
	var wg sync.WaitGroup
	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if v := c.getReconnectInterval(); v <= 0 {
				t.Errorf("重连间隔非法: %v", v)
			}
		}()
	}
	wg.Wait()
}
