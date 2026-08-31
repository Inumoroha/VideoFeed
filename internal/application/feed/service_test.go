package feed

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	domainfeed "github.com/inumoroha/video-feed/internal/domain/feed"
)

// serviceTestRepository 提供信息流应用服务测试所需的内存仓储实现。
type serviceTestRepository struct {
	items             []*domainfeed.FeedItem
	listLimit         int
	listCursor        *domainfeed.TimeCursor
	savedCursor       *domainfeed.TimeCursor
	savedCursorToken  string
	savedCursorExpiry time.Time
	saveCursorErr     error
	findCursor        *domainfeed.TimeCursor
	findCursorErr     error
	findEvent         *domainfeed.ViewEvent
	findErr           error
	saveEvent         *domainfeed.ViewEvent
	saveErr           error
	findCalls         int
}

// ListTimeFeed 返回测试信息流并记录服务请求的查询数量。
func (r *serviceTestRepository) ListTimeFeed(_ context.Context, cursor *domainfeed.TimeCursor, limit int) ([]*domainfeed.FeedItem, error) {
	r.listLimit = limit
	r.listCursor = cloneTimeCursor(cursor)
	return r.items, nil
}

// SaveTimeCursor 记录服务生成的分页游标。
func (r *serviceTestRepository) SaveTimeCursor(_ context.Context, token string, cursor *domainfeed.TimeCursor, expiresAt time.Time) error {
	r.savedCursorToken = token
	r.savedCursor = cloneTimeCursor(cursor)
	r.savedCursorExpiry = expiresAt
	return r.saveCursorErr
}

// FindTimeCursor 返回预设或此前保存的分页游标。
func (r *serviceTestRepository) FindTimeCursor(_ context.Context, token string, now time.Time) (*domainfeed.TimeCursor, error) {
	if r.findCursorErr != nil {
		return nil, r.findCursorErr
	}
	if r.findCursor != nil {
		return cloneTimeCursor(r.findCursor), nil
	}
	if token == r.savedCursorToken && r.savedCursor != nil && r.savedCursorExpiry.After(now) {
		return cloneTimeCursor(r.savedCursor), nil
	}
	return nil, domainfeed.ErrTimeCursorNotFound
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
	now := time.Date(2026, time.August, 31, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }

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
	if !strings.HasPrefix(result.NextCursor, timeCursorTokenPrefix) || repository.savedCursor == nil {
		t.Fatalf("next cursor = %q, want persisted cursor", result.NextCursor)
	}
	if !repository.savedCursorExpiry.Equal(now.Add(timeCursorTTL)) {
		t.Fatalf("saved cursor expiry = %s, want %s", repository.savedCursorExpiry, now.Add(timeCursorTTL))
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

// TestGetTimeFeedUsesPersistedCursor 验证服务会读取服务端保存的分页游标。
func TestGetTimeFeedUsesPersistedCursor(t *testing.T) {
	repository := &serviceTestRepository{items: makeFeedItems(2)}
	service := NewService(repository)

	firstPage, err := service.GetTimeFeed(context.TODO(), "", 1)
	if err != nil {
		t.Fatalf("GetTimeFeed() first page error = %v", err)
	}
	if firstPage.NextCursor == "" {
		t.Fatal("GetTimeFeed() first page did not return a cursor")
	}

	if _, err := service.GetTimeFeed(context.TODO(), firstPage.NextCursor, 1); err != nil {
		t.Fatalf("GetTimeFeed() persisted cursor error = %v", err)
	}
	if repository.listCursor == nil || repository.savedCursor == nil || repository.listCursor.VideoID != repository.savedCursor.VideoID || !repository.listCursor.PublishedAt.Equal(repository.savedCursor.PublishedAt) {
		t.Fatalf("ListTimeFeed() cursor = %+v, want saved cursor %+v", repository.listCursor, repository.savedCursor)
	}
}

// TestGetTimeFeedRejectsExpiredPersistedCursor 验证服务会拒绝不存在或已过期的服务端游标。
func TestGetTimeFeedRejectsExpiredPersistedCursor(t *testing.T) {
	service := NewService(&serviceTestRepository{})
	_, err := service.GetTimeFeed(context.TODO(), timeCursorTokenPrefix+"expired", 20)
	if !errors.Is(err, domainfeed.ErrInvalidCursor) {
		t.Fatalf("GetTimeFeed() error = %v, want %v", err, domainfeed.ErrInvalidCursor)
	}
}

// TestGetTimeFeedReturnsErrorWhenCursorCannotBeSaved 验证游标保存失败时不会返回无法继续分页的结果。
func TestGetTimeFeedReturnsErrorWhenCursorCannotBeSaved(t *testing.T) {
	repository := &serviceTestRepository{items: makeFeedItems(2), saveCursorErr: errors.New("database unavailable")}
	_, err := NewService(repository).GetTimeFeed(context.TODO(), "", 1)
	if !errors.Is(err, ErrSaveTimeCursorFailed) {
		t.Fatalf("GetTimeFeed() error = %v, want %v", err, ErrSaveTimeCursorFailed)
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

// cloneTimeCursor 复制分页游标，避免测试仓储共享可变指针。
func cloneTimeCursor(cursor *domainfeed.TimeCursor) *domainfeed.TimeCursor {
	if cursor == nil {
		return nil
	}
	cloned := *cursor
	return &cloned
}

var _ domainfeed.Repository = (*serviceTestRepository)(nil)
