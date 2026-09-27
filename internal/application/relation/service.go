package relation

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	domainmessage "github.com/inumoroha/video-feed/internal/domain/message"
	"strings"
	"time"

	domainrelation "github.com/inumoroha/video-feed/internal/domain/relation"
)

const defaultListLimit = 20

// Service 编排关系用例、关注流回填和成功关注后的尽力通知。
type Service struct {
	messageWriter MessageWriter
	repo          domainrelation.Repository
	backfiller    FollowFeedBackfiller
}

// listCursorPayload 保存关系分页游标的编码字段。
type listCursorPayload struct {
	FollowedAt string `json:"followed_at"`
	UserID     int64  `json:"user_id"`
}

// NewService 创建使用指定关系仓储的应用服务。
func NewService(repo domainrelation.Repository, options ...Option) *Service {
	service := &Service{repo: repo}
	for _, option := range options {
		option(service)
	}
	return service
}

// Follow 设置当前用户关注目标用户。
func (s *Service) Follow(ctx context.Context, userID int64, targetUserID int64, idempotencyKey string) (*FollowResult, error) {
	return s.setFollow(ctx, userID, targetUserID, true, idempotencyKey)
}

// Unfollow 设置当前用户取消关注目标用户。
func (s *Service) Unfollow(ctx context.Context, userID int64, targetUserID int64, idempotencyKey string) (*FollowResult, error) {
	return s.setFollow(ctx, userID, targetUserID, false, idempotencyKey)
}

// ListFollowing 查询当前用户的关注列表。
func (s *Service) ListFollowing(ctx context.Context, userID int64, cursor string, limit int) (*ListResult, error) {
	if !s.ready() {
		return nil, ErrRelationServiceNotInitialized
	}
	if userID <= 0 {
		return nil, domainrelation.ErrInvalidUserID
	}
	if limit < 0 {
		return nil, domainrelation.ErrInvalidLimit
	}
	parsedCursor, err := parseListCursor(cursor)
	if err != nil {
		return nil, err
	}
	limit = normalizeLimit(limit)

	items, err := s.repo.ListFollowing(ctx, userID, parsedCursor, limit+1)
	if err != nil {
		if isExpectedDomainError(err) {
			return nil, err
		}
		return nil, ErrLoadRelationFailed
	}
	return listResult(items, limit), nil
}

// ListFollowers 查询当前用户的粉丝列表。
func (s *Service) ListFollowers(ctx context.Context, userID int64, cursor string, limit int) (*ListResult, error) {
	if !s.ready() {
		return nil, ErrRelationServiceNotInitialized
	}
	if userID <= 0 {
		return nil, domainrelation.ErrInvalidUserID
	}
	if limit < 0 {
		return nil, domainrelation.ErrInvalidLimit
	}
	parsedCursor, err := parseListCursor(cursor)
	if err != nil {
		return nil, err
	}
	limit = normalizeLimit(limit)

	items, err := s.repo.ListFollowers(ctx, userID, parsedCursor, limit+1)
	if err != nil {
		if isExpectedDomainError(err) {
			return nil, err
		}
		return nil, ErrLoadRelationFailed
	}
	return listResult(items, limit), nil
}

// setFollow 保存关系并同步回填；回填失败返回错误但不撤销关系，成功后尽力通知且忽略通知失败。
func (s *Service) setFollow(ctx context.Context, userID int64, targetUserID int64, active bool, idempotencyKey string) (*FollowResult, error) {
	if !s.ready() {
		return nil, ErrRelationServiceNotInitialized
	}

	followRequest, err := domainrelation.NewFollow(userID, targetUserID, idempotencyKey)
	if err != nil {
		return nil, err
	}

	follow, userStat, targetStat, err := s.repo.SetFollow(ctx, userID, targetUserID, active, followRequest.IdempotencyKey)
	if err != nil {
		if isExpectedDomainError(err) {
			return nil, err
		}
		return nil, ErrUpdateRelationFailed
	}
	if follow == nil || userStat == nil || targetStat == nil {
		return nil, ErrUpdateRelationFailed
	}

	if active && follow.Active() {
		if err := s.backfillFollowFeed(ctx, userID, targetUserID); err != nil {
			return nil, ErrBackfillFollowFeedFailed
		}
		s.notifyFollow(ctx, userID, targetUserID)
	}
	return &FollowResult{
		UserID:         follow.UserID,
		TargetUserID:   follow.TargetUserID,
		Status:         follow.Status,
		Following:      follow.Active(),
		FollowingCount: userStat.FollowingCount,
		FollowerCount:  targetStat.FollowerCount,
	}, nil
}

