package relation

import (
	"context"
	"errors"
	domainrelation "github.com/inumoroha/video-feed/internal/domain/relation"
	"sync"
	"testing"
)

// memoryRelationMessageWriter 使用互斥锁保存关注通知并按事件去重。
type memoryRelationMessageWriter struct {
	mu       sync.Mutex
	messages []memoryRelationMessage
	seen     map[string]struct{}
}

// memoryRelationMessage 保存关注通知的断言字段。
type memoryRelationMessage struct {
	UserID         int64
	Type           string
	Title          string
	EventID        string
	ActorID        int64
	ActorNickname  string
	ActorAvatarURL string
}

// TestRelationMessageWriter 验证首次关注发出通知、重复关注去重及取关不发通知。
func TestRelationMessageWriter(t *testing.T) {
	repo := newNotificationRelationRepo()
	writer := newMemoryRelationMessageWriter()
	service := NewService(repo, WithMessageWriter(writer))

	if _, err := service.Follow(context.Background(), 42, 77, "follow-message-1"); err != nil {
		t.Fatalf("follow: %v", err)
	}
	if _, err := service.Follow(context.Background(), 42, 77, "follow-message-1"); err != nil {
		t.Fatalf("follow replay: %v", err)
	}
	if _, err := service.Unfollow(context.Background(), 42, 77, "unfollow-message-1"); err != nil {
		t.Fatalf("unfollow: %v", err)
	}

	messages := writer.Messages()
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %+v", messages)
	}
	if messages[0].UserID != 77 || messages[0].Type != "FOLLOW" {
		t.Fatalf("unexpected follow message: %+v", messages[0])
	}
	if messages[0].ActorID != 42 || messages[0].ActorNickname != "viewer-42" || messages[0].ActorAvatarURL != "https://example.com/42.jpg" {
		t.Fatalf("unexpected follow actor: %+v", messages[0])
	}
}

// newMemoryRelationMessageWriter 初始化关注通知和事件去重容器。
func newMemoryRelationMessageWriter() *memoryRelationMessageWriter {
	return &memoryRelationMessageWriter{
		messages: []memoryRelationMessage{},
		seen:     map[string]struct{}{},
	}
}

// CreateFromEvent 加锁记录关注消息，同一事件重复调用直接返回。
func (w *memoryRelationMessageWriter) CreateFromEvent(ctx context.Context, userID int64, messageType string, title string, content string, eventID string, idempotencyKey string) (any, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if _, exists := w.seen[eventID]; exists {
		return nil, nil
	}
	w.seen[eventID] = struct{}{}
	w.messages = append(w.messages, memoryRelationMessage{
		UserID:  userID,
		Type:    messageType,
		Title:   title,
		EventID: eventID,
	})
	return nil, nil
}

// CreateFromActorEvent 加锁记录带触发用户资料的关注消息，同一事件重复调用直接返回。
func (w *memoryRelationMessageWriter) CreateFromActorEvent(ctx context.Context, userID int64, messageType string, title string, content string, eventID string, idempotencyKey string, actorID int64, actorNickname string, actorAvatarURL string) (any, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if _, exists := w.seen[eventID]; exists {
		return nil, nil
	}
	w.seen[eventID] = struct{}{}
	w.messages = append(w.messages, memoryRelationMessage{
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
func (w *memoryRelationMessageWriter) Messages() []memoryRelationMessage {
	w.mu.Lock()
	defer w.mu.Unlock()

	items := make([]memoryRelationMessage, len(w.messages))
	copy(items, w.messages)
	return items
}

// newNotificationRelationRepo 返回有效关注状态和双方统计的测试仓储。
func newNotificationRelationRepo() *relationServiceRepository {
	return &relationServiceRepository{follow: &domainrelation.Follow{UserID: 42, TargetUserID: 77, Status: domainrelation.FollowStatusActive}, userStat: &domainrelation.RelationStat{}, targetStat: &domainrelation.RelationStat{}}
}

// failingFollowWriter 记录调用次数并模拟消息写入失败。
type failingFollowWriter struct{ calls int }

// CreateFromEvent 返回消息保存错误以验证主关注请求仍然成功。
func (w *failingFollowWriter) CreateFromEvent(context.Context, int64, string, string, string, string, string) (any, error) {
	w.calls++
	return nil, errors.New("message failed")
}

// TestFollowMessageFailureIsBestEffort 验证通知失败不改变成功关注结果。
func TestFollowMessageFailureIsBestEffort(t *testing.T) {
	w := &failingFollowWriter{}
	s := NewService(newNotificationRelationRepo(), WithMessageWriter(w))
	if _, err := s.Follow(context.Background(), 42, 77, "k"); err != nil || w.calls != 1 {
		t.Fatalf("err=%v calls=%d", err, w.calls)
	}
}

// TestFollowNotificationAfterBackfillRetry 验证回填失败不发通知，修复后使用同一请求键重试只产生一条消息。
func TestFollowNotificationAfterBackfillRetry(t *testing.T) {
	repo := newNotificationRelationRepo()
	writer := newMemoryRelationMessageWriter()
	backfill := &failingBackfiller{stage: "count"}
	s := NewService(repo, WithMessageWriter(writer), WithFollowFeedBackfiller(backfill))
	ctx := context.Background()
	if _, err := s.Follow(ctx, 42, 77, "retry"); !errors.Is(err, ErrBackfillFollowFeedFailed) || len(writer.Messages()) != 0 {
		t.Fatalf("err=%v messages=%v", err, writer.Messages())
	}
	backfill.stage = ""
	for i := 0; i < 2; i++ {
		if _, err := s.Follow(ctx, 42, 77, "retry"); err != nil {
			t.Fatal(err)
		}
	}
	if len(writer.Messages()) != 1 {
		t.Fatal(writer.Messages())
	}
}
