package interaction

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	domaininteraction "github.com/inumoroha/video-feed/internal/domain/interaction"
)

const defaultCommentLimit = 20
const hotScoreLikeWeight = 3
const hotScoreFavoriteWeight = 4
const hotScoreCommentWeight = 5

// Service 编排点赞、收藏和评论相关的应用用例。
type Service struct {
	repo     domaininteraction.Repository
	hotCache interface {
		AddHotScore(context.Context, int64, int, time.Time) error
	}
}

// WithHotScoreCache 配置用于记录互动热度的缓存。
func WithHotScoreCache(cache interface {
	AddHotScore(context.Context, int64, int, time.Time) error
}) func(*Service) {
	return func(s *Service) { s.hotCache = cache }
}

// commentCursorPayload 表示评论分页游标的编码载荷。
type commentCursorPayload struct {
	CreatedAt string `json:"created_at"`
	CommentID int64  `json:"comment_id"`
}

// NewService 创建使用指定互动仓储的应用服务。
func NewService(repo domaininteraction.Repository, options ...func(*Service)) *Service {
	s := &Service{repo: repo}
	for _, option := range options {
		option(s)
	}
	return s
}

// Like 设置用户对视频为点赞状态并返回最新互动统计。
func (s *Service) Like(ctx context.Context, userID int64, videoID int64, idempotencyKey string) (*ActionResult, error) {
	return s.setAction(ctx, userID, videoID, domaininteraction.ActionTypeLike, true, idempotencyKey)
}

// Unlike 取消点赞并返回最新点赞统计。
func (s *Service) Unlike(ctx context.Context, userID int64, videoID int64, idempotencyKey string) (*ActionResult, error) {
	return s.setAction(ctx, userID, videoID, domaininteraction.ActionTypeLike, false, idempotencyKey)
}

// Favorite 收藏视频并返回最新收藏统计。
func (s *Service) Favorite(ctx context.Context, userID int64, videoID int64, idempotencyKey string) (*ActionResult, error) {
	return s.setAction(ctx, userID, videoID, domaininteraction.ActionTypeFavorite, true, idempotencyKey)
}

// Unfavorite 取消收藏视频并返回最新收藏统计。
func (s *Service) Unfavorite(ctx context.Context, userID int64, videoID int64, idempotencyKey string) (*ActionResult, error) {
	return s.setAction(ctx, userID, videoID, domaininteraction.ActionTypeFavorite, false, idempotencyKey)
}

// CreateComment 校验参数、处理幂等重试并创建视频评论。
func (s *Service) CreateComment(ctx context.Context, userID int64, videoID int64, content string, idempotencyKey string) (*CreateCommentResult, error) {
	if !s.ready() {
		return nil, ErrInteractionServiceNotInitialized
	}

	comment, err := domaininteraction.NewComment(videoID, userID, content, idempotencyKey)
	if err != nil {
		return nil, err
	}
	idempotencyKey = comment.IdempotencyKey

	if idempotencyKey != "" {
		existing, count, lookupErr := s.repo.FindCommentByUserAndIdempotencyKey(ctx, userID, idempotencyKey)
		if lookupErr == nil {
			if existing == nil {
				return nil, ErrLoadInteractionFailed
			}
			return &CreateCommentResult{Comment: existing, CommentCount: count}, nil
		}
		if !errors.Is(lookupErr, domaininteraction.ErrCommentNotFound) {
			return nil, ErrLoadInteractionFailed
		}
	}

	created, count, err := s.repo.CreateComment(ctx, comment)
	if err != nil {
		if isExpectedDomainError(err) {
			return nil, err
		}
		return nil, ErrSaveInteractionFailed
	}
	if created == nil {
		return nil, ErrSaveInteractionFailed
	}
	if s.hotCache != nil {
		_ = s.hotCache.AddHotScore(ctx, created.VideoID, hotScoreCommentWeight, time.Now().UTC())
	}

	return &CreateCommentResult{Comment: created, CommentCount: count}, nil
}

