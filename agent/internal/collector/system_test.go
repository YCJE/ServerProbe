package collector

import (
	"testing"

	"github.com/server-probe/shared/model"
)

const uptimeSample = `86400.50 85432.20
`

// mockProcListDir 模拟 /proc 目录列表
type mockProcListDir struct {
	dirs []string
}

func TestSystemCollector_Collect(t *testing.T) {
	reader := &mockFileReader{
		files: map[string][]byte{
			"/proc/uptime": []byte(uptimeSample),
		},
	}

	collector := NewSystemCollector(reader, "v1.0.0")
	result, err := collector.Collect()
	if err != nil {
		t.Fatalf("采集失败: %v", err)
	}

	sysInfo, ok := result.(model.SystemInfo)
	if !ok {
		t.Fatalf("返回类型错误，期望 model.SystemInfo，得到 %T", result)
	}

	// 验证 Agent 版本
	if sysInfo.AgentVersion != "v1.0.0" {
		t.Errorf("Agent 版本错误: 期望 v1.0.0, 得到 %s", sysInfo.AgentVersion)
	}
}

func TestSystemCollector_Name(t *testing.T) {
	collector := NewSystemCollector(&OSFileReader{}, "v1.0.0")
	if collector.Name() != "system" {
		t.Errorf("采集器名称错误: 期望 system, 得到 %s", collector.Name())
	}
}

func TestParseUptime(t *testing.T) {
	uptime, err := parseUptime(uptimeSample)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}

	// 86400.50 秒
	if uptime != 86400 {
		t.Errorf("运行时间错误: 期望 86400, 得到 %d", uptime)
	}
}

func TestParseUptime_InvalidFormat(t *testing.T) {
	_, err := parseUptime("invalid")
	if err == nil {
		t.Error("期望返回错误，但未返回")
	}
}

func TestCountProcesses(t *testing.T) {
	// 模拟 /proc 目录中的进程
	// 正常的进程目录是纯数字
	dirs := []string{"1", "2", "10", "100", "self", "cpuinfo", "200", "meminfo"}
	count := countProcessDirs(dirs)

	// 纯数字目录: 1, 2, 10, 100, 200 = 5 个
	if count != 5 {
		t.Errorf("进程数错误: 期望 5, 得到 %d", count)
	}
}
