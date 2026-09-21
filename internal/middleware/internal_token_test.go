package middleware

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestInternalTokenAuth 验证缺失配置、错误令牌均拒绝，去除首尾空白后匹配才执行后续处理器。
func TestInternalTokenAuth(t *testing.T) {
	for _, tc := range []struct {
		configured, provided string
		status               int
	}{{"", "", 401}, {"", "token", 401}, {"token", "", 401}, {"token", "wrong", 401}, {" token ", " token ", 204}} {
		g := gin.New()
		g.GET("/", NewInternalTokenAuth(tc.configured), func(c *gin.Context) { c.Status(http.StatusNoContent) })
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("X-Internal-Token", tc.provided)
		resp := httptest.NewRecorder()
		g.ServeHTTP(resp, req)
		if resp.Code != tc.status {
			t.Fatalf("status=%d want=%d", resp.Code, tc.status)
		}
	}
}
