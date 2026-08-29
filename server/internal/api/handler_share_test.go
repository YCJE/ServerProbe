package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/server-probe/server/internal/model"
	"github.com/server-probe/server/internal/repository"
)

type shareTestEnv struct {
	handler *ShareHandler
	repo    *repository.SharePageRepository
	db      *repository.SQLiteDB
	router  *gin.Engine
}

func setupShareTest(t *testing.T) *shareTestEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := repository.NewSQLiteDB(t.TempDir())
	if err != nil {
		t.Fatalf("创建测试数据库失败: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	repo := repository.NewSharePageRepository(db.DB())
	handler := NewShareHandler(repo)

	r := gin.New()
	r.GET("/api/v1/public/share/:shareId", handler.HandlePublicSharePage)
	r.POST("/api/v1/share-pages", handler.HandleCreateSharePage)
	r.PUT("/api/v1/share-pages/:id", handler.HandleUpdateSharePage)
	return &shareTestEnv{handler: handler, repo: repo, db: db, router: r}
}

func (e *shareTestEnv) doJSON(method, path string, body any) *httptest.ResponseRecorder {
	data, _ := json.Marshal(body)
	req := httptest.NewRequest(method, path, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, req)
	return w
}

func (e *shareTestEnv) doPublicShare(shareID string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/public/share/"+shareID, nil)
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, req)
	return w
}

func decodePage(t *testing.T, w *httptest.ResponseRecorder) model.SharePage {
	t.Helper()
	var resp struct {
		Page model.SharePage `json:"page"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析响应失败: %v (%s)", err, w.Body.String())
	}
	return resp.Page
}

func TestSharePageExpiry_Create(t *testing.T) {
	e := setupShareTest(t)

	// 未来过期时间 → 创建成功，公开可访问
	future := time.Now().Add(24 * time.Hour).Format(time.RFC3339)
	w := e.doJSON(http.MethodPost, "/api/v1/share-pages", gin.H{
		"title": "临时分享", "enabled": true, "expires_at": future,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("创建带过期时间的分享页失败: %d %s", w.Code, w.Body.String())
	}
	page := decodePage(t, w)
	if page.ExpiresAt == nil {
		t.Fatalf("expires_at 未保存")
	}
	if e.doPublicShare(page.ShareID).Code != http.StatusOK {
		t.Errorf("未过期的分享页应可访问")
	}

	// 过去时间 → 400
	past := time.Now().Add(-time.Hour).Format(time.RFC3339)
	w = e.doJSON(http.MethodPost, "/api/v1/share-pages", gin.H{
		"title": "无效", "expires_at": past,
	})
	if w.Code != http.StatusBadRequest {
		t.Errorf("过去时间应返回 400, 得到 %d", w.Code)
	}

	// 不传 expires_at → 永久有效
	w = e.doJSON(http.MethodPost, "/api/v1/share-pages", gin.H{"title": "永久分享", "enabled": true})
	if w.Code != http.StatusOK {
		t.Fatalf("创建永久分享页失败: %d", w.Code)
	}
	if p := decodePage(t, w); p.ExpiresAt != nil {
		t.Errorf("未设置时应为永久（nil）")
	}
}

func TestSharePageExpiry_ExpiredReturns404(t *testing.T) {
	e := setupShareTest(t)

	w := e.doJSON(http.MethodPost, "/api/v1/share-pages", gin.H{
		"title": "临时分享", "enabled": true,
		"expires_at": time.Now().Add(24 * time.Hour).Format(time.RFC3339),
	})
	if w.Code != http.StatusOK {
		t.Fatalf("创建失败: %d", w.Code)
	}
	page := decodePage(t, w)

	// 直连 DB 将过期时间回拨到过去，模拟到期
	e.db.DB().Exec("UPDATE share_pages SET expires_at = ? WHERE id = ?",
		time.Now().Add(-time.Minute), page.ID)

	if code := e.doPublicShare(page.ShareID).Code; code != http.StatusNotFound {
		t.Errorf("过期分享页应返回 404, 得到 %d", code)
	}
}

func TestSharePageExpiry_UpdateClear(t *testing.T) {
	e := setupShareTest(t)

	// 创建带过期时间的分享页
	w := e.doJSON(http.MethodPost, "/api/v1/share-pages", gin.H{
		"title": "临时分享", "enabled": true,
		"expires_at": time.Now().Add(48 * time.Hour).Format(time.RFC3339),
	})
	page := decodePage(t, w)

	// 更新为永久（空字符串清除）
	w = e.doJSON(http.MethodPut, "/api/v1/share-pages/"+strconv.FormatInt(page.ID, 10), gin.H{
		"expires_at": "",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("更新失败: %d %s", w.Code, w.Body.String())
	}
	updated := decodePage(t, w)
	if updated.ExpiresAt != nil {
		t.Errorf("空字符串应清除过期时间（改回永久）, 得到 %v", updated.ExpiresAt)
	}

	// 更新为新过期时间
	newExpiry := time.Now().Add(72 * time.Hour).Format(time.RFC3339)
	w = e.doJSON(http.MethodPut, "/api/v1/share-pages/"+strconv.FormatInt(page.ID, 10), gin.H{
		"expires_at": newExpiry,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("更新过期时间失败: %d", w.Code)
	}
	if p := decodePage(t, w); p.ExpiresAt == nil {
		t.Errorf("新过期时间未保存")
	}
}
