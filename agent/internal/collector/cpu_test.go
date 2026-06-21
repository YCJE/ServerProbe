package collector

import (
	"testing"

	"github.com/server-probe/shared/model"
)

// mockFileReader 用于测试的文件读取模拟
type mockFileReader struct {
	files map[string][]byte
}

func (m *mockFileReader) ReadFile(path string) ([]byte, error) {
	if content, ok := m.files[path]; ok {
		return content, nil
	}
	return nil, &osPathError{path: path}
}

type osPathError struct {
	path string
}

func (e *osPathError) Error() string {
	return "file not found: " + e.path
}

// 测试用的 /proc/stat 数据
// 格式：cpu  user nice system idle iowait irq softirq steal guest guest_nice
const statSample1 = `cpu  1000 200 500 8000 100 10 5 0 0 0
cpu0 250 50 125 2000 25 5 2 0 0 0
cpu1 250 50 125 2000 25 5 2 0 0 0
cpu2 250 50 125 2000 25 0 1 0 0 0
cpu3 250 50 125 2000 25 0 0 0 0 0
intr 1234567890
ctxt 9876543210
btime 1718900000
processes 100000
procs_running 2
procs_blocked 0
`

// 第二次采样，用于计算差值
// user 增加 100，system 增加 50，idle 增加 900
// 总增量 = 100+0+50+900 = 1050
// 使用率 = (100+50)/1050 = 14.2857...%
const statSample2 = `cpu  1100 200 550 8900 100 10 5 0 0 0
cpu0 275 50 137 2225 25 5 2 0 0 0
cpu1 275 50 137 2225 25 5 2 0 0 0
cpu2 275 50 137 2225 25 0 1 0 0 0
cpu3 275 50 139 2225 25 0 0 0 0 0
intr 1234567990
ctxt 9876543300
btime 1718900000
processes 100010
procs_running 2
procs_blocked 0
`

const cpuinfoSample = `processor	: 0
vendor_id	: GenuineIntel
cpu family	: 6
model		: 79
model name	: Intel(R) Xeon(R) CPU E5-2680 v4 @ 2.40GHz
stepping	: 1
microcode	: 0xb000038
cpu MHz		: 2399.998
cache size	: 35840 KB
physical id	: 0
siblings	: 4
core id		: 0
cpu cores	: 4
processor	: 1
vendor_id	: GenuineIntel
cpu family	: 6
model		: 79
model name	: Intel(R) Xeon(R) CPU E5-2680 v4 @ 2.40GHz
cpu MHz		: 2399.998
processor	: 2
vendor_id	: GenuineIntel
model name	: Intel(R) Xeon(R) CPU E5-2680 v4 @ 2.40GHz
processor	: 3
vendor_id	: GenuineIntel
model name	: Intel(R) Xeon(R) CPU E5-2680 v4 @ 2.40GHz
`

const loadavgSample = `0.52 0.48 0.50 2/156 3456
`

func TestCPUCollector_Collect(t *testing.T) {
	reader := &mockFileReader{
		files: map[string][]byte{
			"/proc/stat":    []byte(statSample1),
			"/proc/cpuinfo": []byte(cpuinfoSample),
			"/proc/loadavg": []byte(loadavgSample),
		},
	}

	collector := NewCPUCollector(reader)

	// 第一次采样
	result, err := collector.Collect()
	if err != nil {
		t.Fatalf("第一次采集失败: %v", err)
	}

	cpuInfo, ok := result.(model.CPUInfo)
	if !ok {
		t.Fatalf("返回类型错误，期望 model.CPUInfo，得到 %T", result)
	}

	// 验证 CPU 核心数
	if cpuInfo.Cores != 4 {
		t.Errorf("CPU 核心数错误: 期望 4, 得到 %d", cpuInfo.Cores)
	}

	// 验证 CPU 型号
	expectedModel := "Intel(R) Xeon(R) CPU E5-2680 v4 @ 2.40GHz"
	if cpuInfo.Model != expectedModel {
		t.Errorf("CPU 型号错误: 期望 %s, 得到 %s", expectedModel, cpuInfo.Model)
	}

	// 验证负载
	if cpuInfo.Load1 != 0.52 {
		t.Errorf("1分钟负载错误: 期望 0.52, 得到 %f", cpuInfo.Load1)
	}
	if cpuInfo.Load5 != 0.48 {
		t.Errorf("5分钟负载错误: 期望 0.48, 得到 %f", cpuInfo.Load5)
	}
	if cpuInfo.Load15 != 0.50 {
		t.Errorf("15分钟负载错误: 期望 0.50, 得到 %f", cpuInfo.Load15)
	}

	// 第一次采样使用率应为 0（没有前一次数据）
	if cpuInfo.Usage != 0 {
		t.Errorf("首次采集使用率应为 0, 得到 %f", cpuInfo.Usage)
	}
}

