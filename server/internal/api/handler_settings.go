package api

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/server-probe/server/internal/model"
	"github.com/server-probe/server/internal/repository"
	"github.com/server-probe/server/internal/service"
	"gorm.io/gorm"
)

// SettingsHandler 系统设置与数据库管理处理器
type SettingsHandler struct {
	settings   *service.SettingsService
	recordRepo *repository.RecordRepository
	hourlyRepo *repository.HourlyRepository
	alertRepo  *repository.AlertRepository
	tagRepo    *repository.TagRepository
	agentRepo  *repository.AgentRepository
	db         *gorm.DB
	dataDir    string
	// DB 维护操作（备份/清理/压缩）互斥标志：VACUUM 并发执行会触发 database is locked
	dbMaintaining atomic.Bool
}

// NewSettingsHandler 创建设置处理器
func NewSettingsHandler(settings *service.SettingsService, recordRepo *repository.RecordRepository, hourlyRepo *repository.HourlyRepository, alertRepo *repository.AlertRepository, tagRepo *repository.TagRepository, agentRepo *repository.AgentRepository, db *gorm.DB, dataDir string) *SettingsHandler {
	return &SettingsHandler{
		settings:   settings,
		recordRepo: recordRepo,
		hourlyRepo: hourlyRepo,
		alertRepo:  alertRepo,
		tagRepo:    tagRepo,
		agentRepo:  agentRepo,
		db:         db,
		dataDir:    dataDir,
	}
}

// settingsResponse 管理端设置响应（带默认值兜底）
type settingsResponse struct {
	SiteTitle            string `json:"site_title"`
	SiteDescription      string `json:"site_description"`
	Announcement         string `json:"announcement"`
	CustomFooter         string `json:"custom_footer"`
	DefaultHistoryRange  string `json:"default_history_range"`
	OfflineGraceSeconds  int    `json:"offline_grace_seconds"`
	RetentionDays        int    `json:"retention_days"`
	RetentionDaysHourly  int    `json:"retention_days_hourly"`
	MaxChartPoints       int    `json:"max_chart_points"`
	// 到期提前通知（P2）
	ExpireNotifyEnabled   bool  `json:"expire_notify_enabled"`
	ExpireNotifyLeadDays  int   `json:"expire_notify_lead_days"`
	ExpireNotifyChannelID int64 `json:"expire_notify_channel_id"`
}

// HandleGetSettings 获取全部系统设置
// 路由: GET /api/v1/settings
func (h *SettingsHandler) HandleGetSettings(c *gin.Context) {
	c.JSON(http.StatusOK, settingsResponse{
		SiteTitle:             h.settings.SiteTitle(),
		SiteDescription:       h.settings.SiteDescription(),
		Announcement:          h.settings.Announcement(),
		CustomFooter:          h.settings.CustomFooter(),
		DefaultHistoryRange:   h.settings.DefaultHistoryRange(),
		OfflineGraceSeconds:   h.settings.OfflineGraceSeconds(),
		RetentionDays:         h.settings.RetentionDays(),
		RetentionDaysHourly:   h.settings.RetentionDaysHourly(),
		MaxChartPoints:        h.settings.MaxChartPoints(),
		ExpireNotifyEnabled:   h.settings.ExpireNotifyEnabled(),
		ExpireNotifyLeadDays:  h.settings.ExpireNotifyLeadDays(),
		ExpireNotifyChannelID: h.settings.ExpireNotifyChannelID(),
	})
}

