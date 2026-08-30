package feed

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"testing"
	"time"

	domainfeed "github.com/inumoroha/video-feed/internal/domain/feed"
)

type serviceTestRepository struct {
	items     []*domainfeed.FeedItem
	listLimit int
	findEvent *domainfeed.ViewEvent
	findErr   error
	saveEvent *domainfeed.ViewEvent
	saveErr   error
	findCalls int
}

// ListTimeFeed 返回测试信息流并记录服务请求的查询数量。
func (r *serviceTestRepository) ListTimeFeed(_ context.Context, _ *domainfeed.TimeCursor, limit int) ([]*domainfeed.FeedItem, error) {
	r.listLimit = limit
	return r.items, nil
}

// SaveViewEvent 保存测试观看事件并记录传入的领域对象。
func (r *serviceTestRepository) SaveViewEvent(_ context.Context, event *domainfeed.ViewEvent) error {
	r.saveEvent = event
	return r.saveErr
}

// FindViewEventByIdempotencyKey 返回预设的幂等事件结果。
func (r *serviceTestRepository) FindViewEventByIdempotencyKey(_ context.Context, _ string) (*domainfeed.ViewEvent, error) {
	r.findCalls++
	return r.findEvent, r.findErr
}

// TestServiceRejectsUninitializedService 验证未初始化服务不会触发空指针异常。
func TestServiceRejectsUninitializedService(t *testing.T) {
	var service *Service
	if _, err := service.GetTimeFeed(context.TODO(), "", 20); !errors.Is(err, ErrFeedServiceNotInitialized) {
		t.Fatalf("GetTimeFeed() error = %v, want %v", err, ErrFeedServiceNotInitialized)
	}
	if _, err := service.ReportViewEvent(context.TODO(), nil, "visitor-1", 7, domainfeed.EventTypeView, 0, ""); !errors.Is(err, ErrFeedServiceNotInitialized) {
		t.Fatalf("ReportViewEvent() error = %v, want %v", err, ErrFeedServiceNotInitialized)
	}
}

// TestGetTimeFeedUsesLookaheadAndBuildsCursor 验证服务会多查一条数据并生成下一页游标。
func TestGetTimeFeedUsesLookaheadAndBuildsCursor(t *testing.T) {
	repository := &serviceTestRepository{items: makeFeedItems(domainfeed.MaxLimit + 1)}
	service := NewService(repository)

	result, err := service.GetTimeFeed(context.TODO(), "", domainfeed.MaxLimit)
	if err != nil {
		t.Fatalf("GetTimeFeed() error = %v", err)
	}
	if repository.listLimit != domainfeed.MaxLimit+1 {
		t.Fatalf("ListTimeFeed() limit = %d, want %d", repository.listLimit, domainfeed.MaxLimit+1)
	}
	if len(result.Items) != domainfeed.MaxLimit || !result.HasMore || result.NextCursor == "" {
		t.Fatalf("GetTimeFeed() result = %+v, want capped items with next cursor", result)
	}
	if _, err := parseTimeCursor(result.NextCursor); err != nil {
		t.Fatalf("next cursor cannot be parsed: %v", err)
	}
}

// TestGetTimeFeedOmitsCursorWhenExhausted 验证没有更多数据时不会返回下一页游标。
func TestGetTimeFeedOmitsCursorWhenExhausted(t *testing.T) {
	repository := &serviceTestRepository{items: makeFeedItems(1)}
	result, err := NewService(repository).GetTimeFeed(context.TODO(), "", 20)
	if err != nil {
		t.Fatalf("GetTimeFeed() error = %v", err)
	}
	if result.HasMore || result.NextCursor != "" {
		t.Fatalf("GetTimeFeed() result = %+v, want no next cursor", result)
	}
}

// TestGetTimeFeedRejectsNegativeLimit 验证负数分页数量不会被当作默认值处理。
func TestGetTimeFeedRejectsNegativeLimit(t *testing.T) {
	repository := &serviceTestRepository{}
	_, err := NewService(repository).GetTimeFeed(context.TODO(), "", -1)
	if !errors.Is(err, domainfeed.ErrInvalidLimit) {
		t.Fatalf("GetTimeFeed() error = %v, want %v", err, domainfeed.ErrInvalidLimit)
	}
	if repository.listLimit != 0 {
		t.Fatalf("ListTimeFeed() limit = %d, want no repository call", repository.listLimit)
	}
}

