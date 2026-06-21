package api

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/server-probe/server/internal/pkg"
)

// Middleware 中间件管理
type Middleware struct {
	jwtManager *pkg.JWTManager
}

// NewMiddleware 创建中间件
func NewMiddleware(jwtManager *pkg.JWTManager) *Middleware {
	return &Middleware{jwtManager: jwtManager}
}

// AuthRequired JWT 认证中间件
func (m *Middleware) AuthRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 从 Cookie 中获取 Token
		tokenString, err := c.Cookie("token")
		if err != nil {
			// 尝试从 Authorization header 获取
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
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Token 无效或已过期"})
			c.Abort()
			return
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

		rateLimiter.mu.Lock()
		defer rateLimiter.mu.Unlock()

		now := time.Now()
		cutoff := now.Add(-time.Minute)

		// 每 5 分钟清理一次不活跃 IP 的记录，防止内存泄漏
		if now.Sub(rateLimiter.lastClean) > 5*time.Minute {
			for ip, attempts := range rateLimiter.attempts {
				valid := attempts[:0]
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
		valid := attempts[:0]
		for _, t := range attempts {
			if t.After(cutoff) {
				valid = append(valid, t)
			}
		}

		if len(valid) >= 5 {
			rateLimiter.attempts[ip] = valid
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "登录尝试过于频繁，请稍后再试"})
			c.Abort()
			return
		}

		valid = append(valid, now)
		rateLimiter.attempts[ip] = valid

		c.Next()
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

// SecurityHeaders 安全响应头中间件
func (m *Middleware) SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-XSS-Protection", "1; mode=block")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Next()
	}
}
