package repository

import (
	"sync"
	"testing"
	"time"

	sharedmodel "github.com/server-probe/shared/model"
)

func TestRingBuffer_New(t *testing.T) {
	rb := NewRingBuffer(100)
	if rb == nil {
		t.Fatal("创建环形缓冲失败")
	}
	if rb.capacity != 100 {
		t.Errorf("容量错误: 期望 100, 得到 %d", rb.capacity)
	}
	if rb.size != 0 {
		t.Errorf("初始大小应为 0, 得到 %d", rb.size)
	}
}

func TestRingBuffer_WriteAndRead(t *testing.T) {
	rb := NewRingBuffer(10)

	// 写入一个数据点
	point := MetricPoint{
		Timestamp: time.Now().Unix(),
		CPU:       45.2,
		Mem:       60.0,
	}
	rb.Write(point)

	// 读取
	points := rb.Latest(1)
	if len(points) != 1 {
		t.Fatalf("读取数量错误: 期望 1, 得到 %d", len(points))
	}
	if points[0].CPU != 45.2 {
		t.Errorf("CPU 值错误: 期望 45.2, 得到 %f", points[0].CPU)
	}
}

func TestRingBuffer_Overwrite(t *testing.T) {
	rb := NewRingBuffer(3)

	// 写入 5 个数据点，容量为 3，应覆盖前 2 个
	for i := 0; i < 5; i++ {
		rb.Write(MetricPoint{
			Timestamp: int64(i),
			CPU:       float64(i),
		})
	}

	// 应该只有 3 个数据点
	if rb.size != 3 {
		t.Errorf("大小错误: 期望 3, 得到 %d", rb.size)
	}

	// 读取所有数据，应该是最后 3 个（2, 3, 4）
	points := rb.Latest(3)
	if len(points) != 3 {
		t.Fatalf("读取数量错误: 期望 3, 得到 %d", len(points))
	}

	// 验证顺序（最新的在前）
	if points[0].CPU != 4 {
		t.Errorf("第一个数据应为最新的(CPU=4), 得到 %f", points[0].CPU)
	}
	if points[1].CPU != 3 {
		t.Errorf("第二个数据 CPU=3, 得到 %f", points[1].CPU)
	}
	if points[2].CPU != 2 {
		t.Errorf("第三个数据 CPU=2, 得到 %f", points[2].CPU)
	}
}

func TestRingBuffer_LatestMoreThanSize(t *testing.T) {
	rb := NewRingBuffer(10)

	// 写入 3 个数据点
	for i := 0; i < 3; i++ {
		rb.Write(MetricPoint{
			Timestamp: int64(i),
			CPU:       float64(i),
		})
	}

	// 请求 5 个，但只有 3 个
	points := rb.Latest(5)
	if len(points) != 3 {
		t.Errorf("读取数量错误: 期望 3, 得到 %d", len(points))
	}
}

func TestRingBuffer_LatestZero(t *testing.T) {
	rb := NewRingBuffer(10)

	// 请求 0 个
	points := rb.Latest(0)
	if len(points) != 0 {
		t.Errorf("请求 0 个应返回空, 得到 %d", len(points))
	}
}

func TestRingBuffer_Empty(t *testing.T) {
	rb := NewRingBuffer(10)

	points := rb.Latest(5)
	if len(points) != 0 {
		t.Errorf("空缓冲应返回空, 得到 %d", len(points))
	}
}

func TestRingBuffer_GetByTimeRange(t *testing.T) {
	rb := NewRingBuffer(100)

	// 写入带时间戳的数据
	for i := 1000; i < 1010; i++ {
		rb.Write(MetricPoint{
			Timestamp: int64(i),
			CPU:       float64(i - 1000),
		})
	}

	// 查询时间范围 1003-1007
	points := rb.GetByTimeRange(1003, 1007)
	if len(points) != 5 {
		t.Fatalf("时间范围查询数量错误: 期望 5, 得到 %d", len(points))
	}

	// 验证第一个点
	if points[0].Timestamp != 1003 {
		t.Errorf("第一个时间戳错误: 期望 1003, 得到 %d", points[0].Timestamp)
	}
}

func TestRingBuffer_ConcurrentWrite(t *testing.T) {
	rb := NewRingBuffer(1000)
	var wg sync.WaitGroup

	// 100 个 goroutine 并发写入
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				rb.Write(MetricPoint{
					Timestamp: int64(n*100 + j),
					CPU:       float64(j),
				})
			}
		}(i)
	}

	wg.Wait()

	// 应该有 10000 个数据点，但容量 1000
	if rb.size != 1000 {
		t.Errorf("并发写入后大小错误: 期望 1000, 得到 %d", rb.size)
	}

	// 读取不应 panic
	points := rb.Latest(100)
	if len(points) != 100 {
		t.Errorf("并发读取数量错误: 期望 100, 得到 %d", len(points))
	}
}

func TestRingBuffer_ConcurrentReadWrite(t *testing.T) {
	rb := NewRingBuffer(100)
	var wg sync.WaitGroup

	// 并发写入
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			rb.Write(MetricPoint{
				Timestamp: int64(n),
				CPU:       float64(n),
			})
		}(i)
	}

	// 并发读取
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = rb.Latest(10)
		}()
	}

	wg.Wait()
}

