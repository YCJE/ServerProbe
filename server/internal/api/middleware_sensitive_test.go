package api

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/server-probe/server/internal/model"
	"github.com/server-probe/server/internal/pkg"
	"github.com/server-probe/server/internal/repository"
	"github.com/server-probe/server/internal/service"
)

// genTOTPCode 本地实现 RFC 6238 码生成（与生产 ValidateTOTP 交叉验证，
// 避免用被测代码自身生成期望值而掩盖实现缺陷）
func genTOTPCode(t *testing.T, secret string) string {
	t.Helper()
	padded := strings.ToUpper(secret)
	if m := len(padded) % 8; m != 0 {
		padded += strings.Repeat("=", 8-m)
	}
	key, err := base32.StdEncoding.DecodeString(padded)
	if err != nil {
		t.Fatalf("解码密钥失败: %v", err)
	}
	msg := make([]byte, 8)
	binary.BigEndian.PutUint64(msg, uint64(time.Now().Unix()/30))
	mac := hmac.New(sha1.New, key)
	mac.Write(msg)
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	code := (binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff) % 1000000
	return fmt.Sprintf("%06d", code)
}

// setupSensitiveTest 创建测试 DB + 管理员 + 中间件
func setupSensitiveTest(t *testing.T, totpEnabled bool, adminID int64) (*Middleware, *repository.AdminRepository, *service.AuditService, *repository.AuditLogRepository, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := repository.NewSQLiteDB(t.TempDir())
	if err != nil {
		t.Fatalf("创建测试数据库失败: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	adminRepo := repository.NewAdminRepository(db.DB())
	secret := ""
	if totpEnabled {
		secret, err = pkg.GenerateTOTPSecret()
		if err != nil {
			t.Fatalf("生成密钥失败: %v", err)
		}
	}
	if err := adminRepo.Create(&model.Admin{
		ID:          adminID,
		Username:    fmt.Sprintf("admin%d", adminID),
		PasswordHash: "$2a$12$placeholderplaceholderplaceholderplaceholderplaceholderplaceh",
		TOTPSecret:  secret,
		TOTPEnabled: totpEnabled,
	}); err != nil {
		t.Fatalf("创建管理员失败: %v", err)
	}

	auditRepo := repository.NewAuditLogRepository(db.DB())
	auditSvc := service.NewAuditService(auditRepo)
	auditSvc.Start()
	t.Cleanup(auditSvc.Stop)

	return NewMiddleware(nil, nil), adminRepo, auditSvc, auditRepo, secret
}

// newTestContext 构造带 admin_id 的测试上下文
func newTestContext(method, path string, adminID int64, header map[string]string) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(method, path, nil)
	for k, v := range header {
		req.Header.Set(k, v)
	}
	c.Request = req
	if adminID > 0 {
		c.Set("admin_id", adminID)
	}
	return c, w
}

func TestRequireSensitive2FA(t *testing.T) {
	// 每个 case 使用独立 adminID，避免 consumeTOTPStep 全局步进状态串扰
	t.Run("未启用 TOTP 直接放行", func(t *testing.T) {
		mw, adminRepo, _, _, _ := setupSensitiveTest(t, false, 101)
		c, w := newTestContext(http.MethodGet, "/api/v1/agents/1/token", 101, nil)
		mw.RequireSensitive2FA(adminRepo)(c)
		if c.IsAborted() {
			t.Errorf("未启用 TOTP 应放行, got %d", w.Code)
		}
	})

	t.Run("启用 TOTP 无验证码拒绝", func(t *testing.T) {
		mw, adminRepo, _, _, _ := setupSensitiveTest(t, true, 102)
		c, w := newTestContext(http.MethodDelete, "/api/v1/agents/1", 102, nil)
		mw.RequireSensitive2FA(adminRepo)(c)
		if !c.IsAborted() || w.Code != http.StatusForbidden {
			t.Fatalf("应返回 403, got %d", w.Code)
		}
		if !strings.Contains(w.Body.String(), "totp_required") {
			t.Errorf("响应应携带 code=totp_required: %s", w.Body.String())
		}
	})

	t.Run("启用 TOTP 错误验证码拒绝", func(t *testing.T) {
		mw, adminRepo, _, _, _ := setupSensitiveTest(t, true, 103)
		c, w := newTestContext(http.MethodDelete, "/api/v1/agents/1", 103,
			map[string]string{"X-TOTP-Code": "000000"})
		mw.RequireSensitive2FA(adminRepo)(c)
		if !c.IsAborted() || w.Code != http.StatusForbidden {
			t.Fatalf("错误验证码应返回 403, got %d", w.Code)
		}
	})

	t.Run("有效验证码放行", func(t *testing.T) {
		mw, adminRepo, _, _, secret := setupSensitiveTest(t, true, 104)
		code := genTOTPCode(t, secret)
		c, w := newTestContext(http.MethodDelete, "/api/v1/agents/1", 104,
			map[string]string{"X-TOTP-Code": code})
		mw.RequireSensitive2FA(adminRepo)(c)
		if c.IsAborted() {
			t.Errorf("有效验证码应放行, got %d %s", w.Code, w.Body.String())
		}
	})

	t.Run("同一验证码重放拒绝", func(t *testing.T) {
		mw, adminRepo, _, _, secret := setupSensitiveTest(t, true, 105)
		code := genTOTPCode(t, secret)

		c1, _ := newTestContext(http.MethodDelete, "/api/v1/agents/1", 105,
			map[string]string{"X-TOTP-Code": code})
		mw.RequireSensitive2FA(adminRepo)(c1)
		if c1.IsAborted() {
			t.Fatalf("首次使用应放行")
		}

		c2, w2 := newTestContext(http.MethodDelete, "/api/v1/agents/1", 105,
			map[string]string{"X-TOTP-Code": code})
		mw.RequireSensitive2FA(adminRepo)(c2)
		if !c2.IsAborted() || w2.Code != http.StatusForbidden {
			t.Fatalf("重放应返回 403, got %d", w2.Code)
		}
	})

	t.Run("账户不存在返回 401", func(t *testing.T) {
		mw, adminRepo, _, _, _ := setupSensitiveTest(t, false, 106)
		c, w := newTestContext(http.MethodDelete, "/api/v1/agents/1", 99999, nil)
		mw.RequireSensitive2FA(adminRepo)(c)
		if !c.IsAborted() || w.Code != http.StatusUnauthorized {
			t.Fatalf("账户不存在应返回 401, got %d", w.Code)
		}
	})
}

func TestAuditMutations(t *testing.T) {
	mw, adminRepo, auditSvc, auditRepo, _ := setupSensitiveTest(t, false, 201)

	waitFor := func(t *testing.T, want int64) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			_, total, err := auditRepo.List(repository.AuditLogQuery{Page: 1, PageSize: 100})
			if err == nil && total >= want {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("等待审计日志落库超时")
	}

	t.Run("POST 请求被审计", func(t *testing.T) {
		c, _ := newTestContext(http.MethodPost, "/api/v1/agents", 201, nil)
		c.Request.URL.Path = "/api/v1/agents"
		mw.AuditMutations(auditSvc, adminRepo)(c)
		waitFor(t, 1)

		logs, _, _ := auditRepo.List(repository.AuditLogQuery{Page: 1, PageSize: 10})
		if len(logs) == 0 {
			t.Fatal("POST 请求未被审计")
		}
		if logs[0].Action != "POST /api/v1/agents" {
			t.Errorf("action=%s, want 'POST /api/v1/agents'", logs[0].Action)
		}
		if logs[0].Username != "admin201" {
			t.Errorf("username=%s, want admin201", logs[0].Username)
		}
		if !logs[0].Success {
			t.Errorf("默认 200 状态应记为成功")
		}
	})

	t.Run("普通 GET 不被审计", func(t *testing.T) {
		c, _ := newTestContext(http.MethodGet, "/api/v1/servers", 201, nil)
		mw.AuditMutations(auditSvc, adminRepo)(c)
		time.Sleep(150 * time.Millisecond)
		_, total, _ := auditRepo.List(repository.AuditLogQuery{Page: 1, PageSize: 100})
		if total != 1 {
			t.Errorf("GET 不应产生审计记录: total=%d, want 1", total)
		}
	})

	t.Run("敏感 GET 被审计", func(t *testing.T) {
		c, _ := newTestContext(http.MethodGet, "/api/v1/agents/1/token", 201, nil)
		c.Set("sensitive_2fa", true) // 模拟 RequireSensitive2FA 的标记
		mw.AuditMutations(auditSvc, adminRepo)(c)
		waitFor(t, 2)

		logs, _, _ := auditRepo.List(repository.AuditLogQuery{Page: 1, PageSize: 10})
		if len(logs) < 2 {
			t.Fatalf("敏感 GET 应被审计: len=%d", len(logs))
		}
		if logs[0].Action != "GET /api/v1/agents/1/token" {
			t.Errorf("action=%s", logs[0].Action)
		}
	})
}
