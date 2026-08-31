package feed

import (
	"context"
	"time"
)

type Repository interface {
	ListTimeFeed(ctx context.Context, cursor *TimeCursor, limit int) ([]*FeedItem, error)
	SaveTimeCursor(ctx context.Context, token string, cursor *TimeCursor, expiresAt time.Time) error
	FindTimeCursor(ctx context.Context, token string, now time.Time) (*TimeCursor, error)
	SaveViewEvent(ctx context.Context, event *ViewEvent) error
	FindViewEventByIdempotencyKey(ctx context.Context, key string) (*ViewEvent, error)
}
