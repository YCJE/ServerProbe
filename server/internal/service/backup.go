package service

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
)

// 内部运行时设置键（下划线前缀 = 不参与设置导出/导入）
const (
	SettingAppVersion     = "_app_version"      // 程序版本（变更触发自动备份）
	SettingExpireLastSent = "_expire_last_sent" // 到期通知上次发送日期（YYYY-MM-DD，防重启重发）
)

// 自动备份保留策略：版本升级触发的自动备份保留最近 5 份
const (
	autoBackupPrefix   = "auto-before-"
	autoBackupKeep     = 5
	autoBackupMaxAge   = 30 * 24 * time.Hour // 自动备份最长保留 30 天
)

// RunAutoBackupIfNeeded 检测程序版本变更，变更时在启动早期生成自动备份
// （借鉴 Komari backupOnVersionUpgrade：升级常伴随 AutoMigrate 结构变更，
// 自动备份保证升级失败时可直接回退到旧版本数据文件）
// 返回是否执行了备份
func RunAutoBackupIfNeeded(db *gorm.DB, settings *SettingsService, dataDir, version string) bool {
	if db == nil || settings == nil {
		return false
	}

	stored := settings.GetString(SettingAppVersion, "")
	if stored == version {
		return false
	}

	backupDone := false
	if stored != "" {
		// 已存在历史版本记录 → 本次是升级，执行备份
		backupDir := filepath.Join(dataDir, "backup")
		if err := os.MkdirAll(backupDir, 0700); err == nil {
			name := fmt.Sprintf("%s%s-to-%s-%s.db", autoBackupPrefix, sanitizeBackupName(stored), sanitizeBackupName(version), time.Now().Format("20060102-150405"))
			path := filepath.Join(backupDir, name)
			if err := db.Exec("VACUUM INTO ?", path).Error; err != nil {
				log.Printf("警告: 升级自动备份失败（%s → %s）: %v", stored, version, err)
			} else {
				log.Printf("检测到版本变更（%s → %s），已生成升级前自动备份: %s", stored, version, name)
				backupDone = true
				pruneAutoBackups(backupDir)
			}
		}
	}

	// 首次运行（stored 为空）只记录版本不备份
	if err := settings.Update(map[string]string{SettingAppVersion: version}); err != nil {
		log.Printf("警告: 记录程序版本失败: %v", err)
	}
	return backupDone
}

// sanitizeBackupName 清理版本字符串中不适合出现在文件名中的字符
func sanitizeBackupName(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "unknown"
	}
	var b strings.Builder
	for _, r := range v {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	return b.String()
}

// pruneAutoBackups 自动备份数量滚动清理：保留最近 autoBackupKeep 份，
// 同时删除超过 autoBackupMaxAge 的旧自动备份（手动备份不受影响）
func pruneAutoBackups(backupDir string) {
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return
	}

	type autoBackup struct {
		name    string
		modTime time.Time
	}
	var autos []autoBackup
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), autoBackupPrefix) || !strings.HasSuffix(e.Name(), ".db") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		autos = append(autos, autoBackup{name: e.Name(), modTime: info.ModTime()})
	}
	if len(autos) <= autoBackupKeep {
		return
	}

	// 按修改时间倒序，保留最新 autoBackupKeep 份
	sort.Slice(autos, func(i, j int) bool { return autos[i].modTime.After(autos[j].modTime) })
	for _, a := range autos[autoBackupKeep:] {
		_ = os.Remove(filepath.Join(backupDir, a.name))
	}
}
