package video

import (
	"strings"
	"time"
	"unicode/utf8"
)

const (
	StatusDraft     = 1
	StatusPublished = 2
	StatusOffline   = 3
	StatusDeleted   = 4

	MaxTitleLength          = 128
	MaxIdempotencyKeyLength = 128
)

type Video struct {
	ID             int64
	AuthorID       int64
	Title          string
	MediaURL       string
	CoverURL       string
	Status         int
	LikeCount      int
	CommentCount   int
	FavoriteCount  int
	PublishedAt    *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
	IdempotencyKey string
}

// NewPublished 校验发布参数并创建已发布状态的视频。
func NewPublished(authorID int64, title, mediaURL, coverURL, idempotencyKey string) (*Video, error) {
	if authorID <= 0 {
		return nil, ErrInvalidAuthorID
	}

	title = strings.TrimSpace(title)
	mediaURL = strings.TrimSpace(mediaURL)
	coverURL = strings.TrimSpace(coverURL)
	idempotencyKey = strings.TrimSpace(idempotencyKey)

	if title == "" {
		return nil, ErrEmptyTitle
	}
	if utf8.RuneCountInString(title) > MaxTitleLength {
		return nil, ErrTitleTooLong
	}
	if mediaURL == "" {
		return nil, ErrEmptyMediaURL
	}
	if coverURL == "" {
		return nil, ErrEmptyCoverURL
	}
	if len(idempotencyKey) > MaxIdempotencyKeyLength {
		return nil, ErrIdempotencyKeyTooLong
	}

	now := time.Now()
	return &Video{
		AuthorID:       authorID,
		Title:          title,
		MediaURL:       mediaURL,
		CoverURL:       coverURL,
		Status:         StatusPublished,
		PublishedAt:    &now,
		IdempotencyKey: idempotencyKey,
	}, nil
}

// RestoreVideo 根据持久化数据恢复视频领域对象。
func RestoreVideo(
	id int64,
	authorID int64,
	title string,
	mediaURL string,
	coverURL string,
	status int,
	likeCount int,
	commentCount int,
	favoriteCount int,
	publishedAt *time.Time,
	createdAt time.Time,
	updatedAt time.Time,
	idempotencyKey string,
) *Video {
	title = strings.TrimSpace(title)
	mediaURL = strings.TrimSpace(mediaURL)
	coverURL = strings.TrimSpace(coverURL)
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if status == 0 {
		status = StatusPublished
	}

	return &Video{
		ID:             id,
		AuthorID:       authorID,
		Title:          title,
		MediaURL:       mediaURL,
		CoverURL:       coverURL,
		Status:         status,
		LikeCount:      likeCount,
		CommentCount:   commentCount,
		FavoriteCount:  favoriteCount,
		PublishedAt:    publishedAt,
		CreatedAt:      createdAt,
		UpdatedAt:      updatedAt,
		IdempotencyKey: idempotencyKey,
	}
}

// DeleteBy 校验作者身份后将视频标记为已删除。
func (v *Video) DeleteBy(authorID int64) error {
	if authorID <= 0 {
		return ErrInvalidAuthorID
	}
	if v.AuthorID != authorID {
		return ErrVideoPermissionDenied
	}
	if v.Status == StatusDeleted {
		return nil
	}
	v.Status = StatusDeleted
	return nil
}
