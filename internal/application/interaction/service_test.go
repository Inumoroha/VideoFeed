package interaction

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	domaininteraction "github.com/inumoroha/video-feed/internal/domain/interaction"
)

// interactionServiceRepository 是服务测试使用的最小内存仓储。
type interactionServiceRepository struct {
	lookupComment   *domaininteraction.Comment
	lookupErr       error
	lookupCalls     int
	listLimit       int
	listComments    []*domaininteraction.Comment
	action          *domaininteraction.Action
	actionErr       error
	likeCount       int
	favoriteCount   int
	setActionCalls  int
	setActionActive bool
	setActionType   string
	deleteComment   *domaininteraction.Comment
	deleteCount     int
	deleteErr       error
	createdComment  *domaininteraction.Comment
}

// SetAction 记录显式互动状态并返回预设的互动动作结果。
func (r *interactionServiceRepository) SetAction(_ context.Context, _ int64, _ int64, actionType string, active bool, _ string) (*domaininteraction.Action, int, int, error) {
	r.setActionCalls++
	r.setActionActive = active
	r.setActionType = actionType
	return r.action, r.likeCount, r.favoriteCount, r.actionErr
}

// CreateComment 返回预设的评论创建结果。
func (r *interactionServiceRepository) CreateComment(_ context.Context, comment *domaininteraction.Comment) (*domaininteraction.Comment, int, error) {
	if r.createdComment != nil {
		return r.createdComment, 1, nil
	}
	return comment, 1, nil
}

// FindCommentByUserAndIdempotencyKey 返回预设的幂等评论结果。
func (r *interactionServiceRepository) FindCommentByUserAndIdempotencyKey(context.Context, int64, string) (*domaininteraction.Comment, int, error) {
	r.lookupCalls++
	return r.lookupComment, 1, r.lookupErr
}

// ListComments 记录服务传入的分页数量并返回预设评论。
func (r *interactionServiceRepository) ListComments(_ context.Context, _ int64, _ *domaininteraction.CommentCursor, limit int) ([]*domaininteraction.Comment, error) {
	r.listLimit = limit
	return r.listComments, nil
}

// DeleteComment 返回预设的评论删除结果。
func (r *interactionServiceRepository) DeleteComment(context.Context, int64, int64, string) (*domaininteraction.Comment, int, error) {
	return r.deleteComment, r.deleteCount, r.deleteErr
}

// GetUserProfile 返回可预测的昵称和头像，供通知用例断言触发者展示信息。
func (r *interactionServiceRepository) GetUserProfile(_ context.Context, userID int64) (*domaininteraction.UserProfile, error) {
	if userID <= 0 {
		return nil, domaininteraction.ErrInvalidUserID
	}
	return &domaininteraction.UserProfile{
		ID:        userID,
		Nickname:  fmt.Sprintf("actor-%d", userID),
		AvatarURL: fmt.Sprintf("https://example.com/avatar-%d.jpg", userID),
	}, nil
}

// TestServiceRejectsUninitializedDependencies 验证服务依赖缺失时返回明确错误。
func TestServiceRejectsUninitializedDependencies(t *testing.T) {
	var service *Service
	if _, err := service.Like(context.Background(), 1, 1, ""); !errors.Is(err, ErrInteractionServiceNotInitialized) {
		t.Fatalf("Like() error = %v, want %v", err, ErrInteractionServiceNotInitialized)
	}
	if _, err := NewService(nil).ListComments(context.Background(), 1, "", 0); !errors.Is(err, ErrInteractionServiceNotInitialized) {
		t.Fatalf("ListComments() error = %v, want %v", err, ErrInteractionServiceNotInitialized)
	}
}

