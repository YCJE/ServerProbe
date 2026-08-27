package api

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/server-probe/server/internal/repository"
	"github.com/server-probe/server/internal/service"
	"gorm.io/gorm"
)

// SettingsHandler 系统设置与数据库管理处理器
type SettingsHandler struct {
	settings   *service.SettingsService
	recordRepo *repository.RecordRepository
	alertRepo  *repository.AlertRepository
	db         *gorm.DB
	dataDir    string
	// DB 维护操作（备份/清理/压缩）互斥标志：VACUUM 并发执行会触发 database is locked
	dbMaintaining atomic.Bool
}

// NewSettingsHandler 创建设置处理器
func NewSettingsHandler(settings *service.SettingsService, recordRepo *repository.RecordRepository, alertRepo *repository.AlertRepository, db *gorm.DB, dataDir string) *SettingsHandler {
	return &SettingsHandler{
		settings:   settings,
		recordRepo: recordRepo,
		alertRepo:  alertRepo,
		db:         db,
		dataDir:    dataDir,
	}
}

// settingsResponse 管理端设置响应（带默认值兜底）
type settingsResponse struct {
	SiteTitle           string `json:"site_title"`
	SiteDescription     string `json:"site_description"`
	Announcement        string `json:"announcement"`
	CustomFooter        string `json:"custom_footer"`
	DefaultHistoryRange string `json:"default_history_range"`
	OfflineGraceSeconds int    `json:"offline_grace_seconds"`
	RetentionDays       int    `json:"retention_days"`
	MaxChartPoints      int    `json:"max_chart_points"`
}

// HandleGetSettings 获取全部系统设置
// 路由: GET /api/v1/settings
func (h *SettingsHandler) HandleGetSettings(c *gin.Context) {
	c.JSON(http.StatusOK, settingsResponse{
		SiteTitle:           h.settings.SiteTitle(),
		SiteDescription:     h.settings.SiteDescription(),
		Announcement:        h.settings.Announcement(),
		CustomFooter:        h.settings.CustomFooter(),
		DefaultHistoryRange: h.settings.DefaultHistoryRange(),
		OfflineGraceSeconds: h.settings.OfflineGraceSeconds(),
		RetentionDays:       h.settings.RetentionDays(),
		MaxChartPoints:      h.settings.MaxChartPoints(),
	})
}

// HandleUpdateSettings 更新系统设置
// 路由: PUT /api/v1/settings
func (h *SettingsHandler) HandleUpdateSettings(c *gin.Context) {
	var req settingsResponse
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求体"})
		return
	}

	// 字段校验与范围钳制
	grace := req.OfflineGraceSeconds
	if grace < 30 {
		grace = 30
	} else if grace > 86400 {
		grace = 86400
	}
	retention := req.RetentionDays
	if retention < 1 {
		retention = 1
	} else if retention > 3650 {
		retention = 3650
	}
	points := req.MaxChartPoints
	if points < 100 {
		points = 100
	} else if points > 2000 {
		points = 2000
	}
	historyRange := req.DefaultHistoryRange
	validRange := map[string]bool{"1h": true, "6h": true, "12h": true, "1d": true, "2d": true, "3d": true}
	if !validRange[historyRange] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "默认历史范围无效，支持: 1h/6h/12h/1d/2d/3d"})
		return
	}
	// 长度限制防止滥用存储
	if len(req.SiteTitle) > 100 || len(req.SiteDescription) > 300 || len(req.Announcement) > 1000 || len(req.CustomFooter) > 500 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "文本字段过长（标题≤100/描述≤300/公告≤1000/页脚≤500）"})
		return
	}

	kv := map[string]string{
		service.SettingSiteTitle:           req.SiteTitle,
		service.SettingSiteDescription:     req.SiteDescription,
		service.SettingAnnouncement:        req.Announcement,
		service.SettingCustomFooter:        req.CustomFooter,
		service.SettingDefaultHistoryRange: historyRange,
		service.SettingOfflineGraceSeconds: strconv.Itoa(grace),
		service.SettingRetentionDays:       strconv.Itoa(retention),
		service.SettingMaxChartPoints:      strconv.Itoa(points),
	}
	if err := h.settings.Update(kv); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存设置失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "设置已保存"})
}

// HandlePublicSettings 公开设置（站点标题/描述/公告/页脚/默认历史范围，非敏感）
// 路由: GET /api/v1/public/settings
func (h *SettingsHandler) HandlePublicSettings(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"site_title":            h.settings.SiteTitle(),
		"site_description":      h.settings.SiteDescription(),
		"announcement":          h.settings.Announcement(),
		"custom_footer":         h.settings.CustomFooter(),
		"default_history_range": h.settings.DefaultHistoryRange(),
	})
}

// dbStatsResponse 数据库统计
type dbStatsResponse struct {
	DBSizeBytes      int64 `json:"db_size_bytes"`
	WALSizeBytes     int64 `json:"wal_size_bytes"`
	MetricRecords    int64 `json:"metric_records"`
	AlertHistory     int64 `json:"alert_history"`
	Agents           int64 `json:"agents"`
	TrafficRecords   int64 `json:"traffic_records"`
	ServiceMonitors  int64 `json:"service_monitors"`
	SSLMonitors      int64 `json:"ssl_monitors"`
	OldestMetricTime int64 `json:"oldest_metric_time"`
}