// buildSettingsKV 校验并规范化设置值，返回可落盘的键值对；校验失败返回错误信息
// （HandleUpdateSettings 与设置导入共用，保证两处写入走完全相同的校验与钳制）
func buildSettingsKV(req *settingsResponse) (map[string]string, string) {
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
	retentionHourly := req.RetentionDaysHourly
	if retentionHourly < 30 {
		retentionHourly = 30
	} else if retentionHourly > 3650 {
		retentionHourly = 3650
	}
	points := req.MaxChartPoints
	if points < 100 {
		points = 100
	} else if points > 2000 {
		points = 2000
	}
	leadDays := req.ExpireNotifyLeadDays
	if leadDays < 1 {
		leadDays = 1
	} else if leadDays > 90 {
		leadDays = 90
	}
	channelID := req.ExpireNotifyChannelID
	if channelID < 0 {
		channelID = 0
	}
	if !service.IsValidHistoryRange(req.DefaultHistoryRange) {
		return nil, "默认历史范围无效，支持: 1h/6h/12h/1d/2d/3d/7d/30d/90d/1y"
	}
	// 长度限制防止滥用存储
	if len(req.SiteTitle) > 100 || len(req.SiteDescription) > 300 || len(req.Announcement) > 1000 || len(req.CustomFooter) > 500 {
		return nil, "文本字段过长（标题≤100/描述≤300/公告≤1000/页脚≤500）"
	}

	return map[string]string{
		service.SettingSiteTitle:             req.SiteTitle,
		service.SettingSiteDescription:       req.SiteDescription,
		service.SettingAnnouncement:          req.Announcement,
		service.SettingCustomFooter:          req.CustomFooter,
		service.SettingDefaultHistoryRange:   req.DefaultHistoryRange,
		service.SettingOfflineGraceSeconds:   strconv.Itoa(grace),
		service.SettingRetentionDays:         strconv.Itoa(retention),
		service.SettingRetentionDaysHourly:   strconv.Itoa(retentionHourly),
		service.SettingMaxChartPoints:        strconv.Itoa(points),
		service.SettingExpireNotifyEnabled:   strconv.FormatBool(req.ExpireNotifyEnabled),
		service.SettingExpireNotifyLeadDays:  strconv.Itoa(leadDays),
		service.SettingExpireNotifyChannelID: strconv.FormatInt(channelID, 10),
	}, ""
}

// HandleUpdateSettings 更新系统设置
// 路由: PUT /api/v1/settings
func (h *SettingsHandler) HandleUpdateSettings(c *gin.Context) {
	var req settingsResponse
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求体"})
		return
	}

	kv, errMsg := buildSettingsKV(&req)
	if errMsg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": errMsg})
		return
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
	MetricHourly     int64 `json:"metric_records_hourly"`
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
	h.db.Table("metric_records_hourly").Count(&stats.MetricHourly)
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
	deletedHourly, err := h.hourlyRepo.DeleteOlderThan(before.Unix())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "清理小时聚合记录失败"})
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
		"deleted_hourly":    deletedHourly,
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

// settingsExportVersion 导出文件结构版本（不兼容变更时递增，导入按版本拒绝）
const settingsExportVersion = 1

// settingsExportFile 设置导出文件结构（P2：迁移场景）
// 仅含站点设置、标签与 Agent 元数据；绝不含 Agent Token、管理员密码/TOTP 密钥、
// 通知渠道凭证等敏感字段——导入侧也只接受此白名单结构
type settingsExportFile struct {
	Version    int              `json:"version"`
	ExportedAt string           `json:"exported_at"`
	Settings   settingsResponse `json:"settings"`
	Tags       []exportTag      `json:"tags"`
	Agents     []exportAgent    `json:"agents"`
}

// exportTag 标签导出结构
type exportTag struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

// exportAgent Agent 元数据导出结构（按 hostname 匹配回填，不含 Token/指纹等敏感字段）
type exportAgent struct {
	Hostname          string  `json:"hostname"`
	DisplayName       string  `json:"display_name"`
	Tags              string  `json:"tags"`
	Region            string  `json:"region"`
	CountryCode       string  `json:"country_code"`
	ISP               string  `json:"isp"`
	ExpiresAt         string  `json:"expires_at"` // RFC3339，空=永不过期
	PriceAmount       float64 `json:"price_amount"`
	PriceCurrency     string  `json:"price_currency"`
	PriceCycle        string  `json:"price_cycle"`
	TrafficQuotaBytes int64   `json:"traffic_quota_bytes"`
	TrafficQuotaType  string  `json:"traffic_quota_type"`
}