// TestCreateCommentValidatesBeforeIdempotencyLookup 验证无效评论不会绕过领域校验命中幂等记录。
func TestCreateCommentValidatesBeforeIdempotencyLookup(t *testing.T) {
	repository := &interactionServiceRepository{
		lookupComment: &domaininteraction.Comment{ID: 7},
	}
	service := NewService(repository)

	if _, err := service.CreateComment(context.Background(), 1, 0, "content", "request-1"); !errors.Is(err, domaininteraction.ErrInvalidVideoID) {
		t.Fatalf("CreateComment() error = %v, want %v", err, domaininteraction.ErrInvalidVideoID)
	}
	if repository.lookupCalls != 0 {
		t.Fatalf("CreateComment() lookup calls = %d, want 0", repository.lookupCalls)
	}
}

// TestListCommentsNormalizesLimit 验证负数分页被拒绝且零值使用默认数量。
func TestListCommentsNormalizesLimit(t *testing.T) {
	repository := &interactionServiceRepository{}
	service := NewService(repository)

	if _, err := service.ListComments(context.Background(), 1, "", -1); !errors.Is(err, domaininteraction.ErrInvalidLimit) {
		t.Fatalf("ListComments() error = %v, want %v", err, domaininteraction.ErrInvalidLimit)
	}
	if _, err := service.ListComments(context.Background(), 1, "", 0); err != nil {
		t.Fatalf("ListComments() error = %v, want nil", err)
	}
	if repository.listLimit != defaultCommentLimit+1 {
		t.Fatalf("ListComments() repository limit = %d, want %d", repository.listLimit, defaultCommentLimit+1)
	}
}

// memoryInteractionStatCache 使用互斥锁记录被同步的视频计数。
type memoryInteractionStatCache struct {
	mu    sync.Mutex
	stats map[int64]*domaininteraction.VideoStat
}

// newMemoryInteractionStatCache 初始化空的互动计数缓存替身。
func newMemoryInteractionStatCache() *memoryInteractionStatCache {
	return &memoryInteractionStatCache{stats: map[int64]*domaininteraction.VideoStat{}}
}

// SetVideoStat 加锁保存视频计数副本。
func (c *memoryInteractionStatCache) SetVideoStat(_ context.Context, stat *domaininteraction.VideoStat) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	cloned := *stat
	c.stats[stat.VideoID] = &cloned
	return nil
}

// StatForTest 加锁读取视频计数副本，未记录时返回空值。
func (c *memoryInteractionStatCache) StatForTest(videoID int64) *domaininteraction.VideoStat {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stats[videoID] == nil {
		return nil
	}
	cloned := *c.stats[videoID]
	return &cloned
}

// TestInteractionCommentSyncsStatCache 验证新增和删除评论都会把评论数同步到计数缓存。
func TestInteractionCommentSyncsStatCache(t *testing.T) {
	repository := &interactionServiceRepository{
		lookupErr:      domaininteraction.ErrCommentNotFound,
		createdComment: &domaininteraction.Comment{ID: 11, VideoID: 1001, UserID: 77, Content: "cache comment", Status: domaininteraction.CommentStatusNormal},
	}
	cache := newMemoryInteractionStatCache()
	service := NewService(repository, WithStatCache(cache))

	created, err := service.CreateComment(context.Background(), 77, 1001, "cache comment", "comment-cache-1")
	if err != nil {
		t.Fatalf("create comment: %v", err)
	}
	stat := cache.StatForTest(1001)
	if stat == nil || stat.CommentCount != 1 {
		t.Fatalf("expected comment count cache to be 1, got %+v", stat)
	}

	repository.deleteComment = &domaininteraction.Comment{ID: created.Comment.ID, VideoID: 1001, Status: domaininteraction.CommentStatusDeleted}
	repository.deleteCount = 0
	if _, err := service.DeleteComment(context.Background(), created.Comment.ID, 77, "user"); err != nil {
		t.Fatalf("delete comment: %v", err)
	}
	stat = cache.StatForTest(1001)
	if stat == nil || stat.CommentCount != 0 {
		t.Fatalf("expected comment count cache to be 0, got %+v", stat)
	}
}

