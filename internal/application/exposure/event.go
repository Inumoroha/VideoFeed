package exposure

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	domainexposure "github.com/inumoroha/video-feed/internal/domain/exposure"
	"time"
)

// ViewEventRecordedEvent 是观看行为已落库事件，供用户画像和推荐画像 worker 消费。
type ViewEventRecordedEvent struct {
	EventID       string    `json:"event_id"`
	ViewEventID   int64     `json:"view_event_id"`
	UserID        int64     `json:"user_id"`
	VideoID       int64     `json:"video_id"`
	Scene         string    `json:"scene"`
	RequestID     string    `json:"request_id,omitempty"`
	EventType     string    `json:"event_type"`
	WatchMs       int       `json:"watch_ms"`
	Completed     bool      `json:"completed"`
	RecordedAt    time.Time `json:"recorded_at"`
	OccurredAt    time.Time `json:"occurred_at"`
	ExposureCount int       `json:"exposure_count,omitempty"`
}

// NewViewEventRecordedEvent 根据已保存行为生成画像事件；空行为返回空值，并保留曝光次数和 UTC 时间。
func NewViewEventRecordedEvent(event *domainexposure.ViewEvent, exposure *domainexposure.Exposure) *ViewEventRecordedEvent {
	if event == nil {
		return nil
	}
	message := &ViewEventRecordedEvent{
		EventID:     newEventID(),
		ViewEventID: event.ID,
		UserID:      event.UserID,
		VideoID:     event.VideoID,
		Scene:       event.Scene,
		RequestID:   event.RequestID,
		EventType:   event.EventType,
		WatchMs:     event.WatchMs,
		Completed:   event.Completed,
		RecordedAt:  event.CreatedAt.UTC(),
		OccurredAt:  time.Now().UTC(),
	}
	if exposure != nil {
		message.ExposureCount = exposure.ExposureCount
	}
	return message
}

// newEventID 优先生成随机事件编号，随机源失败时回退到纳秒时间戳。
func newEventID() string {
	content := make([]byte, 12)
	if _, err := rand.Read(content); err == nil {
		return hex.EncodeToString(content)
	}
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
