package api

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/server-probe/server/internal/model"
	"github.com/server-probe/server/internal/pkg"
	"github.com/server-probe/server/internal/repository"
	"github.com/server-probe/server/internal/service"
)

// Middleware 中间件管理
type Middleware struct {
	jwtManager *pkg.JWTManager
	sessionRepo *repository.SessionRepository
}

// NewMiddleware 创建中间件
func NewMiddleware(jwtManager *pkg.JWTManager, sessionRepo *repository.SessionRepository) *Middleware {
	return &Middleware{jwtManager: jwtManager, sessionRepo: sessionRepo}
}

// validateSession 校验 JWT 对应的会话是否有效（存在、未撤销、未过期）
// 返回 nil 表示有效；sessionRepo 为 nil 时（测试环境）跳过会话校验
func (m *Middleware) validateSession(claims *pkg.Claims) *model.Session {
	if m.sessionRepo == nil {
		return nil
	}
	if claims.ID == "" {
		return nil
	}
	s, err := m.sessionRepo.GetBySessionID(claims.ID)
	if err != nil {
		return nil
	}
	if s.RevokedAt != nil || time.Now().After(s.ExpiresAt) {
		return nil
	}
	return s
}

// AuthRequired JWT 认证中间件
func (m *Middleware) AuthRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 从 Cookie 中获取 Token（HttpOnly Cookie，前端 JS 无法读取）
		tokenString, err := c.Cookie("token")
		if err != nil {
			// 尝试从 Authorization header 获取（兼容 API 客户端）
			auth := c.GetHeader("Authorization")
			if strings.HasPrefix(auth, "Bearer ") {
				tokenString = strings.TrimPrefix(auth, "Bearer ")
			}
		}

		if tokenString == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
			c.Abort()
			return
		}

		claims, err := m.jwtManager.ValidateToken(tokenString)
		if err != nil {
			// Token 过期或无效，清除 Cookie 防止浏览器持续发送过期凭证
			c.SetSameSite(http.SameSiteStrictMode)
			c.SetCookie("token", "", -1, "/", "", cookieSecure(), true)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Token 无效或已过期"})
			c.Abort()
			return
		}

		// 会话校验（P2）：登出/远程撤销后即使 Token 未过期也立即失效
		session := m.validateSession(claims)
		if session == nil {
			if m.sessionRepo != nil {
				c.SetSameSite(http.SameSiteStrictMode)
				c.SetCookie("token", "", -1, "/", "", cookieSecure(), true)
				c.JSON(http.StatusUnauthorized, gin.H{"error": "会话已失效"})
				c.Abort()
				return
			}
		} else {
			// 节流更新最后活跃时间（距上次超过 60s 才写库，避免高频请求全量写）
			if time.Since(session.LastSeenAt) > time.Minute {
				_ = m.sessionRepo.Touch(session)
			}
			c.Set("session_id", session.SessionID)
		}

		c.Set("admin_id", claims.AdminID)
		c.Next()
	}
}

// LoginRateLimit 登录限速中间件
// 每个 IP 每分钟最多 5 次尝试
type loginRateLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
	lastClean time.Time
}

var rateLimiter = &loginRateLimiter{
	attempts:  make(map[string][]time.Time),
	lastClean: time.Now(),
}