// TestInteractionStatCacheWithoutProviderUsesZeroBase 验证仓储不支持计数读取时仍以零计数基数同步评论数。
func TestInteractionStatCacheWithoutProviderUsesZeroBase(t *testing.T) {
	repository := &interactionServiceRepository{
		lookupErr:      domaininteraction.ErrCommentNotFound,
		createdComment: &domaininteraction.Comment{ID: 12, VideoID: 1002, UserID: 77, Content: "text", Status: domaininteraction.CommentStatusNormal},
	}
	cache := newMemoryInteractionStatCache()
	service := NewService(repository, WithStatCache(cache))

	if _, err := service.CreateComment(context.Background(), 77, 1002, "text", "comment-cache-2"); err != nil {
		t.Fatalf("create comment: %v", err)
	}
	stat := cache.StatForTest(1002)
	if stat == nil || stat.CommentCount != 1 || stat.LikeCount != 0 || stat.FavoriteCount != 0 {
		t.Fatalf("unexpected stat cache: %+v", stat)
	}
}

// TestLikeReturnsBothActionCounts 验证点赞响应同时包含点赞和收藏统计。
func TestLikeReturnsBothActionCounts(t *testing.T) {
	repository := &interactionServiceRepository{
		action:        &domaininteraction.Action{VideoID: 3, ActionType: domaininteraction.ActionTypeLike, Status: domaininteraction.ActionStatusActive},
		likeCount:     8,
		favoriteCount: 5,
	}
	result, err := NewService(repository).Like(context.Background(), 1, 3, "")
	if err != nil {
		t.Fatalf("Like() error = %v", err)
	}
	if result.LikeCount != 8 || result.FavoriteCount != 5 {
		t.Fatalf("Like() counts = (%d, %d), want (8, 5)", result.LikeCount, result.FavoriteCount)
	}
	if repository.setActionCalls != 1 || !repository.setActionActive || repository.setActionType != domaininteraction.ActionTypeLike {
		t.Fatalf("Like() SetAction call = (calls=%d, active=%t, type=%q), want (1, true, %q)", repository.setActionCalls, repository.setActionActive, repository.setActionType, domaininteraction.ActionTypeLike)
	}
}

// TestUnlikePassesInactiveState 验证取消点赞会向仓储传递 active=false。
func TestUnlikePassesInactiveState(t *testing.T) {
	repository := &interactionServiceRepository{
		action: &domaininteraction.Action{VideoID: 3, ActionType: domaininteraction.ActionTypeLike, Status: domaininteraction.ActionStatusCanceled},
	}
	result, err := NewService(repository).Unlike(context.Background(), 1, 3, "request-1")
	if err != nil {
		t.Fatalf("Unlike() error = %v", err)
	}
	if result.Active {
		t.Fatalf("Unlike() active = %t, want false", result.Active)
	}
	if repository.setActionCalls != 1 || repository.setActionActive || repository.setActionType != domaininteraction.ActionTypeLike {
		t.Fatalf("Unlike() SetAction call = (calls=%d, active=%t, type=%q), want (1, false, %q)", repository.setActionCalls, repository.setActionActive, repository.setActionType, domaininteraction.ActionTypeLike)
	}
}

// TestCommentCursorRoundTrip 验证评论分页游标编码后可以恢复原始位置。
func TestCommentCursorRoundTrip(t *testing.T) {
	want := &domaininteraction.CommentCursor{
		CreatedAt: time.Date(2026, 9, 2, 12, 0, 0, 0, time.FixedZone("CST", 8*60*60)),
		CommentID: 9,
	}
	got, err := parseCommentCursor(encodeCommentCursor(want))
	if err != nil {
		t.Fatalf("parseCommentCursor() error = %v", err)
	}
	if !got.CreatedAt.Equal(want.CreatedAt) || got.CommentID != want.CommentID {
		t.Fatalf("cursor = %+v, want %+v", got, want)
	}
}

var _ domaininteraction.Repository = (*interactionServiceRepository)(nil)
