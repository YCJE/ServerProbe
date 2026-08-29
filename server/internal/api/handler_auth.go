package api

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/server-probe/server/internal/model"
	"github.com/server-probe/server/internal/pkg"
	"github.com/server-probe/server/internal/repository"
	"github.com/server-probe/server/internal/service"
	"golang.org/x/crypto/bcrypt"
)

// AuthHandler 认证处理器
type AuthHandler struct {
	adminRepo   *repository.AdminRepository
	sessionRepo *repository.SessionRepository
	jwtManager  *pkg.JWTManager
	auditSvc    *service.AuditService
}

// NewAuthHandler 创建认证处理器
func NewAuthHandler(adminRepo *repository.AdminRepository, sessionRepo *repository.SessionRepository, jwtManager *pkg.JWTManager, auditSvc *service.AuditService) *AuthHandler {
	return &AuthHandler{
		adminRepo:   adminRepo,
		sessionRepo: sessionRepo,
		jwtManager:  jwtManager,
		auditSvc:    auditSvc,
	}
}

// issueSession 创建会话记录并签发绑定会话的 JWT，写入 Cookie
// 返回 error 时已写响应；成功时由调用方返回自己的成功响应
func (h *AuthHandler) issueSession(c *gin.Context, adminID int64) error {
	sessionID, err := repository.GenerateSessionID()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "生成会话失败"})
		return err
	}

	expiresAt := time.Now().Add(h.jwtManager.Expiry())
	session := &model.Session{
		SessionID:  sessionID,
		AdminID:    adminID,
		IP:         c.ClientIP(),
		UserAgent:  c.Request.UserAgent(),
		LastSeenAt: time.Now(),
		ExpiresAt:  expiresAt,
	}
	if h.sessionRepo != nil {
		if err := h.sessionRepo.Create(session); err != nil {
			log.Printf("创建会话记录失败: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "创建会话失败"})
			return err
		}
	}

	token, err := h.jwtManager.GenerateToken(adminID, sessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "生成 Token 失败"})
		return err
	}

	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie("token", token, int(h.jwtManager.Expiry()/time.Second), "/", "", cookieSecure(), true)
	return nil
}

// auditLogin 记录登录事件审计日志（含失败尝试，供暴力破解事后溯源）
func (h *AuthHandler) auditLogin(c *gin.Context, adminID int64, username string, success bool) {
	if h.auditSvc == nil {
		return
	}
	h.auditSvc.Record(model.AuditLog{
		AdminID:   adminID,
		Username:  username,
		Action:    "auth.login",
		Target:    username,
		Success:   success,
		IP:        c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
	})
}

// SetupRequest 首次设置请求
type SetupRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// LoginRequest 登录请求
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	TOTPCode string `json:"totp_code"`
}

// LoginResponse 登录响应
// Token 不再通过 JSON 返回，仅通过 HttpOnly Cookie 传递，防止 XSS 窃取
type LoginResponse struct {
	Success  bool   `json:"success"`
	Message  string `json:"message"`
	NeedTOTP bool   `json:"need_totp"`
}

// validateUsername 验证用户名格式: 长度 3-32，仅允许字母、数字、下划线和连字符
func validateUsername(username string) error {
	if len(username) < 3 || len(username) > 32 {
		return fmt.Errorf("用户名长度必须在 3-32 个字符之间")
	}
	for _, r := range username {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_' || r == '-':
		default:
			return fmt.Errorf("用户名只能包含字母、数字、下划线和连字符")
		}
	}
	return nil
}

// dummyPasswordHash 用于在用户名不存在时执行一次等价耗时的 bcrypt 比较，
// 消除基于响应时间的用户名枚举时序侧信道。cost 与真实管理员密码哈希一致 (12)，
// 在包初始化时生成一次。
var dummyPasswordHash = func() []byte {
	hash, err := bcrypt.GenerateFromPassword([]byte("dummy"), 12)
	if err != nil {
		panic(err)
	}
	return hash
}()

