package interaction

import (
	"context"
	"errors"
	domaininteraction "github.com/inumoroha/video-feed/internal/domain/interaction"
	"sync"
	"testing"
)

// memoryInteractionMessageWriter 使用互斥锁记录互动通知。
type memoryInteractionMessageWriter struct {
	mu       sync.Mutex
	messages []memoryInteractionMessage
}

// memoryInteractionMessage 保存互动通知的断言字段。
type memoryInteractionMessage struct {
	UserID         int64
	Type           string
	Title          string
	EventID        string
	ActorID        int64
	ActorNickname  string
	ActorAvatarURL string
}

// TestInteractionMessageWriter 验证点赞重放不重复通知、评论通知及自评跳过。
func TestInteractionMessageWriter(t *testing.T) {
	repo := newNotificationRepo()
	writer := newMemoryInteractionMessageWriter()
	service := NewService(repo, WithMessageWriter(writer))

	if _, err := service.Like(context.Background(), 77, 1001, "like-notify-1"); err != nil {
		t.Fatalf("like: %v", err)
	}
	if _, err := service.Like(context.Background(), 77, 1001, "like-notify-1"); err != nil {
		t.Fatalf("like replay: %v", err)
	}
	if _, err := service.CreateComment(context.Background(), 77, 1001, "notify comment", "comment-notify-1"); err != nil {
		t.Fatalf("comment: %v", err)
	}
	if _, err := service.CreateComment(context.Background(), 42, 1001, "own comment", "comment-own-1"); err != nil {
		t.Fatalf("own comment: %v", err)
	}

	messages := writer.Messages()
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %+v", messages)
	}
	if messages[0].UserID != 42 || messages[0].Type != "LIKE" {
		t.Fatalf("unexpected like message: %+v", messages[0])
	}
	if messages[0].ActorID != 77 || messages[0].ActorNickname != "actor-77" || messages[0].ActorAvatarURL != "https://example.com/avatar-77.jpg" {
		t.Fatalf("unexpected like actor: %+v", messages[0])
	}
	if messages[1].UserID != 42 || messages[1].Type != "COMMENT" {
		t.Fatalf("unexpected comment message: %+v", messages[1])
	}
	if messages[1].ActorID != 77 || messages[1].ActorNickname != "actor-77" || messages[1].ActorAvatarURL != "https://example.com/avatar-77.jpg" {
		t.Fatalf("unexpected comment actor: %+v", messages[1])
	}
}

// newMemoryInteractionMessageWriter 初始化空的互动通知记录器。
func newMemoryInteractionMessageWriter() *memoryInteractionMessageWriter {
	return &memoryInteractionMessageWriter{messages: []memoryInteractionMessage{}}
}

// CreateFromEvent 加锁记录互动消息字段。
func (w *memoryInteractionMessageWriter) CreateFromEvent(ctx context.Context, userID int64, messageType string, title string, content string, eventID string, idempotencyKey string) (any, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.messages = append(w.messages, memoryInteractionMessage{
		UserID:  userID,
		Type:    messageType,
		Title:   title,
		EventID: eventID,
	})
	return nil, nil
}

// CreateFromActorEvent 加锁记录带触发用户资料的互动消息字段。
func (w *memoryInteractionMessageWriter) CreateFromActorEvent(ctx context.Context, userID int64, messageType string, title string, content string, eventID string, idempotencyKey string, actorID int64, actorNickname string, actorAvatarURL string) (any, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.messages = append(w.messages, memoryInteractionMessage{
		UserID:         userID,
		Type:           messageType,
		Title:          title,
		EventID:        eventID,
		ActorID:        actorID,
		ActorNickname:  actorNickname,
		ActorAvatarURL: actorAvatarURL,
	})
	return nil, nil
}

// Messages 加锁复制已记录的消息，避免暴露内部切片。
func (w *memoryInteractionMessageWriter) Messages() []memoryInteractionMessage {
	w.mu.Lock()
	defer w.mu.Unlock()

	items := make([]memoryInteractionMessage, len(w.messages))
	copy(items, w.messages)
	return items
}

// notificationRepo 适配现有仓储夹具并模拟事务变化量与幂等评论。
type notificationRepo struct {
	interactionServiceRepository
	seen        map[string]bool
	authorErr   error
	actionDelta int
}

// newNotificationRepo 初始化作者为 42 的通知测试仓储。
func newNotificationRepo() *notificationRepo {
	return &notificationRepo{interactionServiceRepository: interactionServiceRepository{lookupErr: domaininteraction.ErrCommentNotFound}, seen: map[string]bool{}, actionDelta: 1}
}

// GetVideoAuthorID 返回测试视频作者，或注入的查询错误。
func (r *notificationRepo) GetVideoAuthorID(context.Context, int64) (int64, error) {
	return 42, r.authorErr
}

