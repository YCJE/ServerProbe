package api

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/server-probe/server/internal/model"
	"github.com/server-probe/server/internal/pkg"
	"github.com/server-probe/server/internal/repository"
	"golang.org/x/crypto/bcrypt"
)

// AuthHandler 认证处理器
type AuthHandler struct {
	adminRepo   *repository.AdminRepository
	jwtManager  *pkg.JWTManager
}

// NewAuthHandler 创建认证处理器
func NewAuthHandler(adminRepo *repository.AdminRepository, jwtManager *pkg.JWTManager) *AuthHandler {
	return &AuthHandler{
		adminRepo:  adminRepo,
		jwtManager: jwtManager,
	}
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
		c.JSON(http.StatusUnauthorized, LoginResponse{
			Success: false,
			Message: "用户名或密码错误",
		})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(req.Password)); err != nil {
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

		// TOTP 验证暂未实现，拒绝登录而非跳过
		c.JSON(http.StatusUnauthorized, LoginResponse{
			Success: false,
			Message: "两步验证未配置，请联系管理员重置账户",
		})
		return
	}

	// 生成 JWT
	token, err := h.jwtManager.GenerateToken(admin.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "生成 Token 失败"})
		return
	}

	// 标记登录成功，供 LoginRateLimit 中间件移除限速计数
	c.Set("login_success", true)

	// 设置 Cookie（HttpOnly + SameSite=Strict）
	// secure=false 以兼容 HTTP 开发环境和反向代理部署（浏览器到反代间使用 HTTPS 保护传输）
	// maxAge 从 JWTManager.Expiry() 派生，避免与 JWT 过期时间失配
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie("token", token, int(h.jwtManager.Expiry()/time.Second), "/", "", cookieSecure(), true)

	c.JSON(http.StatusOK, LoginResponse{
		Success: true,
		Message: "登录成功",
	})
}

// HandleLogout 处理登出
// 路由: POST /api/v1/auth/logout
// 防止 Logout CSRF：仅当请求携带了有效 Token Cookie 时才清除 Cookie
func (h *AuthHandler) HandleLogout(c *gin.Context) {
	tokenString, err := c.Cookie("token")
	if err != nil || tokenString == "" {
		// 无 Cookie（可能是跨站攻击），返回成功但不清除 Cookie
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "已登出"})
		return
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

	// 生成 JWT,自动登录
	token, err := h.jwtManager.GenerateToken(admin.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "生成 Token 失败"})
		return
	}

	// 设置 Cookie（HttpOnly + SameSite=Strict）
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie("token", token, int(h.jwtManager.Expiry()/time.Second), "/", "", cookieSecure(), true)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "管理员账户创建成功",
	})
}

// HandleCheckAuth 检查当前登录状态
// 路由: GET /api/v1/auth/me
// 供前端在页面加载时检查 Cookie 中的 Token 是否仍然有效
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

	c.JSON(http.StatusOK, gin.H{
		"authenticated": true,
		"admin_id":      claims.AdminID,
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
