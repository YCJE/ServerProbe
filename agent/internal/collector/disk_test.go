package collector

import (
	"testing"

	"github.com/server-probe/shared/model"
)

// mockStatFS 模拟 statfs 结果
type mockStatFS struct {
	total uint64
	free  uint64
}

// mockDiskMounter 模拟磁盘挂载点
type mockDiskMounter struct {
	mounts []MountPoint
	stats  map[string]mockStatFS
}

func (m *mockDiskMounter) GetMountPoints() ([]MountPoint, error) {
	return m.mounts, nil
}

func (m *mockDiskMounter) StatFS(path string) (uint64, uint64, error) {
	if stat, ok := m.stats[path]; ok {
		return stat.total, stat.free, nil
	}
	return 0, 0, nil
}

func TestDiskCollector_Collect(t *testing.T) {
	mounter := &mockDiskMounter{
		mounts: []MountPoint{
			{Device: "/dev/sda1", MountPoint: "/", FSType: "ext4"},
			{Device: "/dev/sdb1", MountPoint: "/data", FSType: "ext4"},
		},
		stats: map[string]mockStatFS{
			"/":     {total: 53687091200, free: 16106127360},  // 50GB total, 15GB free
			"/data": {total: 107374182400, free: 53687091200}, // 100GB total, 50GB free
		},
	}

	collector := NewDiskCollector(mounter)
	result, err := collector.Collect()
	if err != nil {
		t.Fatalf("采集失败: %v", err)
	}

	disks, ok := result.([]model.DiskInfo)
	if !ok {
		t.Fatalf("返回类型错误，期望 []model.DiskInfo，得到 %T", result)
	}

	// 现在返回单个汇总磁盘
	if len(disks) != 1 {
		t.Fatalf("磁盘数量错误: 期望 1 (汇总), 得到 %d", len(disks))
	}

	// 验证汇总磁盘
	disk := disks[0]
	if disk.Device != "total" {
		t.Errorf("设备名错误: 期望 total, 得到 %s", disk.Device)
	}
	expectedTotal := uint64(53687091200 + 107374182400)
	if disk.Total != expectedTotal {
		t.Errorf("总量错误: 期望 %d, 得到 %d", expectedTotal, disk.Total)
	}
	expectedUsed := uint64((53687091200 - 16106127360) + (107374182400 - 53687091200))
	if disk.Used != expectedUsed {
		t.Errorf("已用错误: 期望 %d, 得到 %d", expectedUsed, disk.Used)
	}
}

func TestDiskCollector_Name(t *testing.T) {
	collector := NewDiskCollector(&OSDiskMounter{})
	if collector.Name() != "disk" {
		t.Errorf("采集器名称错误: 期望 disk, 得到 %s", collector.Name())
	}
}

func TestDiskCollector_FilterSpecialFS(t *testing.T) {
	mounter := &mockDiskMounter{
		mounts: []MountPoint{
			{Device: "/dev/sda1", MountPoint: "/", FSType: "ext4"},
			{Device: "proc", MountPoint: "/proc", FSType: "proc"},
			{Device: "sysfs", MountPoint: "/sys", FSType: "sysfs"},
			{Device: "tmpfs", MountPoint: "/dev/shm", FSType: "tmpfs"},
			{Device: "/dev/sdb1", MountPoint: "/data", FSType: "ext4"},
		},
		stats: map[string]mockStatFS{
			"/":     {total: 53687091200, free: 16106127360},
			"/data": {total: 107374182400, free: 53687091200},
		},
	}

	collector := NewDiskCollector(mounter)
	result, err := collector.Collect()
	if err != nil {
		t.Fatalf("采集失败: %v", err)
	}

	disks, ok := result.([]model.DiskInfo)
	if !ok {
		t.Fatalf("返回类型错误，期望 []model.DiskInfo，得到 %T", result)
	}

	// 过滤特殊文件系统后，应返回 1 个汇总磁盘
	if len(disks) != 1 {
		t.Errorf("磁盘数量错误: 期望 1 (汇总), 得到 %d", len(disks))
	}
}