// usedTOTPSteps 记录每个管理员最近成功消费的 TOTP 时间步（adminID → step）
// RFC 6238 §5.2 要求验证码一次性使用：同一时间步的验证码验证成功后不得再次接受（防重放）
var usedTOTPSteps sync.Map

// consumeTOTPStep 验证 TOTP 验证码并消费其时间步
// 验证码无效、或所属时间步已被消费（重放）时返回 false
func consumeTOTPStep(adminID int64, secret, code string) bool {
	ok, step := pkg.ValidateTOTPWithStep(secret, code)
	if !ok {
		return false
	}
	if v, loaded := usedTOTPSteps.LoadOrStore(adminID, step); loaded {
		if lastStep, _ := v.(int64); step <= lastStep {
			return false
		}
		usedTOTPSteps.Store(adminID, step)
	}
	return true
}

// HandleLogin 处理登录
// 路由: POST /api/v1/auth/login
func (h *AuthHandler) HandleLogin(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效"})
		return
	}

	// 验证用户名格式 (格式非法时直接返回通用错误，避免无效用户名查询数据库)
	if err := validateUsername(req.Username); err != nil {
		c.JSON(http.StatusUnauthorized, LoginResponse{
			Success: false,
			Message: "用户名或密码错误",
		})
		return
	}

	admin, err := h.adminRepo.GetByUsername(req.Username)
	if err != nil {
		// 执行 dummy bcrypt 比较以消除时序侧信道，使“用户名不存在”与“密码错误”的响应时间一致
		_ = bcrypt.CompareHashAndPassword(dummyPasswordHash, []byte("dummy"))
		h.auditLogin(c, 0, req.Username, false)
		c.JSON(http.StatusUnauthorized, LoginResponse{
			Success: false,
			Message: "用户名或密码错误",
		})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(req.Password)); err != nil {
		h.auditLogin(c, admin.ID, req.Username, false)
		c.JSON(http.StatusUnauthorized, LoginResponse{
			Success: false,
			Message: "用户名或密码错误",
		})
		return
	}

	// 检查是否需要 TOTP
	if admin.TOTPEnabled {
		if req.TOTPCode == "" {
			c.JSON(http.StatusOK, LoginResponse{
				Success:  false,
				NeedTOTP: true,
				Message:  "需要两步验证",
			})
			return
		}

		if !consumeTOTPStep(admin.ID, admin.TOTPSecret, req.TOTPCode) {
			h.auditLogin(c, admin.ID, req.Username, false)
			c.JSON(http.StatusUnauthorized, LoginResponse{
				Success: false,
				Message: "两步验证码错误",
			})
			return
		}
	}

	// 创建会话并签发绑定会话的 JWT（P2：会话管理）
	// 标记登录成功，供 LoginRateLimit 中间件移除限速计数
	c.Set("login_success", true)
	h.auditLogin(c, admin.ID, req.Username, true)

	if err := h.issueSession(c, admin.ID); err != nil {
		return
	}

	c.JSON(http.StatusOK, LoginResponse{
		Success: true,
		Message: "登录成功",
	})
}

// HandleLogout 处理登出
// 路由: POST /api/v1/auth/logout
// 防止 Logout CSRF：仅当请求携带了有效 Token Cookie 时才清除 Cookie
// P2：同时撤销会话记录，使该 Token 在其他持有者手中也立即失效
func (h *AuthHandler) HandleLogout(c *gin.Context) {
	tokenString, err := c.Cookie("token")
	if err != nil || tokenString == "" {
		// 无 Cookie（可能是跨站攻击），返回成功但不清除 Cookie
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "已登出"})
		return
	}
	// 撤销会话记录（Token 即使未到期也立即失效）
	if h.sessionRepo != nil {
		if claims, err := h.jwtManager.ValidateToken(tokenString); err == nil && claims.ID != "" {
			if err := h.sessionRepo.Revoke(claims.ID); err != nil {
				log.Printf("撤销会话失败: %v", err)
			}
		}
	}
	// 有 Cookie，验证并清除
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie("token", "", -1, "/", "", cookieSecure(), true)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "已登出"})
}

