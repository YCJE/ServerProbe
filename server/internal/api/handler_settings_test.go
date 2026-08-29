package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/server-probe/server/internal/model"
	"github.com/server-probe/server/internal/repository"
	"github.com/server-probe/server/internal/service"
)

type settingsTestEnv struct {
	handler  *SettingsHandler
	settings *service.SettingsService
	agentRepo *repository.AgentRepository
	tagRepo   *repository.TagRepository
	adminRepo *repository.AdminRepository
	notifyRepo *repository.NotifyRepository
	db       *repository.SQLiteDB
	router   *gin.Engine
}

func setupSettingsTest(t *testing.T) *settingsTestEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := repository.NewSQLiteDB(t.TempDir())
	if err != nil {
		t.Fatalf("创建测试数据库失败: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	settingRepo := repository.NewSettingRepository(db.DB())
	settingsSvc, err := service.NewSettingsService(settingRepo)
	if err != nil {
		t.Fatalf("初始化设置服务失败: %v", err)
	}

	agentRepo := repository.NewAgentRepository(db.DB())
	tagRepo := repository.NewTagRepository(db.DB())
	handler := NewSettingsHandler(settingsSvc, nil, nil, nil, tagRepo, agentRepo, db.DB(), t.TempDir())

	r := gin.New()
	r.GET("/api/v1/settings/export", handler.HandleExportSettings)
	r.POST("/api/v1/settings/import", handler.HandleImportSettings)
	r.GET("/api/v1/settings", handler.HandleGetSettings)
	return &settingsTestEnv{
		handler:    handler,
		settings:   settingsSvc,
		agentRepo:  agentRepo,
		tagRepo:    tagRepo,
		adminRepo:  repository.NewAdminRepository(db.DB()),
		notifyRepo: repository.NewNotifyRepository(db.DB()),
		db:         db,
		router:     r,
	}
}

func (e *settingsTestEnv) doJSON(method, path string, body any) *httptest.ResponseRecorder {
	var reader *bytes.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		reader = bytes.NewReader(data)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, req)
	return w
}

// seedExportData 准备待导出的数据：设置 + 标签 + Agent 元数据 + 敏感凭证（不应被导出）
func (e *settingsTestEnv) seedExportData(t *testing.T) {
	t.Helper()
	if err := e.settings.Update(map[string]string{
		service.SettingSiteTitle:   "测试站点",
		service.SettingAnnouncement: "导出测试公告",
	}); err != nil {
		t.Fatalf("写入设置失败: %v", err)
	}
	if err := e.tagRepo.Create(&model.Tag{Name: "web", Color: "#3b82f6"}); err != nil {
		t.Fatalf("创建标签失败: %v", err)
	}
	expires := time.Now().AddDate(1, 0, 0).UTC()
	if err := e.agentRepo.Create(&model.Agent{
		Token:             "agent-secret-token-xyz",
		Hostname:          "vps-1",
		DisplayName:       "生产机",
		Tags:              "web",
		Region:            "上海",
		CountryCode:       "CN",
		ISP:               "Bandwagon",
		ExpiresAt:         &expires,
		PriceAmount:       5,
		PriceCurrency:     "CNY",
		PriceCycle:        "monthly",
		TrafficQuotaBytes: 1 << 30,
		TrafficQuotaType:  model.QuotaTypeMax,
	}); err != nil {
		t.Fatalf("创建 Agent 失败: %v", err)
	}
	if err := e.adminRepo.Create(&model.Admin{
		Username:     "admin",
		PasswordHash: "bcrypt-hash-secret",
		TOTPSecret:   "totp-secret-abc",
	}); err != nil {
		t.Fatalf("创建管理员失败: %v", err)
	}
	if err := e.notifyRepo.Create(&model.NotifyChannel{
		Name:   "webhook",
		Type:   "webhook",
		Config: `{"url":"https://example.com/hook","secret":"webhook-secret-123"}`,
	}); err != nil {
		t.Fatalf("创建通知渠道失败: %v", err)
	}
}

