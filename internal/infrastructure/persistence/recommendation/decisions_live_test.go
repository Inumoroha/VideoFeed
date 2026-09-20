package recommendation

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	app "github.com/inumoroha/video-feed/internal/application/recommendation"
	domain "github.com/inumoroha/video-feed/internal/domain/recommendation"
	handler "github.com/inumoroha/video-feed/internal/handler/recommendation"
	exposure "github.com/inumoroha/video-feed/internal/infrastructure/persistence/exposure"
	"gorm.io/gorm"
)

// verifyExposureDecisionsLive 在调用者的隔离数据库中验证真实时间边界、用户隔离及并发 HTTP 决策不修改曝光。
func verifyExposureDecisionsLive(t *testing.T, db *gorm.DB) {
	t.Helper()
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	since := now.Add(-domain.RecentExposureWindow)
	rows := []exposure.ExposureModel{
		{UserID: 9001, VideoID: 1, FirstExposedAt: since, LastExposedAt: since, ExposureCount: 7, LastScene: "timeline"},
		{UserID: 9001, VideoID: 2, FirstExposedAt: since.Add(-time.Hour), LastExposedAt: since.Add(-time.Microsecond), ExposureCount: 2, LastScene: "recommend"},
		{UserID: 9002, VideoID: 3, FirstExposedAt: now, LastExposedAt: now, ExposureCount: 3, LastScene: "recommend"},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/internal/exposure-decisions", handler.New(app.New(New(db), app.WithNow(func() time.Time { return now }))).DecideExposures)
	body := []byte(`{"user_id":9001,"scene":"recommend","request_id":"same","video_ids":[1,2,3,999,1]}`)
	request := httptest.NewRequest(http.MethodPost, "/internal/exposure-decisions", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	// result 保存本次真实 HTTP 响应的决策字段。
	var result struct {
		Decisions []struct {
			VideoID       int64      `json:"video_id"`
			Allowed       bool       `json:"allowed"`
			Reason        string     `json:"reason"`
			LastExposedAt *time.Time `json:"last_exposed_at"`
		} `json:"decisions"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Decisions) != 4 {
		t.Fatal(response.Body.String())
	}
	first := result.Decisions[0]
	if first.VideoID != 1 || first.Allowed || first.Reason != "recently_exposed" || first.LastExposedAt == nil || !first.LastExposedAt.Equal(since) {
		t.Fatal(first)
	}
	for i, id := range []int64{2, 3, 999} {
		item := result.Decisions[i+1]
		if item.VideoID != id || !item.Allowed || item.Reason != "fresh" || item.LastExposedAt != nil {
			t.Fatal(item)
		}
	}
	expected := response.Body.String()
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodPost, "/internal/exposure-decisions", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			res := httptest.NewRecorder()
			router.ServeHTTP(res, req)
			if res.Code != 200 || res.Body.String() != expected {
				t.Errorf("并发查询错误: %d %s", res.Code, res.Body.String())
			}
		}()
	}
	wg.Wait()
	var saved []exposure.ExposureModel
	if err := db.Where("user_id IN ?", []int64{9001, 9002}).Order("id").Find(&saved).Error; err != nil {
		t.Fatal(err)
	}
	if len(saved) != len(rows) {
		t.Fatal(saved)
	}
	for i, item := range saved {
		if item.ExposureCount != rows[i].ExposureCount || !item.LastExposedAt.Equal(rows[i].LastExposedAt) {
			t.Fatal("只读决策改变曝光", item)
		}
	}
	var count int64
	if err := db.Model(&exposure.ViewEventModel{}).Where("user_id IN ?", []int64{9001, 9002}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("只读决策产生流水", count, err)
	}
}