// HandleSetup 处理首次设置（创建管理员账户）
// 路由: POST /api/v1/auth/setup
func (h *AuthHandler) HandleSetup(c *gin.Context) {
	// 检查是否已有管理员（快速路径，事务外预检查）
	count, err := h.adminRepo.Count()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "检查管理员失败"})
		return
	}
	if count > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "管理员账户已存在"})
		return
	}

	var req SetupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}

	// 验证用户名格式 (长度 3-32，仅允许字母、数字、下划线和连字符)
	if err := validateUsername(req.Username); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 验证密码强度
	if err := pkg.ValidatePasswordStrength(req.Password); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 哈希密码
	hash, err := pkg.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "密码哈希失败"})
		return
	}

	// 创建管理员（事务内再次检查，防止 TOCTOU 竞态条件导致创建多个管理员）
	admin := &model.Admin{
		Username:     req.Username,
		PasswordHash: hash,
	}

	if err := h.adminRepo.CreateFirstAdmin(admin); err != nil {
		if errors.Is(err, repository.ErrAdminAlreadyExists) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "管理员账户已存在"})
			return
		}
		log.Printf("创建管理员失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建管理员失败"})
		return
	}

	// 审计首次设置事件（仅发生一次，是账户体系的起点）
	if h.auditSvc != nil {
		h.auditSvc.Record(model.AuditLog{
			AdminID:   admin.ID,
			Username:  admin.Username,
			Action:    "auth.setup",
			Target:    admin.Username,
			Success:   true,
			IP:        c.ClientIP(),
			UserAgent: c.Request.UserAgent(),
		})
	}

	// 创建会话并签发绑定会话的 JWT（P2）
	if err := h.issueSession(c, admin.ID); err != nil {
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "管理员账户创建成功",
	})
}

// HandleCheckAuth 检查当前登录状态
// 路由: GET /api/v1/auth/me
// 供前端在页面加载时检查 Cookie 中的 Token 是否仍然有效
// P2：除 JWT 签名外还校验会话记录（登出/远程撤销后立即返回未认证）
func (h *AuthHandler) HandleCheckAuth(c *gin.Context) {
	tokenString, err := c.Cookie("token")
	if err != nil || tokenString == "" {
		c.JSON(http.StatusOK, gin.H{"authenticated": false})
		return
	}

	claims, err := h.jwtManager.ValidateToken(tokenString)
	if err != nil {
		// Token 过期或无效，清除 Cookie
		c.SetSameSite(http.SameSiteStrictMode)
		c.SetCookie("token", "", -1, "/", "", cookieSecure(), true)
		c.JSON(http.StatusOK, gin.H{"authenticated": false})
		return
	}

	// 会话记录校验（会话不存在/已撤销/已过期 → 视为未登录）
	if h.sessionRepo != nil {
		if claims.ID == "" {
			c.SetSameSite(http.SameSiteStrictMode)
			c.SetCookie("token", "", -1, "/", "", cookieSecure(), true)
			c.JSON(http.StatusOK, gin.H{"authenticated": false})
			return
		}
		s, err := h.sessionRepo.GetBySessionID(claims.ID)
		if err != nil || s.RevokedAt != nil || time.Now().After(s.ExpiresAt) {
			c.SetSameSite(http.SameSiteStrictMode)
			c.SetCookie("token", "", -1, "/", "", cookieSecure(), true)
			c.JSON(http.StatusOK, gin.H{"authenticated": false})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"authenticated": true,
	})
}

// cookieSecure 返回 Cookie 的 Secure 标志
// 生产环境（COOKIE_SECURE=true 或 GIN_MODE=release）强制 true，开发环境 false
func cookieSecure() bool {
	return os.Getenv("COOKIE_SECURE") == "true" || os.Getenv("GIN_MODE") == "release"
}

