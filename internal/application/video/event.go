package video

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	domainvideo "github.com/inumoroha/video-feed/internal/domain/video"
	"strings"
	"time"
)

// PublishedEvent 保存视频发布后的标识、内容和发生时间。
type PublishedEvent struct {
	EventID     string    `json:"event_id"`
	VideoID     int64     `json:"video_id"`
	AuthorID    int64     `json:"author_id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	MediaURL    string    `json:"media_url"`
	CoverURL    string    `json:"cover_url"`
	PublishedAt time.Time `json:"published_at"`
	OccurredAt  time.Time `json:"occurred_at"`
}

// NewPublishedEvent 从已具有发布时间的视频生成事件，清理文本并使用协调世界时。
func NewPublishedEvent(video *domainvideo.Video) *PublishedEvent {
	if video == nil || video.PublishedAt == nil {
		return nil
	}
	return &PublishedEvent{
		EventID:     newEventID(),
		VideoID:     video.ID,
		AuthorID:    video.AuthorID,
		Title:       strings.TrimSpace(video.Title),
		Description: strings.TrimSpace(video.Description),
		MediaURL:    strings.TrimSpace(video.MediaURL),
		CoverURL:    strings.TrimSpace(video.CoverURL),
		PublishedAt: video.PublishedAt.UTC(),
		OccurredAt:  time.Now().UTC(),
	}
}

// newEventID 生成随机事件编号，随机源失败时使用纳秒时间兜底。
func newEventID() string {
	content := make([]byte, 12)
	if _, err := rand.Read(content); err == nil {
		return hex.EncodeToString(content)
	}
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