// listResult 根据多查一条的结果生成分页信息和下一页游标。
func listResult(items []*domainrelation.UserItem, limit int) *ListResult {
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}

	nextCursor := ""
	if len(items) > 0 {
		last := items[len(items)-1]
		nextCursor = encodeListCursor(&domainrelation.ListCursor{
			FollowedAt: last.FollowedAt,
			UserID:     last.UserID,
		})
	}

	return &ListResult{
		Items:      items,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}
}

// normalizeLimit 将零值分页数量转换为默认值，并限制最大返回数量。
func normalizeLimit(limit int) int {
	if limit == 0 {
		return defaultListLimit
	}
	if limit > domainrelation.MaxLimit {
		return domainrelation.MaxLimit
	}
	return limit
}

// parseListCursor 解码并校验关系分页游标。
func parseListCursor(raw string) (*domainrelation.ListCursor, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	content, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		content, err = base64.StdEncoding.DecodeString(raw)
		if err != nil {
			return nil, domainrelation.ErrInvalidCursor
		}
	}

	var payload listCursorPayload
	if err := json.Unmarshal(content, &payload); err != nil {
		return nil, domainrelation.ErrInvalidCursor
	}

	followedAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(payload.FollowedAt))
	if err != nil || followedAt.IsZero() || payload.UserID <= 0 {
		return nil, domainrelation.ErrInvalidCursor
	}

	return &domainrelation.ListCursor{
		FollowedAt: followedAt,
		UserID:     payload.UserID,
	}, nil
}

// encodeListCursor 将关系分页游标编码为 URL 安全字符串。
func encodeListCursor(cursor *domainrelation.ListCursor) string {
	if cursor == nil || cursor.UserID <= 0 || cursor.FollowedAt.IsZero() {
		return ""
	}

	content, err := json.Marshal(listCursorPayload{
		FollowedAt: cursor.FollowedAt.UTC().Format(time.RFC3339Nano),
		UserID:     cursor.UserID,
	})
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(content)
}

// ready 判断关系服务及其仓储依赖是否已初始化。
func (s *Service) ready() bool {
	return s != nil && s.repo != nil
}

// isExpectedDomainError 判断仓储错误是否属于可直接返回的关系领域错误。
func isExpectedDomainError(err error) bool {
	return errors.Is(err, domainrelation.ErrInvalidUserID) ||
		errors.Is(err, domainrelation.ErrInvalidTargetUserID) ||
		errors.Is(err, domainrelation.ErrFollowSelfForbidden) ||
		errors.Is(err, domainrelation.ErrInvalidLimit) ||
		errors.Is(err, domainrelation.ErrInvalidCursor) ||
		errors.Is(err, domainrelation.ErrIdempotencyKeyTooLong) ||
		errors.Is(err, domainrelation.ErrUserNotFound) ||
		errors.Is(err, domainrelation.ErrTargetUserNotFound) ||
		errors.Is(err, domainrelation.ErrFollowNotFound)
}

// MessageWriter 写入关注触发的站内消息。
type MessageWriter interface {
	CreateFromEvent(ctx context.Context, userID int64, messageType string, title string, content string, eventID string, idempotencyKey string) (any, error)
}

// ActorMessageWriter 可在关注消息里携带触发用户资料。
type ActorMessageWriter interface {
	CreateFromActorEvent(ctx context.Context, userID int64, messageType string, title string, content string, eventID string, idempotencyKey string, actorID int64, actorNickname string, actorAvatarURL string) (any, error)
}

// WithMessageWriter 为关注成功后的通知写入启用消息中心。
func WithMessageWriter(writer MessageWriter) Option {
	return func(s *Service) {
		s.messageWriter = writer
	}
}

// notifyFollow 尽力写入关注通知，使用固定事件标识保证消息幂等，不传播写入错误。
func (s *Service) notifyFollow(ctx context.Context, userID int64, targetUserID int64) {
	if s.messageWriter == nil {
		return
	}
	eventID := fmt.Sprintf("relation:follow:%d:%d", targetUserID, userID)
	actor, _ := s.repo.GetUserProfile(ctx, userID)
	if writer, ok := s.messageWriter.(ActorMessageWriter); ok {
		actorNickname := ""
		actorAvatarURL := ""
		if actor != nil {
			actorNickname = actor.Nickname
			actorAvatarURL = actor.AvatarURL
		}
		_, _ = writer.CreateFromActorEvent(
			ctx,
			targetUserID,
			domainmessage.TypeFollow,
			"新增关注",
			"关注了你",
			eventID,
			eventID,
			userID,
			actorNickname,
			actorAvatarURL,
		)
		return
	}
	actorName := fmt.Sprintf("用户 %d", userID)
	if actor != nil && actor.Nickname != "" {
		actorName = actor.Nickname
	}
	_, _ = s.messageWriter.CreateFromEvent(
		ctx,
		targetUserID,
		domainmessage.TypeFollow,
		"新增关注",
		fmt.Sprintf("%s 关注了你", actorName),
		eventID,
		eventID,
	)
}