// HandleCheckSetup 检查是否需要初始化
// 路由: GET /api/v1/auth/setup-status
func (h *AuthHandler) HandleCheckSetup(c *gin.Context) {
	log.Printf("[SetupCheck] 请求来自 %s", c.ClientIP())

	count, err := h.adminRepo.Count()
	if err != nil {
		log.Printf("[SetupCheck] 查询管理员数量失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "检查失败"})
		return
	}

	log.Printf("[SetupCheck] 管理员数量: %d, needs_setup: %v", count, count == 0)

	c.JSON(http.StatusOK, gin.H{
		"needs_setup": count == 0,
	})
}

// HandleTOTPStatus 查询当前 TOTP 绑定状态
// 路由: GET /api/v1/auth/totp/status
func (h *AuthHandler) HandleTOTPStatus(c *gin.Context) {
	admin, err := h.adminRepo.GetByID(c.GetInt64("admin_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "账户不存在"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"totp_enabled": admin.TOTPEnabled})
}

// HandleTOTPSetup 生成 TOTP 密钥（未启用状态，需验证一次动态码后才生效）
// 路由: POST /api/v1/auth/totp/setup
func (h *AuthHandler) HandleTOTPSetup(c *gin.Context) {
	admin, err := h.adminRepo.GetByID(c.GetInt64("admin_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "账户不存在"})
		return
	}

	if admin.TOTPEnabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "两步验证已启用，如需重新绑定请先停用"})
		return
	}

	secret, err := pkg.GenerateTOTPSecret()
	if err != nil {
		log.Printf("生成 TOTP 密钥失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "生成密钥失败"})
		return
	}

	// 仅写入密钥，TOTPEnabled 保持 false，验证通过后才真正启用
	admin.TOTPSecret = secret
	admin.TOTPEnabled = false
	if err := h.adminRepo.Update(admin); err != nil {
		log.Printf("保存 TOTP 密钥失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存密钥失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"secret":      secret,
		"otpauth_url": pkg.GenerateOTPAuthURL(secret, admin.Username, "ServerProbe"),
	})
}

// HandleTOTPEnable 验证动态码并启用两步验证
// 路由: POST /api/v1/auth/totp/enable
func (h *AuthHandler) HandleTOTPEnable(c *gin.Context) {
	var req struct {
		Code string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请输入验证码"})
		return
	}

	admin, err := h.adminRepo.GetByID(c.GetInt64("admin_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "账户不存在"})
		return
	}

	if admin.TOTPEnabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "两步验证已启用"})
		return
	}
	if admin.TOTPSecret == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请先生成密钥"})
		return
	}

	if !consumeTOTPStep(admin.ID, admin.TOTPSecret, req.Code) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "验证码错误，请确认认证器时间同步后重试"})
		return
	}

	admin.TOTPEnabled = true
	if err := h.adminRepo.Update(admin); err != nil {
		log.Printf("启用 TOTP 失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "启用失败"})
		return
	}

	// 认证配置变更后，其他设备上的旧会话不应存活（P2）
	h.revokeOtherSessions(c, admin.ID)

	log.Printf("管理员 %s 启用了 TOTP 两步验证", admin.Username)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "两步验证已启用，其他设备的登录状态已注销"})
}