// TestGetTimeFeedRejectsInvalidCursor 验证无法通过领域校验的游标会被拒绝。
func TestGetTimeFeedRejectsInvalidCursor(t *testing.T) {
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"published_at":"","video_id":7}`))
	_, err := NewService(&serviceTestRepository{}).GetTimeFeed(context.TODO(), payload, 20)
	if !errors.Is(err, domainfeed.ErrInvalidCursor) {
		t.Fatalf("GetTimeFeed() error = %v, want %v", err, domainfeed.ErrInvalidCursor)
	}
}

// TestReportViewEventCreatesEvent 验证没有幂等键时会创建新的观看事件。
func TestReportViewEventCreatesEvent(t *testing.T) {
	repository := &serviceTestRepository{}
	result, err := NewService(repository).ReportViewEvent(context.TODO(), nil, "visitor-1", 7, "view", 1200, "")
	if err != nil {
		t.Fatalf("ReportViewEvent() error = %v", err)
	}
	if result == nil || result.Event == nil || !result.Created || repository.saveEvent == nil {
		t.Fatalf("ReportViewEvent() result = %+v, want created event", result)
	}
	if repository.saveEvent.EventType != domainfeed.EventTypeView || repository.saveEvent.WatchMS != 1200 {
		t.Fatalf("saved event = %+v, want normalized event", repository.saveEvent)
	}
}

// TestReportViewEventReplaysExistingEvent 验证幂等键命中时直接返回原有事件。
func TestReportViewEventReplaysExistingEvent(t *testing.T) {
	existing := &domainfeed.ViewEvent{ID: 9, VideoID: 7, EventType: domainfeed.EventTypeView}
	repository := &serviceTestRepository{findEvent: existing}
	result, err := NewService(repository).ReportViewEvent(context.TODO(), nil, "visitor-1", 7, domainfeed.EventTypeView, 1200, "event-1")
	if err != nil {
		t.Fatalf("ReportViewEvent() error = %v", err)
	}
	if result == nil || result.Event != existing || result.Created || repository.saveEvent != nil {
		t.Fatalf("ReportViewEvent() result = %+v, want replayed event", result)
	}
}

// TestReportViewEventRejectsNilReplay 验证幂等查询返回空事件时会返回应用层错误。
func TestReportViewEventRejectsNilReplay(t *testing.T) {
	repository := &serviceTestRepository{findErr: nil}
	_, err := NewService(repository).ReportViewEvent(context.TODO(), nil, "visitor-1", 7, domainfeed.EventTypeView, 1200, "event-1")
	if !errors.Is(err, ErrLoadViewEventFailed) {
		t.Fatalf("ReportViewEvent() error = %v, want %v", err, ErrLoadViewEventFailed)
	}
}

// TestReportViewEventMapsDuplicateReplayFailure 验证幂等冲突后无法回查事件时返回加载错误。
func TestReportViewEventMapsDuplicateReplayFailure(t *testing.T) {
	repository := &serviceTestRepository{
		findErr:   domainfeed.ErrViewEventNotFound,
		saveErr:   domainfeed.ErrDuplicateIdempotencyKey,
		findEvent: nil,
	}
	_, err := NewService(repository).ReportViewEvent(context.TODO(), nil, "visitor-1", 7, domainfeed.EventTypeView, 1200, "event-1")
	if !errors.Is(err, ErrLoadViewEventFailed) {
		t.Fatalf("ReportViewEvent() error = %v, want %v", err, ErrLoadViewEventFailed)
	}
}

// TestTimeCursorRoundTrip 验证时间游标编码后可以无损解析。
func TestTimeCursorRoundTrip(t *testing.T) {
	original := &domainfeed.TimeCursor{PublishedAt: time.Date(2026, time.August, 30, 12, 0, 0, 0, time.UTC), VideoID: 7}
	encoded := encodeTimeCursor(original)
	decoded, err := parseTimeCursor(encoded)
	if err != nil {
		t.Fatalf("parseTimeCursor() error = %v", err)
	}
	if decoded.VideoID != original.VideoID || !decoded.PublishedAt.Equal(original.PublishedAt) {
		t.Fatalf("decoded cursor = %+v, want %+v", decoded, original)
	}
}

// makeFeedItems 创建指定数量的按时间递减信息流测试数据。
func makeFeedItems(count int) []*domainfeed.FeedItem {
	items := make([]*domainfeed.FeedItem, 0, count)
	base := time.Date(2026, time.August, 30, 12, 0, 0, 0, time.UTC)
	for i := 0; i < count; i++ {
		items = append(items, &domainfeed.FeedItem{
			VideoID:     int64(count - i),
			Title:       fmt.Sprintf("video-%d", count-i),
			PublishedAt: base.Add(-time.Duration(i) * time.Minute),
		})
	}
	return items
}

var _ domainfeed.Repository = (*serviceTestRepository)(nil)
