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

// actionCounterRepository 表示能够返回视频完整互动统计的可选仓储能力。
type actionCounterRepository interface {
	GetActionCounts(ctx context.Context, videoID int64) (likeCount int, favoriteCount int, err error)
}

// Service 编排点赞、收藏和评论相关的应用用例。
type Service struct {
	repo domaininteraction.Repository
}

// commentCursorPayload 表示评论分页游标的编码载荷。
type commentCursorPayload struct {
	CreatedAt string `json:"created_at"`
	CommentID int64  `json:"comment_id"`
}

// NewService 创建使用指定互动仓储的应用服务。
func NewService(repo domaininteraction.Repository) *Service {
	return &Service{repo: repo}
}

// ToggleLike 切换用户对视频的点赞状态并返回最新点赞统计。
func (s *Service) ToggleLike(ctx context.Context, userID int64, videoID int64, idempotencyKey string) (*ToggleActionResult, error) {
	return s.toggleAction(ctx, userID, videoID, domaininteraction.ActionTypeLike, idempotencyKey)
}

// ToggleFavorite 切换用户对视频的收藏状态并返回最新收藏统计。
func (s *Service) ToggleFavorite(ctx context.Context, userID int64, videoID int64, idempotencyKey string) (*ToggleActionResult, error) {
	return s.toggleAction(ctx, userID, videoID, domaininteraction.ActionTypeFavorite, idempotencyKey)
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

	return &DeleteCommentResult{
		CommentID:    comment.ID,
		Status:       comment.Status,
		CommentCount: count,
	}, nil
}

// toggleAction 执行指定类型的互动切换，并将仓储结果转换为应用结果。
func (s *Service) toggleAction(ctx context.Context, userID int64, videoID int64, actionType string, idempotencyKey string) (*ToggleActionResult, error) {
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

	action, count, err := s.repo.ToggleAction(ctx, userID, videoID, actionType, idempotencyKey)
	if err != nil {
		if isExpectedDomainError(err) {
			return nil, err
		}
		return nil, ErrUpdateInteractionFailed
	}
	if action == nil {
		return nil, ErrUpdateInteractionFailed
	}
	likeCount, favoriteCount := 0, 0
	if counterRepository, ok := s.repo.(actionCounterRepository); ok {
		likeCount, favoriteCount, err = counterRepository.GetActionCounts(ctx, videoID)
		if err != nil {
			if isExpectedDomainError(err) {
				return nil, err
			}
			return nil, ErrLoadInteractionFailed
		}
	} else {
		switch action.ActionType {
		case domaininteraction.ActionTypeLike:
			likeCount = count
		case domaininteraction.ActionTypeFavorite:
			favoriteCount = count
		}
	}

	result := &ToggleActionResult{
		VideoID:       action.VideoID,
		ActionType:    action.ActionType,
		Active:        action.Active(),
		LikeCount:     likeCount,
		FavoriteCount: favoriteCount,
	}
	if action.ActionType != domaininteraction.ActionTypeLike && action.ActionType != domaininteraction.ActionTypeFavorite {
		return nil, ErrUpdateInteractionFailed
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