// HandleTOTPDisable 停用两步验证（密码 + 动态码双重确认）
// 路由: POST /api/v1/auth/totp/disable
// 密码确认防止会话被劫持后直接关闭；已启用 TOTP 的账户还需当前动态码
// （关闭 2FA 本身就是最敏感的降级操作，要求当前持有认证器）
func (h *AuthHandler) HandleTOTPDisable(c *gin.Context) {
	var req struct {
		Password string `json:"password" binding:"required"`
		TOTPCode string `json:"totp_code"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请输入密码确认"})
		return
	}

	admin, err := h.adminRepo.GetByID(c.GetInt64("admin_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "账户不存在"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "密码错误"})
		return
	}

	// 已启用 TOTP 的账户必须同时提供有效动态码
	if admin.TOTPEnabled {
		if req.TOTPCode == "" || !consumeTOTPStep(admin.ID, admin.TOTPSecret, req.TOTPCode) {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "停用两步验证需要当前动态码确认",
				"code":  "totp_required",
			})
			return
		}
	}

	admin.TOTPSecret = ""
	admin.TOTPEnabled = false
	if err := h.adminRepo.Update(admin); err != nil {
		log.Printf("停用 TOTP 失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "停用失败"})
		return
	}

	// 认证配置变更后，其他设备上的旧会话不应存活（P2）
	h.revokeOtherSessions(c, admin.ID)

	log.Printf("管理员 %s 停用了 TOTP 两步验证", admin.Username)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "两步验证已停用，其他设备的登录状态已注销"})
}

// revokeOtherSessions 撤销当前会话之外的全部会话（认证配置变更联动）
func (h *AuthHandler) revokeOtherSessions(c *gin.Context, adminID int64) {
	if h.sessionRepo == nil {
		return
	}
	currentSessionID, _ := c.Get("session_id")
	keep, _ := currentSessionID.(string)
	if n, err := h.sessionRepo.RevokeAllOther(adminID, keep); err != nil {
		log.Printf("撤销其他会话失败: %v", err)
	} else if n > 0 {
		log.Printf("已撤销管理员 %d 的 %d 个其他会话", adminID, n)
	}
}

// HandleListSessions 查询当前管理员的全部会话
// 路由: GET /api/v1/auth/sessions
func (h *AuthHandler) HandleListSessions(c *gin.Context) {
	if h.sessionRepo == nil {
		c.JSON(http.StatusOK, gin.H{"sessions": []any{}})
		return
	}
	sessions, err := h.sessionRepo.ListByAdmin(c.GetInt64("admin_id"))
	if err != nil {
		log.Printf("查询会话列表失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询会话列表失败"})
		return
	}
	currentSessionID, _ := c.Get("session_id")
	current, _ := currentSessionID.(string)

	type sessionItem struct {
		model.Session
		Current bool `json:"current"`
		Revoked bool `json:"revoked"`
	}
	items := make([]sessionItem, 0, len(sessions))
	now := time.Now()
	for _, s := range sessions {
		items = append(items, sessionItem{
			Session: s,
			Current: s.SessionID == current,
			Revoked: s.RevokedAt != nil || now.After(s.ExpiresAt),
		})
	}
	c.JSON(http.StatusOK, gin.H{"sessions": items})
}

// HandleRevokeSession 撤销指定会话（盗号踢出）
// 路由: DELETE /api/v1/auth/sessions/:sessionId
func (h *AuthHandler) HandleRevokeSession(c *gin.Context) {
	if h.sessionRepo == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "会话管理未启用"})
		return
	}
	sessionID := c.Param("sessionId")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少会话 ID"})
		return
	}

	// 仅允许撤销自己的会话（校验归属，防止越权撤销他人会话）
	target, err := h.sessionRepo.GetBySessionID(sessionID)
	if err != nil || target.AdminID != c.GetInt64("admin_id") {
		c.JSON(http.StatusNotFound, gin.H{"error": "会话不存在"})
		return
	}

	if err := h.sessionRepo.Revoke(sessionID); err != nil {
		log.Printf("撤销会话失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "撤销会话失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "会话已注销"})
}

// HandleRevokeOtherSessions 撤销除当前会话外的全部会话（"我被盗号了想踢掉所有会话"）
// 路由: POST /api/v1/auth/sessions/revoke-others
func (h *AuthHandler) HandleRevokeOtherSessions(c *gin.Context) {
	if h.sessionRepo == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "会话管理未启用"})
		return
	}
	currentSessionID, _ := c.Get("session_id")
	current, _ := currentSessionID.(string)

	n, err := h.sessionRepo.RevokeAllOther(c.GetInt64("admin_id"), current)
	if err != nil {
		log.Printf("撤销其他会话失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "撤销其他会话失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": fmt.Sprintf("已注销其他 %d 个会话", n), "revoked": n})
}
