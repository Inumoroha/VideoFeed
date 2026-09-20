package relation

import (
	"context"
	"errors"
	"github.com/gin-gonic/gin"
	app "github.com/inumoroha/video-feed/internal/application/relation"
	"github.com/inumoroha/video-feed/internal/middleware"
	"net/http"
	"net/http/httptest"
	"testing"
)

// failedBackfill 模拟粉丝数查询失败，后续方法不得执行。
type failedBackfill struct{ app.FollowFeedBackfiller }

// CountFollowers 返回模拟的回填错误。
func (failedBackfill) CountFollowers(context.Context, int64) (int, error) {
	return 0, errors.New("failed")
}

// TestFollowBackfillHTTPFailure 验证回填失败使用统一 500 响应，取关仍成功。
func TestFollowBackfillHTTPFailure(t *testing.T) {
	h := NewHandler(app.NewService(relationHandlerRepository{}, app.WithFollowFeedBackfiller(failedBackfill{})))
	for _, active := range []bool{true, false} {
		w := httptest.NewRecorder()
		ctx := relationTestContext(http.MethodPut, "/api/relations/3", w)
		ctx.Params = gin.Params{{Key: "targetUserId", Value: "3"}}
		ctx.Set(middleware.ContextUserIDKey, int64(7))
		if active {
			h.Follow(ctx)
			if w.Code != 500 || w.Body.String() != `{"error":"internal server error"}` {
				t.Fatal(w.Code, w.Body.String())
			}
		} else {
			h.Unfollow(ctx)
			if w.Code != 200 {
				t.Fatal(w.Code)
			}
		}
	}
}