// HandleExportSettings 导出站点设置 + 标签 + Agent 元数据（迁移场景）
// 路由: GET /api/v1/settings/export（2FA 再验证保护）
func (h *SettingsHandler) HandleExportSettings(c *gin.Context) {
	out := settingsExportFile{
		Version:    settingsExportVersion,
		ExportedAt: time.Now().Format(time.RFC3339),
		Settings: settingsResponse{
			SiteTitle:           h.settings.SiteTitle(),
			SiteDescription:     h.settings.SiteDescription(),
			Announcement:        h.settings.Announcement(),
			CustomFooter:        h.settings.CustomFooter(),
			DefaultHistoryRange: h.settings.DefaultHistoryRange(),
			OfflineGraceSeconds: h.settings.OfflineGraceSeconds(),
			RetentionDays:       h.settings.RetentionDays(),
			RetentionDaysHourly: h.settings.RetentionDaysHourly(),
			MaxChartPoints:      h.settings.MaxChartPoints(),
		},
	}

	if h.tagRepo != nil {
		tags, err := h.tagRepo.List()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取标签失败"})
			return
		}
		for _, t := range tags {
			out.Tags = append(out.Tags, exportTag{Name: t.Name, Color: t.Color})
		}
	}

	if h.agentRepo != nil {
		agents, err := h.agentRepo.List()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取 Agent 列表失败"})
			return
		}
		for _, a := range agents {
			item := exportAgent{
				Hostname:          a.Hostname,
				DisplayName:       a.DisplayName,
				Tags:              a.Tags,
				Region:            a.Region,
				CountryCode:       a.CountryCode,
				ISP:               a.ISP,
				PriceAmount:       a.PriceAmount,
				PriceCurrency:     a.PriceCurrency,
				PriceCycle:        a.PriceCycle,
				TrafficQuotaBytes: a.TrafficQuotaBytes,
				TrafficQuotaType:  a.TrafficQuotaType,
			}
			if a.ExpiresAt != nil {
				item.ExpiresAt = a.ExpiresAt.Format(time.RFC3339)
			}
			out.Agents = append(out.Agents, item)
		}
	}

	filename := fmt.Sprintf("server-probe-settings-%s.json", time.Now().Format("20060102-150405"))
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.JSON(http.StatusOK, out)
}