// LoginRateLimit 登录限速
func (m *Middleware) LoginRateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()

		now := time.Now()
		cutoff := now.Add(-time.Minute)

		rateLimiter.mu.Lock()

		// 紧急清理：map 过大时立即全量清理（与 pubRateLimiter 一致，防止旋转 IP 耗尽内存）
		if len(rateLimiter.attempts) > 10000 {
			for ip, attempts := range rateLimiter.attempts {
				valid := make([]time.Time, 0, len(attempts))
				for _, t := range attempts {
					if t.After(cutoff) {
						valid = append(valid, t)
					}
				}
				if len(valid) == 0 {
					delete(rateLimiter.attempts, ip)
				} else {
					rateLimiter.attempts[ip] = valid
				}
			}
			rateLimiter.lastClean = now
		} else if now.Sub(rateLimiter.lastClean) > 5*time.Minute {
			for ip, attempts := range rateLimiter.attempts {
				valid := make([]time.Time, 0, len(attempts))
				for _, t := range attempts {
					if t.After(cutoff) {
						valid = append(valid, t)
					}
				}
				if len(valid) == 0 {
					delete(rateLimiter.attempts, ip)
				} else {
					rateLimiter.attempts[ip] = valid
				}
			}
			rateLimiter.lastClean = now
		}

		// 清理当前 IP 的过期记录
		attempts := rateLimiter.attempts[ip]
		valid := make([]time.Time, 0, len(attempts))
		for _, t := range attempts {
			if t.After(cutoff) {
				valid = append(valid, t)
			}
		}

		if len(valid) >= 5 {
			rateLimiter.attempts[ip] = valid
			rateLimiter.mu.Unlock()
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "登录尝试过于频繁，请稍后再试"})
			c.Abort()
			return
		}

		// 暂时记录本次尝试，待 handler 返回后根据状态码决定是否保留
		valid = append(valid, now)
		rateLimiter.attempts[ip] = valid
		rateLimiter.mu.Unlock()

		// 记录本次尝试时间戳，供登录成功后精确移除对应计数
		// 避免并发请求下"移除最后一个元素"误删其他请求的计数
		c.Set("login_attempt_time", now)

		c.Next()

		// 仅当 handler 显式标记登录成功时才移除计数
		// （NeedTOTP 返回 200 但并非真正登录成功，不应移除计数，防止 TOTP 暴力破解）
		if c.GetBool("login_success") {
			if attemptTime, ok := c.Get("login_attempt_time"); ok {
				ts := attemptTime.(time.Time)
				rateLimiter.mu.Lock()
				if cur, ok := rateLimiter.attempts[ip]; ok {
					for i, t := range cur {
						if t.Equal(ts) {
							rateLimiter.attempts[ip] = append(cur[:i], cur[i+1:]...)
							break
						}
					}
				}
				rateLimiter.mu.Unlock()
			}
		}
	}
}

// CORS 跨域中间件 (前端内嵌，同源访问，不需要跨域)
func (m *Middleware) CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 前端内嵌在 Server 中，同源访问，不需要 CORS
		// 仅处理 OPTIONS 预检请求
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// publicRateLimiter 公开 API 限速器（基于 IP 的滑动窗口）
type publicRateLimiter struct {
	mu       sync.Mutex
	requests map[string][]time.Time
	lastClean time.Time
}

var pubRateLimiter = &publicRateLimiter{
	requests:  make(map[string][]time.Time),
	lastClean: time.Now(),
}

// PublicRateLimit 公开 API 限速中间件
// 每个 IP 每分钟最多 60 次请求，防止未认证 DoS 耗尽数据库连接
func (m *Middleware) PublicRateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		now := time.Now()
		cutoff := now.Add(-time.Minute)

		pubRateLimiter.mu.Lock()

		// 紧急清理：map 过大时立即全量清理（防止旋转 IP 耗尽内存）
		if len(pubRateLimiter.requests) > 10000 {
			for i, reqs := range pubRateLimiter.requests {
				valid := make([]time.Time, 0, len(reqs))
				for _, t := range reqs {
					if t.After(cutoff) {
						valid = append(valid, t)
					}
				}
				if len(valid) == 0 {
					delete(pubRateLimiter.requests, i)
				} else {
					pubRateLimiter.requests[i] = valid
				}
			}
			pubRateLimiter.lastClean = now
		} else if now.Sub(pubRateLimiter.lastClean) > 5*time.Minute {
			for ip, reqs := range pubRateLimiter.requests {
				valid := make([]time.Time, 0, len(reqs))
				for _, t := range reqs {
					if t.After(cutoff) {
						valid = append(valid, t)
					}
				}
				if len(valid) == 0 {
					delete(pubRateLimiter.requests, ip)
				} else {
					pubRateLimiter.requests[ip] = valid
				}
			}
			pubRateLimiter.lastClean = now
		}

		// 清理当前 IP 的过期记录
		reqs := pubRateLimiter.requests[ip]
		valid := make([]time.Time, 0, len(reqs))
		for _, t := range reqs {
			if t.After(cutoff) {
				valid = append(valid, t)
			}
		}

		if len(valid) >= 60 {
			pubRateLimiter.requests[ip] = valid
			pubRateLimiter.mu.Unlock()
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "请求过于频繁，请稍后再试"})
			c.Abort()
			return
		}

		valid = append(valid, now)
		pubRateLimiter.requests[ip] = valid
		pubRateLimiter.mu.Unlock()

		c.Next()
	}
}