func TestCPUCollector_UsageCalculation(t *testing.T) {
	reader := &mockFileReader{
		files: map[string][]byte{
			"/proc/stat":    []byte(statSample1),
			"/proc/cpuinfo": []byte(cpuinfoSample),
			"/proc/loadavg": []byte(loadavgSample),
		},
	}

	collector := NewCPUCollector(reader)

	// 第一次采样
	_, err := collector.Collect()
	if err != nil {
		t.Fatalf("第一次采集失败: %v", err)
	}

	// 更新 mock 数据为第二次采样
	reader.files["/proc/stat"] = []byte(statSample2)

	// 第二次采样，计算差值
	result, err := collector.Collect()
	if err != nil {
		t.Fatalf("第二次采集失败: %v", err)
	}

	cpuInfo, ok := result.(model.CPUInfo)
	if !ok {
		t.Fatalf("返回类型错误，期望 model.CPUInfo，得到 %T", result)
	}

	// user 增量 = 1100-1000 = 100
	// nice 增量 = 200-200 = 0
	// system 增量 = 550-500 = 50
	// idle 增量 = 8900-8000 = 900
	// iowait 增量 = 100-100 = 0
	// irq 增量 = 10-10 = 0
	// softirq 增量 = 5-5 = 0
	// steal 增量 = 0-0 = 0
	// 总增量 = 100+0+50+900+0+0+0+0 = 1050
	// 工作时间 = 100+0+50+0+0+0+0 = 150
	// 使用率 = 150/1050 * 100 = 14.2857...%
	expectedUsage := 150.0 / 1050.0 * 100
	if cpuInfo.Usage < expectedUsage-0.01 || cpuInfo.Usage > expectedUsage+0.01 {
		t.Errorf("CPU 使用率错误: 期望 %.4f, 得到 %.4f", expectedUsage, cpuInfo.Usage)
	}
}

func TestCPUCollector_Name(t *testing.T) {
	collector := NewCPUCollector(&OSFileReader{})
	if collector.Name() != "cpu" {
		t.Errorf("采集器名称错误: 期望 cpu, 得到 %s", collector.Name())
	}
}

func TestParseCPUStat(t *testing.T) {
	stat, err := parseCPUStat(statSample1)
	if err != nil {
		t.Fatalf("解析 /proc/stat 失败: %v", err)
	}

	if stat.User != 1000 {
		t.Errorf("User 错误: 期望 1000, 得到 %d", stat.User)
	}
	if stat.Nice != 200 {
		t.Errorf("Nice 错误: 期望 200, 得到 %d", stat.Nice)
	}
	if stat.System != 500 {
		t.Errorf("System 错误: 期望 500, 得到 %d", stat.System)
	}
	if stat.Idle != 8000 {
		t.Errorf("Idle 错误: 期望 8000, 得到 %d", stat.Idle)
	}
	if stat.IOWait != 100 {
		t.Errorf("IOWait 错误: 期望 100, 得到 %d", stat.IOWait)
	}
}

func TestParseCPUInfo(t *testing.T) {
	cores, model, err := parseCPUInfo(cpuinfoSample)
	if err != nil {
		t.Fatalf("解析 /proc/cpuinfo 失败: %v", err)
	}

	if cores != 4 {
		t.Errorf("核心数错误: 期望 4, 得到 %d", cores)
	}

	expectedModel := "Intel(R) Xeon(R) CPU E5-2680 v4 @ 2.40GHz"
	if model != expectedModel {
		t.Errorf("型号错误: 期望 %s, 得到 %s", expectedModel, model)
	}
}

func TestParseLoadavg(t *testing.T) {
	load1, load5, load15, err := parseLoadavg(loadavgSample)
	if err != nil {
		t.Fatalf("解析 /proc/loadavg 失败: %v", err)
	}

	if load1 != 0.52 {
		t.Errorf("1分钟负载错误: 期望 0.52, 得到 %f", load1)
	}
	if load5 != 0.48 {
		t.Errorf("5分钟负载错误: 期望 0.48, 得到 %f", load5)
	}
	if load15 != 0.50 {
		t.Errorf("15分钟负载错误: 期望 0.50, 得到 %f", load15)
	}
}
