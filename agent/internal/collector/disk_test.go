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

	if len(disks) != 2 {
		t.Fatalf("磁盘数量错误: 期望 2, 得到 %d", len(disks))
	}

	// 验证根分区
	// total = 53687091200, free = 16106127360, used = 53687091200 - 16106127360 = 37580963840
	rootDisk := disks[0]
	if rootDisk.Device != "/" {
		t.Errorf("根分区设备错误: 期望 /, 得到 %s", rootDisk.Device)
	}
	if rootDisk.Total != 53687091200 {
		t.Errorf("根分区总量错误: 期望 53687091200, 得到 %d", rootDisk.Total)
	}
	expectedUsed := uint64(53687091200 - 16106127360)
	if rootDisk.Used != expectedUsed {
		t.Errorf("根分区已用错误: 期望 %d, 得到 %d", expectedUsed, rootDisk.Used)
	}

	// 验证 /data 分区
	dataDisk := disks[1]
	if dataDisk.Device != "/data" {
		t.Errorf("/data 分区设备错误: 期望 /data, 得到 %s", dataDisk.Device)
	}
	if dataDisk.Total != 107374182400 {
		t.Errorf("/data 分区总量错误: 期望 107374182400, 得到 %d", dataDisk.Total)
	}
	expectedDataUsed := uint64(107374182400 - 53687091200)
	if dataDisk.Used != expectedDataUsed {
		t.Errorf("/data 分区已用错误: 期望 %d, 得到 %d", expectedDataUsed, dataDisk.Used)
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

	// 应该只返回 ext4 文件系统的挂载点
	if len(disks) != 2 {
		t.Errorf("磁盘数量错误: 期望 2（过滤特殊文件系统后）, 得到 %d", len(disks))
	}
}