// HandleImportSettings 导入设置导出文件：设置覆盖；标签按名称合并；Agent 元数据按 hostname 匹配更新
// 路由: POST /api/v1/settings/import（2FA 再验证保护）
func (h *SettingsHandler) HandleImportSettings(c *gin.Context) {
	var file settingsExportFile
	if err := c.ShouldBindJSON(&file); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的导出文件格式"})
		return
	}
	if file.Version != settingsExportVersion {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("不支持的导出文件版本（当前支持 v%d）", settingsExportVersion)})
		return
	}

	// 1. 设置覆盖（与手动保存走同一套校验与钳制）
	kv, errMsg := buildSettingsKV(&file.Settings)
	if errMsg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "设置部分无效: " + errMsg})
		return
	}
	if err := h.settings.Update(kv); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "写入设置失败"})
		return
	}

	// 2. 标签按名称合并：已存在则更新颜色，不存在则创建
	tagsCreated, tagsUpdated := 0, 0
	if h.tagRepo != nil {
		existing, err := h.tagRepo.List()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取标签失败"})
			return
		}
		byName := make(map[string]*model.Tag, len(existing))
		for i := range existing {
			byName[existing[i].Name] = &existing[i]
		}
		for _, t := range file.Tags {
			name := strings.TrimSpace(t.Name)
			if name == "" || len(name) > 100 || len(t.Color) > 32 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "标签数据无效（名称非空且≤100字符，颜色≤32字符）"})
				return
			}
			if cur, ok := byName[name]; ok {
				cur.Color = t.Color
				if err := h.tagRepo.Update(cur); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "更新标签失败"})
					return
				}
				tagsUpdated++
			} else {
				tag := &model.Tag{Name: name, Color: t.Color}
				if err := h.tagRepo.Create(tag); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "创建标签失败"})
					return
				}
				byName[name] = tag
				tagsCreated++
			}
		}
	}

	// 3. Agent 元数据按 hostname 匹配更新（不存在跳过，不创建新 Agent——
	//    Agent 必须经注册流程绑定 Token/指纹，导入文件不含也无法含这些凭证）
	agentsUpdated, agentsSkipped := 0, 0
	if h.agentRepo != nil {
		existing, err := h.agentRepo.List()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取 Agent 列表失败"})
			return
		}
		byHostname := make(map[string][]*model.Agent, len(existing))
		for i := range existing {
			byHostname[existing[i].Hostname] = append(byHostname[existing[i].Hostname], &existing[i])
		}

		for _, a := range file.Agents {
			hostname := strings.TrimSpace(a.Hostname)
			if hostname == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Agent 数据无效（hostname 不能为空）"})
				return
			}
			matches, ok := byHostname[hostname]
			if !ok || len(matches) == 0 {
				agentsSkipped++
				continue
			}

			// 元数据校验（与手动更新 Agent 元数据同一套规则）
			if len(a.Region) > 100 || len(a.ISP) > 100 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Agent 数据无效（位置/供应商过长）"})
				return
			}
			if cc := strings.ToUpper(strings.TrimSpace(a.CountryCode)); cc != "" && len(cc) != 2 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Agent 数据无效（国家代码须为 2 位字母）"})
				return
			}
			switch a.PriceCurrency {
			case "", "CNY", "USD", "EUR", "JPY", "GBP", "HKD", "KRW", "SGD":
			default:
				c.JSON(http.StatusBadRequest, gin.H{"error": "Agent 数据无效（不支持的币种）"})
				return
			}
			switch a.PriceCycle {
			case "", "monthly", "yearly", "quarterly", "weekly":
			default:
				c.JSON(http.StatusBadRequest, gin.H{"error": "Agent 数据无效（周期须为 monthly/quarterly/yearly/weekly）"})
				return
			}
			if a.PriceAmount < 0 || a.PriceAmount > 1e9 || a.TrafficQuotaBytes < 0 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Agent 数据无效（费用或流量配额数值越界）"})
				return
			}
			if a.TrafficQuotaType != "" && !model.ValidQuotaTypes[a.TrafficQuotaType] {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Agent 数据无效（流量配额口径须为 sum/up/down/max/min）"})
				return
			}
			var expiresAt *time.Time
			if s := strings.TrimSpace(a.ExpiresAt); s != "" {
				t, err := time.Parse(time.RFC3339, s)
				if err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": "Agent 数据无效（到期时间须为 RFC3339 格式）"})
					return
				}
				expiresAt = &t
			}
			quotaType := a.TrafficQuotaType
			if quotaType == "" {
				quotaType = model.QuotaTypeSum
			}

			for _, m := range matches {
				meta := repository.AgentMeta{
					Region:            strings.TrimSpace(a.Region),
					CountryCode:       strings.ToUpper(strings.TrimSpace(a.CountryCode)),
					ISP:               strings.TrimSpace(a.ISP),
					ExpiresAt:         expiresAt,
					PriceAmount:       a.PriceAmount,
					PriceCurrency:     a.PriceCurrency,
					PriceCycle:        a.PriceCycle,
					TrafficQuotaBytes: a.TrafficQuotaBytes,
					TrafficQuotaType:  quotaType,
				}
				if err := h.agentRepo.UpdateMeta(m.ID, meta); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "更新 Agent 元数据失败"})
					return
				}
				if err := h.agentRepo.UpdateProfile(m.ID, a.DisplayName, a.Tags); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "更新 Agent 档案失败"})
					return
				}
				agentsUpdated++
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"message":         fmt.Sprintf("导入完成：设置 %d 项，标签新增 %d/更新 %d，Agent 更新 %d/跳过 %d", len(kv), tagsCreated, tagsUpdated, agentsUpdated, agentsSkipped),
		"tags_created":    tagsCreated,
		"tags_updated":    tagsUpdated,
		"agents_updated":  agentsUpdated,
		"agents_skipped":  agentsSkipped,
	})
}
