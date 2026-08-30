package feed

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	domainfeed "github.com/inumoroha/video-feed/internal/domain/feed"
)

const defaultFeedLimit = 20

type Service struct {
	repo domainfeed.Repository
}

// NewService 创建使用指定信息流仓储的应用服务。
func NewService(repo domainfeed.Repository) *Service {
	return &Service{repo: repo}
}

// GetTimeFeed 查询时间线视频流，处理游标分页并生成下一页游标。
func (s *Service) GetTimeFeed(ctx context.Context, cursor string, limit int) (*TimeFeedResult, error) {
	if !s.ready() {
		return nil, ErrFeedServiceNotInitialized
	}
	if limit < 0 {
		return nil, domainfeed.ErrInvalidLimit
	}

	parsedCursor, err := parseTimeCursor(cursor)
	if err != nil {
		return nil, err
	}
	limit = normalizeLimit(limit)

	items, err := s.repo.ListTimeFeed(ctx, parsedCursor, limit+1)
	if err != nil {
		return nil, ErrLoadFeedFailed
	}

	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}

	nextCursor := ""
	if hasMore && len(items) > 0 {
		nextCursor = encodeTimeCursor(&domainfeed.TimeCursor{
			PublishedAt: items[len(items)-1].PublishedAt,
			VideoID:     items[len(items)-1].VideoID,
		})
	}

	return &TimeFeedResult{
		Items:      items,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

// RefreshTimeFeed 从时间线起点查询视频流，适用于刷新首页数据。
func (s *Service) RefreshTimeFeed(ctx context.Context, limit int) (*TimeFeedResult, error) {
	return s.GetTimeFeed(ctx, "", limit)
}

// ReportViewEvent 创建观看事件或返回幂等键对应的已有事件。
func (s *Service) ReportViewEvent(ctx context.Context, userID *int64, visitorID string, videoID int64, eventType string, watchMS int, idempotencyKey string) (*ViewEventResult, error) {
	if !s.ready() {
		return nil, ErrFeedServiceNotInitialized
	}
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if len(idempotencyKey) > domainfeed.MaxIdempotencyKeyLength {
		return nil, domainfeed.ErrIdempotencyKeyTooLong
	}

	if idempotencyKey != "" {
		existing, err := s.repo.FindViewEventByIdempotencyKey(ctx, idempotencyKey)
		if err == nil {
			if existing == nil {
				return nil, ErrLoadViewEventFailed
			}
			return &ViewEventResult{Event: existing, Created: false}, nil
		}
		if !errors.Is(err, domainfeed.ErrViewEventNotFound) {
			return nil, ErrLoadViewEventFailed
		}
	}

	event, err := domainfeed.NewViewEvent(userID, visitorID, videoID, eventType, watchMS, idempotencyKey)
	if err != nil {
		return nil, err
	}

	if err := s.repo.SaveViewEvent(ctx, event); err != nil {
		if idempotencyKey != "" && errors.Is(err, domainfeed.ErrDuplicateIdempotencyKey) {
			existing, loadErr := s.repo.FindViewEventByIdempotencyKey(ctx, idempotencyKey)
			if loadErr == nil {
				if existing == nil {
					return nil, ErrLoadViewEventFailed
				}
				return &ViewEventResult{Event: existing, Created: false}, nil
			}
			return nil, ErrLoadViewEventFailed
		}
		return nil, ErrSaveViewEventFailed
	}

	return &ViewEventResult{Event: event, Created: true}, nil
}

// ready 判断服务及其信息流仓储依赖是否已初始化。
func (s *Service) ready() bool {
	return s != nil && s.repo != nil
}

// normalizeLimit 将零值分页数量转换为默认值，并限制最大返回数量。
func normalizeLimit(limit int) int {
	if limit <= 0 {
		return defaultFeedLimit
	}
	if limit > domainfeed.MaxLimit {
		return domainfeed.MaxLimit
	}
	return limit
}

// parseTimeCursor 解码并校验时间线分页游标。
func parseTimeCursor(raw string) (*domainfeed.TimeCursor, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	content, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		content, err = base64.StdEncoding.DecodeString(raw)
		if err != nil {
			return nil, domainfeed.ErrInvalidCursor
		}
	}

	var payload timeCursorPayload
	if err := json.Unmarshal(content, &payload); err != nil {
		return nil, domainfeed.ErrInvalidCursor
	}

	publishedAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(payload.PublishedAt))
	if err != nil {
		return nil, domainfeed.ErrInvalidCursor
	}

	cursor := &domainfeed.TimeCursor{
		PublishedAt: publishedAt,
		VideoID:     payload.VideoID,
	}
	if err := cursor.Validate(); err != nil {
		return nil, err
	}
	return cursor, nil
}

// encodeTimeCursor 将时间线分页游标编码为 URL 安全字符串。
func encodeTimeCursor(cursor *domainfeed.TimeCursor) string {
	if cursor == nil {
		return ""
	}
	if err := cursor.Validate(); err != nil {
		return ""
	}

	content, err := json.Marshal(timeCursorPayload{
		PublishedAt: cursor.PublishedAt.UTC().Format(time.RFC3339Nano),
		VideoID:     cursor.VideoID,
	})
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(content)
}
