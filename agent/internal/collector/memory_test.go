package collector

import (
	"testing"

	"github.com/server-probe/shared/model"
)

const meminfoSample = `MemTotal:        8388608 kB
MemFree:          2097152 kB
MemAvailable:     4194304 kB
Buffers:           524288 kB
Cached:           1572864 kB
SwapCached:             0 kB
Active:           1048576 kB
Inactive:          524288 kB
SwapTotal:        4194304 kB
SwapFree:         4194304 kB
Dirty:               1024 kB
Writeback:              0 kB
AnonPages:        1048576 kB
Mapped:            262144 kB
Shmem:              65536 kB
KReclaimable:      393216 kB
Slab:              524288 kB
SReclaimable:      393216 kB
SUnreclaim:        131072 kB
`

func TestMemoryCollector_Collect(t *testing.T) {
	reader := &mockFileReader{
		files: map[string][]byte{
			"/proc/meminfo": []byte(meminfoSample),
		},
	}

	collector := NewMemoryCollector(reader)
	result, err := collector.Collect()
	if err != nil {
		t.Fatalf("采集失败: %v", err)
	}

	memInfo, ok := result.(model.MemoryInfo)
	if !ok {
		t.Fatalf("返回类型错误，期望 model.MemoryInfo，得到 %T", result)
	}

	// MemTotal: 8388608 kB = 8589934592 bytes
	expectedTotal := uint64(8388608 * 1024)
	if memInfo.Total != expectedTotal {
		t.Errorf("总内存错误: 期望 %d, 得到 %d", expectedTotal, memInfo.Total)
	}

	// Used = Total - MemAvailable = 8388608 - 4194304 = 4194304 kB
	expectedUsed := uint64(4194304 * 1024)
	if memInfo.Used != expectedUsed {
		t.Errorf("已用内存错误: 期望 %d, 得到 %d", expectedUsed, memInfo.Used)
	}

	// SwapTotal: 4194304 kB
	expectedSwapTotal := uint64(4194304 * 1024)
	if memInfo.SwapTotal != expectedSwapTotal {
		t.Errorf("Swap 总量错误: 期望 %d, 得到 %d", expectedSwapTotal, memInfo.SwapTotal)
	}

	// SwapFree: 4194304 kB, SwapUsed = 0
	if memInfo.SwapUsed != 0 {
		t.Errorf("Swap 已用错误: 期望 0, 得到 %d", memInfo.SwapUsed)
	}
}

func TestMemoryCollector_Name(t *testing.T) {
	collector := NewMemoryCollector(&OSFileReader{})
	if collector.Name() != "memory" {
		t.Errorf("采集器名称错误: 期望 memory, 得到 %s", collector.Name())
	}
}

func TestParseMemInfo(t *testing.T) {
	info, err := parseMemInfo(meminfoSample)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}

	if info.total != 8388608*1024 {
		t.Errorf("total 错误: 得到 %d", info.total)
	}
	if info.available != 4194304*1024 {
		t.Errorf("available 错误: 得到 %d", info.available)
	}
	if info.swapTotal != 4194304*1024 {
		t.Errorf("swapTotal 错误: 得到 %d", info.swapTotal)
	}
	if info.swapFree != 4194304*1024 {
		t.Errorf("swapFree 错误: 得到 %d", info.swapFree)
	}
}

func TestParseMemInfo_MissingFields(t *testing.T) {
	// 缺少 SwapTotal 和 SwapFree
	data := `MemTotal:        8388608 kB
MemFree:          2097152 kB
MemAvailable:     4194304 kB
`
	info, err := parseMemInfo(data)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}

	if info.total != 8388608*1024 {
		t.Errorf("total 错误: 得到 %d", info.total)
	}
	if info.swapTotal != 0 {
		t.Errorf("swapTotal 应为 0, 得到 %d", info.swapTotal)
	}
}