// testActionLimiter 测试操作限流器（触发外部请求/通知的操作全局共享）
// 防止管理员或被劫持的会话连续触发测试，造成通知轰炸或对外部目标的高频探测
type testActionLimiter struct {
	mu       sync.Mutex
	count    int
	windowAt time.Time
}

var testLimiter = &testActionLimiter{}

// TestActionRateLimit 测试操作限速中间件（全局每分钟最多 10 次）
// 应用于告警测试/通知渠道测试/服务监控测试/SSL 证书测试等会触发外部请求的接口
func (m *Middleware) TestActionRateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		testLimiter.mu.Lock()
		now := time.Now()
		if now.Sub(testLimiter.windowAt) >= time.Minute {
			testLimiter.windowAt = now
			testLimiter.count = 0
		}
		if testLimiter.count >= 10 {
			testLimiter.mu.Unlock()
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "测试操作过于频繁，请稍后再试"})
			c.Abort()
			return
		}
		testLimiter.count++
		testLimiter.mu.Unlock()
		c.Next()
	}
}

// SecurityHeaders 安全响应头中间件
func (m *Middleware) SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-XSS-Protection", "1; mode=block")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; connect-src 'self'; img-src 'self' data:; font-src 'self' data:;")
		// HSTS: 仅 HTTPS 请求设置（开发环境 HTTP 不设置，避免浏览器锁定）
		if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
			c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		c.Next()
	}
}

// AuditMutations 管理端变更操作审计中间件（P1：安全闭环）
// 记录 protected 组内所有 POST/PUT/DELETE 请求，以及被 RequireSensitive2FA
// 标记的敏感 GET 请求（如 Token 查看）。仅记录方法/路由/目标路径/状态码，
// 绝不记录请求体（可能含密码、TOTP 密钥等敏感字段）
func (m *Middleware) AuditMutations(auditSvc *service.AuditService, adminRepo *repository.AdminRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		mutating := c.Request.Method == http.MethodPost ||
			c.Request.Method == http.MethodPut ||
			c.Request.Method == http.MethodDelete
		sensitive := c.GetBool("sensitive_2fa")
		if !mutating && !sensitive {
			return
		}

		adminID := c.GetInt64("admin_id")
		username := ""
		if adminID > 0 && adminRepo != nil {
			if admin, err := adminRepo.GetByID(adminID); err == nil {
				username = admin.Username
			}
		}

		action := c.Request.Method + " " + c.FullPath()
		if c.FullPath() == "" {
			action = c.Request.Method + " " + c.Request.URL.Path
		}

		auditSvc.Record(model.AuditLog{
			AdminID:   adminID,
			Username:  username,
			Action:    action,
			Target:    c.Request.URL.Path,
			Success:   c.Writer.Status() < 400,
			IP:        c.ClientIP(),
			UserAgent: c.Request.UserAgent(),
		})
	}
}

// RequireSensitive2FA 敏感操作两步验证再确认中间件（P1，借鉴 Komari RequireSensitive2FA）
// 会话 Cookie 被劫持（XSS/物理接触）后，攻击者仍无法执行破坏性操作：
// 已启用 TOTP 的账户必须随请求携带有效动态码（X-TOTP-Code 头）。
// 未启用 TOTP 的账户无从校验，直接放行（审计中间件仍会记录该操作）
func (m *Middleware) RequireSensitive2FA(adminRepo *repository.AdminRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 标记敏感操作，供 AuditMutations 记录（GET 类敏感请求也需要审计）
		c.Set("sensitive_2fa", true)

		admin, err := adminRepo.GetByID(c.GetInt64("admin_id"))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "账户不存在"})
			c.Abort()
			return
		}

		if !admin.TOTPEnabled {
			c.Next()
			return
		}

		code := c.GetHeader("X-TOTP-Code")
		if code == "" || !consumeTOTPStep(admin.ID, admin.TOTPSecret, code) {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "敏感操作需要两步验证码确认",
				"code":  "totp_required",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}