// HandleDBStats 数据库统计信息
// 路由: GET /api/v1/db/stats
func (h *SettingsHandler) HandleDBStats(c *gin.Context) {
	var stats dbStatsResponse
	stats.DBSizeBytes = h.recordRepo.GetDBSize()

	// WAL 文件大小
	if info, err := os.Stat(filepath.Join(h.dataDir, "probe.db-wal")); err == nil {
		stats.WALSizeBytes = info.Size()
	}

	h.db.Table("metric_records").Count(&stats.MetricRecords)
	h.db.Table("alert_history").Count(&stats.AlertHistory)
	h.db.Table("agents").Count(&stats.Agents)
	h.db.Table("traffic_records").Count(&stats.TrafficRecords)
	h.db.Table("service_monitors").Count(&stats.ServiceMonitors)
	h.db.Table("ssl_cert_monitors").Count(&stats.SSLMonitors)

	var oldest *int64
	h.db.Table("metric_records").Select("min(timestamp)").Scan(&oldest)
	if oldest != nil {
		stats.OldestMetricTime = *oldest
	}

	c.JSON(http.StatusOK, stats)
}

// HandleDBBackup 下载数据库备份（VACUUM INTO 一致性快照）
// 路由: GET /api/v1/db/backup
func (h *SettingsHandler) HandleDBBackup(c *gin.Context) {
	// 互斥：防止并发 VACUUM 触发 database is locked
	if !h.dbMaintaining.CompareAndSwap(false, true) {
		c.JSON(http.StatusConflict, gin.H{"error": "数据库维护操作进行中，请稍后再试"})
		return
	}
	defer h.dbMaintaining.Store(false)

	backupDir := filepath.Join(h.dataDir, "backup")
	if err := os.MkdirAll(backupDir, 0700); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建备份目录失败"})
		return
	}

	// 清理残留的旧备份文件（进程重启后异步清理 goroutine 丢失，防止孤立文件无限堆积）
	h.cleanStaleBackups(backupDir)

	backupPath := filepath.Join(backupDir, fmt.Sprintf("probe-backup-%s.db", time.Now().Format("20060102-150405")))
	// VACUUM INTO 生成一致性快照，不影响在线服务
	if err := h.db.Exec("VACUUM INTO ?", backupPath).Error; err != nil {
		_ = os.Remove(backupPath)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "生成备份失败: " + err.Error()})
		return
	}

	c.FileAttachment(backupPath, filepath.Base(backupPath))

	// 下载完成后异步清理备份文件（保留 10 分钟供慢速下载）
	go func() {
		time.Sleep(10 * time.Minute)
		_ = os.Remove(backupPath)
	}()
}

// cleanStaleBackups 删除超过 30 分钟的残留备份文件
func (h *SettingsHandler) cleanStaleBackups(backupDir string) {
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-30 * time.Minute)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if info, err := e.Info(); err == nil && info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(backupDir, e.Name()))
		}
	}
}

// HandleDBCleanup 按天数清理历史数据（指标记录 + 告警历史）
// 路由: POST /api/v1/db/cleanup
func (h *SettingsHandler) HandleDBCleanup(c *gin.Context) {
	// 互斥：防止与其他 DB 维护操作并发执行
	if !h.dbMaintaining.CompareAndSwap(false, true) {
		c.JSON(http.StatusConflict, gin.H{"error": "数据库维护操作进行中，请稍后再试"})
		return
	}
	defer h.dbMaintaining.Store(false)

	var req struct {
		Days int `json:"days"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求体"})
		return
	}
	if req.Days < 1 || req.Days > 3650 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "天数范围: 1-3650"})
		return
	}

	before := time.Now().AddDate(0, 0, -req.Days)
	deletedRecords, err := h.recordRepo.DeleteOlderThan(before.Unix())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "清理指标记录失败"})
		return
	}
	deletedAlerts, err := h.alertRepo.CleanupHistoryBefore(before)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "清理告警历史失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":           fmt.Sprintf("已清理 %d 天前的数据", req.Days),
		"deleted_records":   deletedRecords,
		"deleted_alerts":    deletedAlerts,
	})
}

// HandleDBCompact 压缩优化数据库（VACUUM 回收空闲页）
// 路由: POST /api/v1/db/compact
func (h *SettingsHandler) HandleDBCompact(c *gin.Context) {
	// 互斥：VACUUM 会独占数据库写锁，并发执行必然失败，也防止清理/备份同时进行
	if !h.dbMaintaining.CompareAndSwap(false, true) {
		c.JSON(http.StatusConflict, gin.H{"error": "数据库维护操作进行中，请稍后再试"})
		return
	}
	defer h.dbMaintaining.Store(false)

	if err := h.db.Exec("VACUUM").Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "数据库压缩失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message":       "数据库压缩完成",
		"db_size_bytes": h.recordRepo.GetDBSize(),
	})
}