func TestSettingsExport_NoSensitiveFields(t *testing.T) {
	e := setupSettingsTest(t)
	e.seedExportData(t)

	w := e.doJSON(http.MethodGet, "/api/v1/settings/export", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("导出失败: %d %s", w.Code, w.Body.String())
	}

	body := w.Body.String()
	// 敏感凭证绝不出现在导出文件中
	for _, secret := range []string{
		"agent-secret-token-xyz", // Agent Token
		"bcrypt-hash-secret",     // 管理员密码哈希
		"totp-secret-abc",        // TOTP 密钥
		"webhook-secret-123",     // 通知渠道凭证
	} {
		if bytes.Contains(w.Body.Bytes(), []byte(secret)) {
			t.Errorf("导出文件泄露敏感值: %q", secret)
		}
	}

	// 非敏感数据应完整导出
	var file struct {
		Version int    `json:"version"`
		Settings struct {
			SiteTitle    string `json:"site_title"`
			Announcement string `json:"announcement"`
		} `json:"settings"`
		Tags []struct {
			Name  string `json:"name"`
			Color string `json:"color"`
		} `json:"tags"`
		Agents []struct {
			Hostname         string `json:"hostname"`
			Region           string `json:"region"`
			TrafficQuotaType string `json:"traffic_quota_type"`
		} `json:"agents"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &file); err != nil {
		t.Fatalf("解析导出文件失败: %v (%s)", err, body)
	}
	if file.Version != 1 {
		t.Errorf("导出版本应为 1, 得到 %d", file.Version)
	}
	if file.Settings.SiteTitle != "测试站点" || file.Settings.Announcement != "导出测试公告" {
		t.Errorf("站点设置未导出: %+v", file.Settings)
	}
	if len(file.Tags) != 1 || file.Tags[0].Name != "web" || file.Tags[0].Color != "#3b82f6" {
		t.Errorf("标签未导出: %+v", file.Tags)
	}
	if len(file.Agents) != 1 || file.Agents[0].Hostname != "vps-1" ||
		file.Agents[0].Region != "上海" || file.Agents[0].TrafficQuotaType != "max" {
		t.Errorf("Agent 元数据未导出: %+v", file.Agents)
	}
}

func TestSettingsImport_RestoresData(t *testing.T) {
	e := setupSettingsTest(t)
	e.seedExportData(t)

	// 导出
	w := e.doJSON(http.MethodGet, "/api/v1/settings/export", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("导出失败: %d", w.Code)
	}
	var file map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &file); err != nil {
		t.Fatalf("解析导出文件失败: %v", err)
	}

	// 破坏现场：改设置、删标签、抹掉 Agent 元数据
	if err := e.settings.Update(map[string]string{
		service.SettingSiteTitle: "被覆盖的标题",
	}); err != nil {
		t.Fatalf("修改设置失败: %v", err)
	}
	tags, _ := e.tagRepo.List()
	for _, tag := range tags {
		_ = e.tagRepo.Delete(tag.ID)
	}
	agents, _ := e.agentRepo.List()
	for _, a := range agents {
		_ = e.agentRepo.UpdateMeta(a.ID, repository.AgentMeta{
			Region: "", CountryCode: "", ISP: "", TrafficQuotaType: model.QuotaTypeSum,
		})
		_ = e.agentRepo.UpdateProfile(a.ID, "", "")
	}

	// 导入恢复
	w = e.doJSON(http.MethodPost, "/api/v1/settings/import", file)
	if w.Code != http.StatusOK {
		t.Fatalf("导入失败: %d %s", w.Code, w.Body.String())
	}

	if got := e.settings.SiteTitle(); got != "测试站点" {
		t.Errorf("设置未恢复: 期望 %q, 得到 %q", "测试站点", got)
	}
	tags, _ = e.tagRepo.List()
	if len(tags) != 1 || tags[0].Name != "web" || tags[0].Color != "#3b82f6" {
		t.Errorf("标签未恢复: %+v", tags)
	}
	agents, _ = e.agentRepo.List()
	if len(agents) != 1 {
		t.Fatalf("Agent 数量异常: %d", len(agents))
	}
	a := agents[0]
	if a.DisplayName != "生产机" || a.Region != "上海" || a.CountryCode != "CN" ||
		a.ISP != "Bandwagon" || a.Tags != "web" ||
		a.PriceAmount != 5 || a.PriceCurrency != "CNY" || a.PriceCycle != "monthly" ||
		a.TrafficQuotaBytes != 1<<30 || a.TrafficQuotaType != model.QuotaTypeMax {
		t.Errorf("Agent 元数据未恢复: %+v", a)
	}
	if a.ExpiresAt == nil {
		t.Errorf("到期时间未恢复")
	}
	// Token 属接入凭证，导入不得触碰
	if a.Token != "agent-secret-token-xyz" {
		t.Errorf("导入不应修改 Token")
	}
}

func TestSettingsImport_SkipUnknownAgentAndMergeTag(t *testing.T) {
	e := setupSettingsTest(t)

	// 现场仅有一台 vps-1 与一个旧标签（颜色不同）
	if err := e.agentRepo.Create(&model.Agent{Token: "t1", Hostname: "vps-1"}); err != nil {
		t.Fatalf("创建 Agent 失败: %v", err)
	}
	if err := e.tagRepo.Create(&model.Tag{Name: "web", Color: "#111111"}); err != nil {
		t.Fatalf("创建标签失败: %v", err)
	}

	file := gin.H{
		"version":     1,
		"exported_at": time.Now().Format(time.RFC3339),
		"settings": gin.H{
			"site_title":            "导入站",
			"site_description":      "",
			"announcement":          "",
			"custom_footer":         "",
			"default_history_range": "1h",
			"offline_grace_seconds": 90,
			"retention_days":        30,
			"retention_days_hourly": 730,
			"max_chart_points":      800,
		},
		"tags": []gin.H{{"name": "web", "color": "#3b82f6"}, {"name": "new-tag", "color": "#ff0000"}},
		"agents": []gin.H{
			{"hostname": "vps-1", "display_name": "改名机", "traffic_quota_type": "down"},
			{"hostname": "ghost-machine", "display_name": "不存在的机器"},
		},
	}

	w := e.doJSON(http.MethodPost, "/api/v1/settings/import", file)
	if w.Code != http.StatusOK {
		t.Fatalf("导入失败: %d %s", w.Code, w.Body.String())
	}
	var resp struct {
		TagsCreated   int `json:"tags_created"`
		TagsUpdated   int `json:"tags_updated"`
		AgentsUpdated int `json:"agents_updated"`
		AgentsSkipped int `json:"agents_skipped"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	if resp.TagsCreated != 1 || resp.TagsUpdated != 1 {
		t.Errorf("标签合并计数错误: %+v", resp)
	}
	if resp.AgentsUpdated != 1 || resp.AgentsSkipped != 1 {
		t.Errorf("Agent 计数错误: %+v", resp)
	}

	// 已存在标签按名称合并（更新颜色），新标签创建
	tags, _ := e.tagRepo.List()
	byName := map[string]string{}
	for _, tag := range tags {
		byName[tag.Name] = tag.Color
	}
	if byName["web"] != "#3b82f6" || byName["new-tag"] != "#ff0000" {
		t.Errorf("标签合并结果错误: %+v", byName)
	}

	// 未知 hostname 跳过，已知 Agent 更新
	agents, _ := e.agentRepo.List()
	if len(agents) != 1 || agents[0].DisplayName != "改名机" || agents[0].TrafficQuotaType != model.QuotaTypeDown {
		t.Errorf("Agent 更新结果错误: %+v", agents)
	}
}

func TestSettingsImport_RejectsBadInput(t *testing.T) {
	e := setupSettingsTest(t)

	// 版本不兼容
	w := e.doJSON(http.MethodPost, "/api/v1/settings/import", gin.H{
		"version":  99,
		"settings": gin.H{},
	})
	if w.Code != http.StatusBadRequest {
		t.Errorf("不支持的版本应返回 400, 得到 %d", w.Code)
	}

	// 无效请求体
	w = e.doJSON(http.MethodPost, "/api/v1/settings/import", gin.H{
		"version": "1", "settings": gin.H{},
	})
	if w.Code != http.StatusBadRequest {
		t.Errorf("无效请求体应返回 400, 得到 %d", w.Code)
	}
}
