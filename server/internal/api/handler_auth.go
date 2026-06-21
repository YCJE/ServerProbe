package api

import (
	"log"
	"net/http"
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
type LoginResponse struct {
	Success   bool   `json:"success"`
	Message   string `json:"message"`
	NeedTOTP  bool   `json:"need_totp"`
	Token     string `json:"token,omitempty"`
}

// HandleLogin 处理登录
// 路由: POST /api/v1/auth/login
func (h *AuthHandler) HandleLogin(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效"})
		return
	}

	admin, err := h.adminRepo.GetByUsername(req.Username)
	if err != nil {
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

	// 设置 Cookie（HttpOnly + Secure + SameSite=Strict）
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie("token", token, int(12*time.Hour/time.Second), "/", "", true, true)

	c.JSON(http.StatusOK, LoginResponse{
		Success: true,
		Message: "登录成功",
		Token:   token,
	})
}

// HandleLogout 处理登出
// 路由: POST /api/v1/auth/logout
func (h *AuthHandler) HandleLogout(c *gin.Context) {
	c.SetCookie("token", "", -1, "/", "", true, true)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "已登出"})
}

// HandleSetup 处理首次设置（创建管理员账户）
// 路由: POST /api/v1/auth/setup
func (h *AuthHandler) HandleSetup(c *gin.Context) {
	// 检查是否已有管理员
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

	// 再次检查（防止 TOCTOU 竞争条件）
	count, err = h.adminRepo.Count()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "检查管理员失败"})
		return
	}
	if count > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "管理员账户已存在"})
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

	// 创建管理员
	admin := &model.Admin{
		Username:     req.Username,
		PasswordHash: hash,
	}

	if err := h.adminRepo.Create(admin); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建管理员失败"})
		return
	}

	// 生成 JWT,自动登录
	token, err := h.jwtManager.GenerateToken(admin.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "生成 Token 失败"})
		return
	}

	// 设置 Cookie
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie("token", token, int(12*time.Hour/time.Second), "/", "", true, true)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "管理员账户创建成功",
		"token":   token,
	})
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