func TestRingBuffer_PingData(t *testing.T) {
	rb := NewRingBuffer(60)

	pingResults := []sharedmodel.PingResult{
		{Target: "114.114.114.114", Name: "电信", AvgLatency: 12.5, Loss: 0},
		{Target: "223.5.5.5", Name: "移动", AvgLatency: 8.3, Loss: 10},
	}

	rb.Write(MetricPoint{
		Timestamp: time.Now().Unix(),
		PingData:  pingResults,
	})

	points := rb.Latest(1)
	if len(points) != 1 {
		t.Fatalf("读取数量错误: 期望 1, 得到 %d", len(points))
	}
	if len(points[0].PingData) != 2 {
		t.Errorf("Ping 数据数量错误: 期望 2, 得到 %d", len(points[0].PingData))
	}
	if points[0].PingData[0].Name != "电信" {
		t.Errorf("Ping 名称错误: 期望 电信, 得到 %s", points[0].PingData[0].Name)
	}
}

func TestRingBuffer_DiskData(t *testing.T) {
	rb := NewRingBuffer(3600)

	disks := []sharedmodel.DiskInfo{
		{Device: "/", Total: 53687091200, Used: 37580963840},
		{Device: "/data", Total: 107374182400, Used: 53687091200},
	}

	rb.Write(MetricPoint{
		Timestamp: time.Now().Unix(),
		Disks:     disks,
	})

	points := rb.Latest(1)
	if len(points[0].Disks) != 2 {
		t.Errorf("磁盘数据数量错误: 期望 2, 得到 %d", len(points[0].Disks))
	}
}

func TestRingBuffer_Size(t *testing.T) {
	rb := NewRingBuffer(5)

	if rb.Size() != 0 {
		t.Errorf("初始大小应为 0, 得到 %d", rb.Size())
	}

	rb.Write(MetricPoint{Timestamp: 1})
	if rb.Size() != 1 {
		t.Errorf("大小应为 1, 得到 %d", rb.Size())
	}

	for i := 0; i < 10; i++ {
		rb.Write(MetricPoint{Timestamp: int64(i + 2)})
	}

	if rb.Size() != 5 {
		t.Errorf("满后大小应为 5, 得到 %d", rb.Size())
	}
}

func TestRingBuffer_Clear(t *testing.T) {
	rb := NewRingBuffer(10)

	for i := 0; i < 5; i++ {
		rb.Write(MetricPoint{Timestamp: int64(i)})
	}

	rb.Clear()

	if rb.Size() != 0 {
		t.Errorf("清空后大小应为 0, 得到 %d", rb.Size())
	}

	points := rb.Latest(5)
	if len(points) != 0 {
		t.Errorf("清空后读取应返回空, 得到 %d", len(points))
	}
}

func TestRingBuffer_GetAll(t *testing.T) {
	rb := NewRingBuffer(10)

	for i := 0; i < 5; i++ {
		rb.Write(MetricPoint{
			Timestamp: int64(i),
			CPU:       float64(i),
		})
	}

	points := rb.GetAll()
	if len(points) != 5 {
		t.Fatalf("GetAll 数量错误: 期望 5, 得到 %d", len(points))
	}

	// 最新的在前
	if points[0].CPU != 4 {
		t.Errorf("第一个应为最新(CPU=4), 得到 %f", points[0].CPU)
	}
}

func TestRingBuffer_GetAllEmpty(t *testing.T) {
	rb := NewRingBuffer(10)
	points := rb.GetAll()
	if len(points) != 0 {
		t.Errorf("空缓冲 GetAll 应返回空, 得到 %d", len(points))
	}
}

func TestRingBuffer_GetAvgCPU(t *testing.T) {
	rb := NewRingBuffer(10)

	// 写入 CPU 值: 10, 20, 30, 40, 50
	for i := 1; i <= 5; i++ {
		rb.Write(MetricPoint{
			Timestamp: int64(i),
			CPU:       float64(i * 10),
		})
	}

	// 最近 3 个的平均: (50+40+30)/3 = 40
	avg := rb.GetAvgCPU(3)
	if avg != 40 {
		t.Errorf("平均 CPU 错误: 期望 40, 得到 %f", avg)
	}

	// 全部 5 个的平均: (10+20+30+40+50)/5 = 30
	avg = rb.GetAvgCPU(10)
	if avg != 30 {
		t.Errorf("平均 CPU 错误: 期望 30, 得到 %f", avg)
	}
}

func TestRingBuffer_GetAvgCPU_Empty(t *testing.T) {
	rb := NewRingBuffer(10)
	avg := rb.GetAvgCPU(5)
	if avg != 0 {
		t.Errorf("空缓冲平均 CPU 应为 0, 得到 %f", avg)
	}
}

func TestRingBuffer_GetAvgMem(t *testing.T) {
	rb := NewRingBuffer(10)

	for i := 1; i <= 4; i++ {
		rb.Write(MetricPoint{
			Timestamp: int64(i),
			Mem:       float64(i * 25),
		})
	}

	// 最近 2 个的平均: (100+75)/2 = 87.5
	avg := rb.GetAvgMem(2)
	if avg != 87.5 {
		t.Errorf("平均内存错误: 期望 87.5, 得到 %f", avg)
	}
}

func TestRingBuffer_GetAvgMem_Empty(t *testing.T) {
	rb := NewRingBuffer(10)
	avg := rb.GetAvgMem(5)
	if avg != 0 {
		t.Errorf("空缓冲平均内存应为 0, 得到 %f", avg)
	}
}

func TestRingBuffer_GetByTimeRange_Empty(t *testing.T) {
	rb := NewRingBuffer(10)
	points := rb.GetByTimeRange(0, 100)
	if len(points) != 0 {
		t.Errorf("空缓冲时间范围查询应返回空, 得到 %d", len(points))
	}
}

func TestRingBuffer_GetByTimeRange_NoMatch(t *testing.T) {
	rb := NewRingBuffer(10)
	rb.Write(MetricPoint{Timestamp: 50, CPU: 10})

	points := rb.GetByTimeRange(100, 200)
	if len(points) != 0 {
		t.Errorf("无匹配时应返回空, 得到 %d", len(points))
	}
}
