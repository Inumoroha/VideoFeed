package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	authjwt "github.com/inumoroha/video-feed/internal/auth/jwt"
)

const ContextUserIDKey = "auth_user_id"
const ContextUserRoleKey = "auth_user_role"

// NewOptionalJWTAuthMiddleware 创建可选认证中间件，无令牌时允许访客请求继续执行。
func NewOptionalJWTAuthMiddleware(jwtManager *authjwt.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.TrimSpace(c.GetHeader("Authorization")) == "" {
			c.Next()
			return
		}
		NewJWTAuthMiddleware(jwtManager)(c)
	}
}

// NewJWTAuthMiddleware 创建校验访问令牌并将用户 ID 写入 Gin 上下文的认证中间件。
func NewJWTAuthMiddleware(jwtManager *authjwt.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		if jwtManager == nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error": "authentication service unavailable",
			})
			return
		}

		header := strings.TrimSpace(c.GetHeader("Authorization"))
		if header == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "authorization header is required",
			})
			return
		}

		parts := strings.SplitN(header, " ", 2)
		if len(parts) != 2 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "authorization format must be Bearer <token>",
			})
			return
		}
		if !strings.EqualFold(strings.TrimSpace(parts[0]), "Bearer") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "authorization scheme must be Bearer",
			})
			return
		}

		token := strings.TrimSpace(parts[1])
		claims, err := jwtManager.ParseAndValidateToken(token, authjwt.TokenTypeAccess)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "invalid access token",
			})
			return
		}

		c.Set(ContextUserIDKey, claims.UserID)
		c.Set(ContextUserRoleKey, claims.Role)
		c.Next()
	}
}

// NewOptionalJWTAuth 在公共接口中补充可选登录身份，适合 Feed 个性化场景。
func NewOptionalJWTAuth(jwtManager *authjwt.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		if jwtManager == nil {
			c.Next()
			return
		}
		header := strings.TrimSpace(c.GetHeader("Authorization"))
		if header == "" {
			c.Next()
			return
		}

		parts := strings.SplitN(header, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(strings.TrimSpace(parts[0]), "Bearer") {
			c.Next()
			return
		}

		token := strings.TrimSpace(parts[1])
		claims, err := jwtManager.ParseAndValidateToken(token, authjwt.TokenTypeAccess)
		if err == nil {
			c.Set(ContextUserIDKey, claims.UserID)
			c.Set(ContextUserRoleKey, claims.Role)
		}
		c.Next()
	}
}
