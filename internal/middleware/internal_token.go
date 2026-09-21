package middleware

import (
	"crypto/subtle"
	"github.com/gin-gonic/gin"
	"net/http"
	"strings"
)

// NewInternalTokenAuth 校验内部服务调用使用的 X-Internal-Token。
func NewInternalTokenAuth(token string) gin.HandlerFunc {
	token = strings.TrimSpace(token)
	return func(c *gin.Context) {
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "internal token is required",
			})
			return
		}
		provided := strings.TrimSpace(c.GetHeader("X-Internal-Token"))
		if subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "invalid internal token",
			})
			return
		}
		c.Next()
	}
}