// ListComments 查询视频评论并生成下一页游标。
func (s *Service) ListComments(ctx context.Context, videoID int64, cursor string, limit int) (*CommentListResult, error) {
	if !s.ready() {
		return nil, ErrInteractionServiceNotInitialized
	}
	if videoID <= 0 {
		return nil, domaininteraction.ErrInvalidVideoID
	}
	if limit < 0 {
		return nil, domaininteraction.ErrInvalidLimit
	}

	parsedCursor, err := parseCommentCursor(cursor)
	if err != nil {
		return nil, err
	}
	limit = normalizeCommentLimit(limit)

	items, err := s.repo.ListComments(ctx, videoID, parsedCursor, limit+1)
	if err != nil {
		if isExpectedDomainError(err) {
			return nil, err
		}
		return nil, ErrLoadInteractionFailed
	}

	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}

	nextCursor := ""
	if len(items) > 0 {
		nextCursor = encodeCommentCursor(&domaininteraction.CommentCursor{
			CreatedAt: items[len(items)-1].CreatedAt,
			CommentID: items[len(items)-1].ID,
		})
	}

	return &CommentListResult{
		Items:      items,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

// DeleteComment 删除指定评论，并返回删除后的评论统计。
func (s *Service) DeleteComment(ctx context.Context, commentID int64, userID int64, role string) (*DeleteCommentResult, error) {
	if !s.ready() {
		return nil, ErrInteractionServiceNotInitialized
	}
	if commentID <= 0 {
		return nil, domaininteraction.ErrInvalidCommentID
	}
	if userID <= 0 {
		return nil, domaininteraction.ErrInvalidUserID
	}

	comment, count, err := s.repo.DeleteComment(ctx, commentID, userID, role)
	if err != nil {
		if isExpectedDomainError(err) {
			return nil, err
		}
		return nil, ErrUpdateInteractionFailed
	}
	if comment == nil {
		return nil, ErrUpdateInteractionFailed
	}
	if s.hotCache != nil { _ = s.hotCache.AddHotScore(ctx, comment.VideoID, -hotScoreCommentWeight, time.Now().UTC()) }

	return &DeleteCommentResult{
		CommentID:    comment.ID,
		Status:       comment.Status,
		CommentCount: count,
	}, nil
}

// setAction 将指定类型的互动设置为目标 active 状态，并将仓储结果转换为应用结果。
func (s *Service) setAction(ctx context.Context, userID int64, videoID int64, actionType string, active bool, idempotencyKey string) (*ActionResult, error) {
	if !s.ready() {
		return nil, ErrInteractionServiceNotInitialized
	}
	if userID <= 0 {
		return nil, domaininteraction.ErrInvalidUserID
	}
	if videoID <= 0 {
		return nil, domaininteraction.ErrInvalidVideoID
	}
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if len(idempotencyKey) > domaininteraction.MaxIdempotencyKeyLength {
		return nil, domaininteraction.ErrIdempotencyKeyTooLong
	}

	actionType, err := domaininteraction.NormalizeActionType(actionType)
	if err != nil {
		return nil, err
	}

	action, likeCount, favoriteCount, err := s.repo.SetAction(ctx, userID, videoID, actionType, active, idempotencyKey)
	if err != nil {
		if isExpectedDomainError(err) {
			return nil, err
		}
		return nil, ErrUpdateInteractionFailed
	}
	if action == nil {
		return nil, ErrUpdateInteractionFailed
	}
	result := &ActionResult{
		VideoID:       action.VideoID,
		ActionType:    action.ActionType,
		Active:        action.Active(),
		LikeCount:     likeCount,
		FavoriteCount: favoriteCount,
	}
	if action.ActionType != domaininteraction.ActionTypeLike && action.ActionType != domaininteraction.ActionTypeFavorite {
		return nil, ErrUpdateInteractionFailed
	}
	if s.hotCache != nil {
		weight := hotScoreLikeWeight
		if actionType == domaininteraction.ActionTypeFavorite {
			weight = hotScoreFavoriteWeight
		}
		if !active {
			weight = -weight
		}
		_ = s.hotCache.AddHotScore(ctx, action.VideoID, weight, time.Now().UTC())
	}
	return result, nil
}

// normalizeCommentLimit 将零值分页数量转换为默认值，并限制最大返回数量。
func normalizeCommentLimit(limit int) int {
	if limit == 0 {
		return defaultCommentLimit
	}
	if limit > domaininteraction.MaxLimit {
		return domaininteraction.MaxLimit
	}
	return limit
}

// parseCommentCursor 解码并校验评论分页游标。
func parseCommentCursor(raw string) (*domaininteraction.CommentCursor, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	content, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		content, err = base64.StdEncoding.DecodeString(raw)
		if err != nil {
			return nil, domaininteraction.ErrInvalidCursor
		}
	}

	var payload commentCursorPayload
	if err := json.Unmarshal(content, &payload); err != nil {
		return nil, domaininteraction.ErrInvalidCursor
	}

	createdAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(payload.CreatedAt))
	if err != nil || createdAt.IsZero() || payload.CommentID <= 0 {
		return nil, domaininteraction.ErrInvalidCursor
	}

	return &domaininteraction.CommentCursor{
		CreatedAt: createdAt,
		CommentID: payload.CommentID,
	}, nil
}

// encodeCommentCursor 将评论分页游标编码为 URL 安全字符串。
func encodeCommentCursor(cursor *domaininteraction.CommentCursor) string {
	if cursor == nil || cursor.CommentID <= 0 || cursor.CreatedAt.IsZero() {
		return ""
	}

	content, err := json.Marshal(commentCursorPayload{
		CreatedAt: cursor.CreatedAt.UTC().Format(time.RFC3339Nano),
		CommentID: cursor.CommentID,
	})
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(content)
}

// ready 判断服务及其互动仓储依赖是否已初始化。
func (s *Service) ready() bool {
	return s != nil && s.repo != nil
}

// isExpectedDomainError 判断错误是否应直接返回给应用调用方。
func isExpectedDomainError(err error) bool {
	return errors.Is(err, domaininteraction.ErrInvalidUserID) ||
		errors.Is(err, domaininteraction.ErrInvalidVideoID) ||
		errors.Is(err, domaininteraction.ErrInvalidCommentID) ||
		errors.Is(err, domaininteraction.ErrInvalidActionType) ||
		errors.Is(err, domaininteraction.ErrInvalidLimit) ||
		errors.Is(err, domaininteraction.ErrInvalidCursor) ||
		errors.Is(err, domaininteraction.ErrIdempotencyKeyTooLong) ||
		errors.Is(err, domaininteraction.ErrEmptyCommentContent) ||
		errors.Is(err, domaininteraction.ErrCommentContentTooLong) ||
		errors.Is(err, domaininteraction.ErrVideoNotFound) ||
		errors.Is(err, domaininteraction.ErrCommentNotFound) ||
		errors.Is(err, domaininteraction.ErrCommentPermissionDenied)
}
