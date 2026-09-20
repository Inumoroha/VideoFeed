package feed

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	app "github.com/inumoroha/video-feed/internal/application/recommendation"
	domain "github.com/inumoroha/video-feed/internal/domain/recommendation"
	handler "github.com/inumoroha/video-feed/internal/handler/recommendation"
)

// failedDecisionRepository 模拟曝光查询失败，其他仓储能力不应被调用。
type failedDecisionRepository struct{ domain.Repository }

// ListRecentExposures 返回模拟数据库错误以验证内部错误不会泄露。
func (failedDecisionRepository) ListRecentExposures(context.Context, int64, []int64, time.Time) ([]*domain.Exposure, error) {
	return nil, errors.New("private database error")
}

// decisionTestRouter 使用指定仓储和固定时间注册决策接口，不添加认证中间件。
func decisionTestRouter(repo domain.Repository, now time.Time) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := handler.New(app.New(repo, app.WithNow(func() time.Time { return now })))
	r.POST("/internal/exposure-decisions", h.DecideExposures)
	return r
}

// TestExposureDecisionHTTPValidation 验证空列表序列化、元数据校验、字段类型及统一错误响应。
func TestExposureDecisionHTTPValidation(t *testing.T) {
	router := newRecommendationRouter()
	for _, body := range []string{`{"user_id":42,"scene":"recommend"}`, `{"user_id":42,"scene":"recommend","video_ids":null}`, `{"user_id":42,"scene":"recommend","video_ids":[]}`} {
		response := performJSONRequest(router, http.MethodPost, "/internal/exposure-decisions", body, "")
		requireStatus(t, response, 200)
		var fields map[string]json.RawMessage
		decodeJSON(t, response, &fields)
		if string(fields["decisions"]) != "[]" || fields["request_id"] != nil {
			t.Fatal(response.Body.String())
		}
	}
	for _, body := range []string{`{`, `{}`, `{"user_id":42}`, `{"user_id":0,"scene":"recommend"}`, `{"user_id":42,"scene":"recommend","video_ids":[-1]}`, `{"user_id":42,"scene":"recommend","video_ids":[1.5]}`, `{"user_id":"42","scene":"recommend"}`, `{"user_id":42,"scene":"` + strings.Repeat("a", 33) + `"}`, `{"user_id":42,"scene":"r","request_id":"` + strings.Repeat("a", 65) + `"}`} {
		response := performJSONRequest(router, http.MethodPost, "/internal/exposure-decisions", body, "")
		requireStatus(t, response, 400)
		var fields map[string]string
		decodeJSON(t, response, &fields)
		if fields["error"] == "" {
			t.Fatal(response.Body.String())
		}
	}
	response := performJSONRequest(decisionTestRouter(failedDecisionRepository{}, time.Now()), http.MethodPost, "/internal/exposure-decisions", `{"user_id":42,"scene":"r","video_ids":[1]}`, "")
	requireStatus(t, response, 500)
	if response.Body.String() != `{"error":"internal server error"}` {
		t.Fatal(response.Body.String())
	}
}

// TestExposureDecisionBoundaryIsolationAndConcurrency 验证七天含边界、跨场景去重、用户隔离、未知视频放行及并发重复查询不修改曝光。
func TestExposureDecisionBoundaryIsolationAndConcurrency(t *testing.T) {
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	since := now.Add(-domain.RecentExposureWindow)
	repo := newMemoryRecommendationRepo()
	repo.exposures[recommendationExposureKey(42, 1)] = domain.RestoreExposure(1, 42, 1, since, since, 7, "timeline")
	repo.exposures[recommendationExposureKey(42, 2)] = domain.RestoreExposure(2, 42, 2, since, since.Add(-time.Nanosecond), 2, "recommend")
	repo.exposures[recommendationExposureKey(43, 3)] = domain.RestoreExposure(3, 43, 3, now, now, 3, "recommend")
	before := make(map[string]*domain.Exposure)
	for key, item := range repo.exposures {
		before[key] = cloneRecommendationExposure(item)
	}
	router := decisionTestRouter(repo, now)
	body := `{"user_id":42,"scene":" ReCommend ","request_id":" shared ","video_ids":[1,2,3,999,1]}`
	response := performJSONRequest(router, http.MethodPost, "/internal/exposure-decisions", body, "")
	requireStatus(t, response, 200)
	var result recommendationExposureDecisionAPIResponse
	decodeJSON(t, response, &result)
	if result.Scene != "recommend" || result.RequestID != "shared" || len(result.Decisions) != 4 || result.Decisions[0].Allowed || result.Decisions[0].LastExposedAt == nil || !result.Decisions[0].LastExposedAt.Equal(since) {
		t.Fatal(result)
	}
	for _, item := range result.Decisions[1:] {
		if !item.Allowed || item.LastExposedAt != nil || item.Reason != "fresh" {
			t.Fatal(item)
		}
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	var decisions []map[string]json.RawMessage
	if err := json.Unmarshal(raw["decisions"], &decisions); err != nil {
		t.Fatal(err)
	}
	if decisions[1]["last_exposed_at"] != nil {
		t.Fatal("未曝光时间应省略")
	}
	want := response.Body.String()
	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got := performJSONRequest(router, http.MethodPost, "/internal/exposure-decisions", body, "")
			if got.Code != 200 || got.Body.String() != want {
				t.Errorf("并发决策不一致: %d %s", got.Code, got.Body.String())
			}
		}()
	}
	wg.Wait()
	if !reflect.DeepEqual(before, repo.exposures) {
		t.Fatal("只读决策修改了曝光状态")
	}
}
