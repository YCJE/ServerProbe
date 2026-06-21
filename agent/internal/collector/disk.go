package collector

import (
	"fmt"
	"os"
	"strings"

	"github.com/server-probe/shared/model"
)

// MountPoint 挂载点信息
type MountPoint struct {
	Device     string // 设备名
	MountPoint string // 挂载路径
	FSType     string // 文件系统类型
}

// DiskMounter 磁盘挂载点接口（便于测试）
type DiskMounter interface {
	GetMountPoints() ([]MountPoint, error)
	StatFS(path string) (total uint64, free uint64, err error)
}

// DiskCollector 磁盘采集器
type DiskCollector struct {
	mounter DiskMounter
}

// NewDiskCollector 创建磁盘采集器
func NewDiskCollector(mounter DiskMounter) *DiskCollector {
	return &DiskCollector{mounter: mounter}
}

// Name 返回采集器名称
func (c *DiskCollector) Name() string {
	return "disk"
}

// Collect 采集磁盘数据
func (c *DiskCollector) Collect() (interface{}, error) {
	mounts, err := c.mounter.GetMountPoints()
	if err != nil {
		return nil, fmt.Errorf("获取挂载点失败: %w", err)
	}

	var disks []model.DiskInfo

	for _, mount := range mounts {
		// 过滤特殊文件系统
		if !isRealDisk(mount.FSType) {
			continue
		}

		// 跳过设备名不以 / 开头的（如 proc, sysfs）
		if !strings.HasPrefix(mount.Device, "/") {
			continue
		}

		total, free, err := c.mounter.StatFS(mount.MountPoint)
		if err != nil {
			continue
		}

		if total == 0 {
			continue
		}

		used := total - free

		disks = append(disks, model.DiskInfo{
			Device: mount.MountPoint,
			Total:  total,
			Used:   used,
		})
	}

	return disks, nil
}

// isRealDisk 判断是否为真实磁盘文件系统
func isRealFS(fsType string) bool {
	realFS := []string{
		"ext4", "ext3", "ext2", "xfs", "btrfs", "zfs",
		"ntfs", "fat32", "exfat", "f2fs", "reiserfs",
		"apfs", "hfs", "hfsplus",
	}
	for _, fs := range realFS {
		if fsType == fs {
			return true
		}
	}
	return false
}

// isRealDisk 兼容函数名
func isRealDisk(fsType string) bool {
	return isRealFS(fsType)
}

// OSDiskMounter 使用系统调用实现
type OSDiskMounter struct{}

// GetMountPoints 读取 /proc/mounts 获取挂载点
func (m *OSDiskMounter) GetMountPoints() ([]MountPoint, error) {
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return nil, fmt.Errorf("读取 /proc/mounts 失败: %w", err)
	}

	var mounts []MountPoint
	lines := strings.Split(string(data), "\n")

	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		mounts = append(mounts, MountPoint{
			Device:     fields[0],
			MountPoint: fields[1],
			FSType:     fields[2],
		})
	}

	return mounts, nil
}

// StatFS 获取文件系统统计信息
// 在 Linux 上使用 syscall.Statfs
func (m *OSDiskMounter) StatFS(path string) (uint64, uint64, error) {
	return statFS(path)
}