// GetVideoStat 返回异步流程的初始互动计数。
func (r *notificationRepo) GetVideoStat(context.Context, int64) (*domaininteraction.VideoStat, error) {
	return &domaininteraction.VideoStat{VideoID: 1001}, nil
}

// SetActionWithDelta 模拟首次有效点赞和幂等重放，返回实际变化量。
func (r *notificationRepo) SetActionWithDelta(ctx context.Context, u, v int64, typ string, active bool, key string) (*domaininteraction.Action, int, int, int, error) {
	delta := r.actionDelta
	if r.seen[key] {
		delta = 0
	}
	r.seen[key] = true
	status := domaininteraction.ActionStatusCanceled
	if active {
		status = domaininteraction.ActionStatusActive
	}
	return &domaininteraction.Action{UserID: u, VideoID: v, ActionType: typ, Status: status}, 1, 0, delta, r.actionErr
}

// CreateCommentWithDelta 模拟创建评论并为重复键返回零新增量。
func (r *notificationRepo) CreateCommentWithDelta(ctx context.Context, c *domaininteraction.Comment) (*domaininteraction.Comment, int, int, error) {
	delta := 1
	if r.seen[c.IdempotencyKey] {
		delta = 0
	}
	r.seen[c.IdempotencyKey] = true
	c.ID = int64(len(r.seen))
	return c, 1, delta, nil
}

// notificationPipeline 提供可控异步状态和发布失败，用于回退测试。
type notificationPipeline struct {
	delta      int
	publishErr error
	storeErr   error
}

// SetActionState 返回预设变化量或缓存错误。
func (p *notificationPipeline) SetActionState(_ context.Context, u, v int64, typ string, active bool, key string, _ *domaininteraction.VideoStat) (*ActionStateResult, error) {
	return &ActionStateResult{VideoID: v, ActionType: typ, Active: active, Delta: p.delta}, p.storeErr
}

// PublishActionChanged 模拟发布成功或失败。
func (p *notificationPipeline) PublishActionChanged(context.Context, *ActionChangedEvent) error {
	return p.publishErr
}

// failingMessageWriter 记录尝试次数并返回通知写入错误。
type failingMessageWriter struct{ calls int }

// CreateFromEvent 模拟消息库不可用，不应让互动主请求失败。
func (w *failingMessageWriter) CreateFromEvent(context.Context, int64, string, string, string, string, string) (any, error) {
	w.calls++
	return nil, errors.New("message unavailable")
}

// TestMessageAsyncFallback 覆盖异步成功、重放、缓存错误、发布失败回退及数据库失败。
func TestMessageAsyncFallback(t *testing.T) {
	for _, tc := range []struct {
		name                        string
		delta                       int
		publishErr, storeErr, dbErr error
		want                        int
		wantErr                     bool
	}{
		{"async", 1, nil, nil, nil, 1, false}, {"replay", 0, nil, nil, nil, 0, false},
		{"publish fallback", 1, errors.New("mq down"), nil, nil, 1, false},
		{"fallback fails", 1, errors.New("mq down"), nil, errors.New("db down"), 0, true},
		{"cache fails", 1, nil, errors.New("redis down"), nil, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newNotificationRepo()
			repo.actionErr = tc.dbErr
			writer := newMemoryInteractionMessageWriter()
			p := &notificationPipeline{delta: tc.delta, publishErr: tc.publishErr, storeErr: tc.storeErr}
			s := NewService(repo, WithMessageWriter(writer), WithAsyncActionPipeline(p, p))
			_, err := s.Like(context.Background(), 77, 1001, "a")
			if (err != nil) != tc.wantErr || len(writer.Messages()) != tc.want {
				t.Fatalf("err=%v messages=%v", err, writer.Messages())
			}
		})
	}
}

// TestMessageNotificationFailures 验证作者查询、自赞、收藏、取消及通知失败不产生错误通知。
func TestMessageNotificationFailures(t *testing.T) {
	ctx := context.Background()
	repo := newNotificationRepo()
	writer := newMemoryInteractionMessageWriter()
	s := NewService(repo, WithMessageWriter(writer))
	_, _ = s.Like(ctx, 42, 1001, "own")
	_, _ = s.Favorite(ctx, 77, 1001, "favorite")
	_, _ = s.Unlike(ctx, 77, 1001, "unlike")
	repo.authorErr = errors.New("lookup failed")
	_, _ = s.Like(ctx, 77, 1001, "lookup")
	if len(writer.Messages()) != 0 {
		t.Fatal(writer.Messages())
	}
	repo.authorErr = nil
	failing := &failingMessageWriter{}
	s = NewService(repo, WithMessageWriter(failing))
	if _, err := s.Like(ctx, 77, 1001, "fail-message"); err != nil || failing.calls != 1 {
		t.Fatalf("err=%v calls=%d", err, failing.calls)
	}
	if _, err := s.CreateComment(ctx, 77, 1001, "text", "c"); err != nil || failing.calls != 2 {
		t.Fatalf("err=%v calls=%d", err, failing.calls)
	}
}
