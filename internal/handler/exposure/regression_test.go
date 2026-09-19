package exposure_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	applicationexposure "github.com/inumoroha/video-feed/internal/application/exposure"
	domainexposure "github.com/inumoroha/video-feed/internal/domain/exposure"
)

// failingPublisher 模拟持久化成功后的消息服务故障，并记录发布次数。
type failingPublisher struct{ calls int }

// PublishViewEventRecorded 记录发送尝试并返回消息服务故障。
func (p *failingPublisher) PublishViewEventRecorded(context.Context, *applicationexposure.ViewEventRecordedEvent) error {
	p.calls++
	return errors.New("broker down")
}

// failingRepository 模拟无法保存观看流水的仓储。
type failingRepository struct{}

// SaveViewEvent 返回数据库错误，供测试核对错误隐藏和禁止发布行为。
func (failingRepository) SaveViewEvent(context.Context, *domainexposure.ViewEvent) (*domainexposure.ViewEvent, *domainexposure.Exposure, error) {
	return nil, nil, errors.New("private database failure")
}

// TestExposureFailureFallback 验证消息失败仍返回已保存流水、未配置发布器可用，数据库失败不发布且隐藏原始错误。
func TestExposureFailureFallback(t *testing.T) {
	publisher := &failingPublisher{}
	router, manager, repo := newExposureRouterWithPublisher(t, publisher)
	response := performJSONRequest(router, http.MethodPost, "/api/video-view-events", `{"video_id":1001,"scene":"timeline","event_type":"exposed"}`, signTestToken(t, manager, 42))
	requireStatus(t, response, http.StatusCreated)
	if publisher.calls != 1 || repo.EventCount() != 1 {
		t.Fatal("消息失败改变了落库结果或触发额外重试")
	}
	service := applicationexposure.NewService(failingRepository{}, applicationexposure.WithViewEventPublisher(publisher))
	if _, err := service.RecordViewEvent(context.Background(), 42, 1001, "hot", "", "play", 0, false); !errors.Is(err, applicationexposure.ErrSaveExposureFailed) {
		t.Fatal(err)
	}
	if publisher.calls != 1 {
		t.Fatal("保存失败仍发送了消息")
	}
}

// TestConcurrentRepeatedRequestPreservesCounting 验证相同请求编号并发上报仍逐次保存和计数，互不丢失且均发布事件。
func TestConcurrentRepeatedRequestPreservesCounting(t *testing.T) {
	repo := newMemoryExposureRepo()
	publisher := &memoryViewEventPublisher{}
	service := applicationexposure.NewService(repo, applicationexposure.WithViewEventPublisher(publisher))
	var wg sync.WaitGroup
	results := make(chan *applicationexposure.RecordViewEventResult, 32)
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := service.RecordViewEvent(context.Background(), 42, 1001, "hot", "same-request", "exposed", 0, false)
			if err != nil {
				t.Error(err)
				return
			}
			results <- result
		}()
	}
	wg.Wait()
	close(results)
	counts := map[int]bool{}
	ids := map[int64]bool{}
	var first time.Time
	for result := range results {
		counts[result.Exposure.ExposureCount] = true
		ids[result.Event.ID] = true
		if first.IsZero() {
			first = result.Exposure.FirstExposedAt
		}
		if !first.Equal(result.Exposure.FirstExposedAt) {
			t.Error("首次曝光时间变化")
		}
	}
	if len(counts) != 32 || len(ids) != 32 || repo.EventCount() != 32 || len(publisher.Events()) != 32 {
		t.Fatal("并发事件丢失或被错误去重")
	}
}

// TestExposureAdditionalHTTPValidation 覆盖格式错误、字段缺省、长度越界及完整事件响应的可选字段。
func TestExposureAdditionalHTTPValidation(t *testing.T) {
	router, manager, repo := newExposureRouter(t)
	token := signTestToken(t, manager, 42)
	for _, body := range []string{`{`, `null`, `{}`, `{"video_id":"1001"}`, `{"video_id":0,"scene":"hot","event_type":"play"}`, `{"video_id":1001,"scene":"hot","event_type":"play","request_id":"` + strings.Repeat("r", 65) + `"}`, `{"video_id":1001,"scene":"` + strings.Repeat("a", 33) + `","event_type":"play"}`} {
		requireStatus(t, performJSONRequest(router, http.MethodPost, "/api/video-view-events", body, token), http.StatusBadRequest)
	}
	requireStatus(t, performJSONRequest(router, http.MethodPost, "/api/video-view-events", `{}`, "bad-token"), http.StatusUnauthorized)
	if repo.EventCount() != 0 {
		t.Fatal("无效请求写入流水")
	}
	response := performJSONRequest(router, http.MethodPost, "/api/video-view-events", `{"video_id":1001,"scene":" HOT ","event_type":" SKIP ","completed":true}`, token)
	requireStatus(t, response, http.StatusCreated)
	var payload map[string]any
	decodeJSON(t, response, &payload)
	if _, exists := payload["exposure"]; exists {
		t.Fatal("非曝光响应包含聚合")
	}
	event := payload["event"].(map[string]any)
	if _, exists := event["request_id"]; exists {
		t.Fatal("空请求编号未省略")
	}
	if event["scene"] != "hot" || event["event_type"] != "skip" || event["completed"] != true {
		t.Fatal(event)
	}
}
