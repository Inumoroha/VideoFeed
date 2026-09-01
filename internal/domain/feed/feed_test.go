package feed

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// TestNewViewEventValidatesInput 验证观看事件的身份、视频、类型、时长和幂等键校验。
func TestNewViewEventValidatesInput(t *testing.T) {
	validUserID := int64(42)
	tests := []struct {
		name      string
		userID    *int64
		visitorID string
		videoID   int64
		eventType string
		watchMS   int
		key       string
		wantErr   error
		wantType  string
	}{
		{
			name:      "authenticated user",
			userID:    &validUserID,
			videoID:   7,
			eventType: "view",
			wantType:  EventTypeView,
		},
		{
			name:      "anonymous visitor",
			visitorID: " visitor-1 ",
			videoID:   7,
			eventType: EventTypeImpression,
			wantType:  EventTypeImpression,
		},
		{
			name:    "invalid user id",
			userID:  ptrToInt64(0),
			videoID: 7,
			wantErr: ErrInvalidUserID,
		},
		{
			name:    "missing actor",
			videoID: 7,
			wantErr: ErrMissingActor,
		},
		{
			name:      "visitor id too long",
			visitorID: strings.Repeat("v", MaxVisitorIDLength+1),
			videoID:   7,
			wantErr:   ErrVisitorIDTooLong,
		},
		{
			name:      "invalid video id",
			visitorID: "visitor-1",
			wantErr:   ErrInvalidVideoID,
		},
		{
			name:      "empty event type",
			visitorID: "visitor-1",
			videoID:   7,
			wantErr:   ErrEmptyEventType,
		},
		{
			name:      "unsupported event type",
			visitorID: "visitor-1",
			videoID:   7,
			eventType: "share",
			wantErr:   ErrInvalidEventType,
		},
		{
			name:      "negative watch duration",
			visitorID: "visitor-1",
			videoID:   7,
			eventType: EventTypeView,
			watchMS:   -1,
			wantErr:   ErrInvalidWatchMS,
		},
		{
			name:      "idempotency key too long",
			visitorID: "visitor-1",
			videoID:   7,
			eventType: EventTypeView,
			key:       strings.Repeat("k", MaxIdempotencyKeyLength+1),
			wantErr:   ErrIdempotencyKeyTooLong,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			event, err := NewViewEvent(test.userID, test.visitorID, test.videoID, test.eventType, test.watchMS, test.key)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("NewViewEvent() error = %v, want %v", err, test.wantErr)
			}
			if test.wantErr != nil {
				if event != nil {
					t.Fatalf("NewViewEvent() event = %+v, want nil", event)
				}
				return
			}
			if event == nil {
				t.Fatal("NewViewEvent() event = nil, want event")
			}
			if event.EventType != test.wantType {
				t.Fatalf("NewViewEvent() event type = %q, want %q", event.EventType, test.wantType)
			}
			if test.visitorID != "" && event.VisitorID != strings.TrimSpace(test.visitorID) {
				t.Fatalf("NewViewEvent() visitor id = %q, want trimmed value", event.VisitorID)
			}
		})
	}
}

// TestTimeCursorValidate 验证首屏空游标和后续分页游标的合法性。
func TestTimeCursorValidate(t *testing.T) {
	validCursor := &TimeCursor{PublishedAt: time.Date(2026, time.August, 30, 12, 0, 0, 0, time.UTC), VideoID: 7}
	tests := []struct {
		name    string
		cursor  *TimeCursor
		wantErr error
	}{
		{name: "first page", cursor: nil},
		{name: "valid cursor", cursor: validCursor},
		{name: "missing timestamp", cursor: &TimeCursor{VideoID: 7}, wantErr: ErrInvalidCursor},
		{name: "invalid video id", cursor: &TimeCursor{PublishedAt: validCursor.PublishedAt}, wantErr: ErrInvalidCursor},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.cursor.Validate(); !errors.Is(err, test.wantErr) {
				t.Fatalf("TimeCursor.Validate() error = %v, want %v", err, test.wantErr)
			}
		})
	}
}

// TestRestoreViewEventNormalizesValues 验证恢复观看事件时会规范化字符串字段。
func TestRestoreViewEventNormalizesValues(t *testing.T) {
	event := RestoreViewEvent(1, nil, " visitor-1 ", 7, " view ", 1200, " key-1 ", time.Time{})
	if event.VisitorID != "visitor-1" || event.EventType != EventTypeView || event.IdempotencyKey != "key-1" {
		t.Fatalf("RestoreViewEvent() = %+v, want normalized values", event)
	}
}

// TestRestoreFeedItemNormalizesValues 验证恢复信息流条目时会去除字符串首尾空白。
func TestRestoreFeedItemNormalizesValues(t *testing.T) {
	item := RestoreFeedItem(7, 42, " title ", " description ", " media ", " cover ", 1, 2, 3, time.Time{})
	if item.Title != "title" || item.Description != "description" || item.MediaURL != "media" || item.CoverURL != "cover" {
		t.Fatalf("RestoreFeedItem() = %+v, want normalized values", item)
	}
}

// ptrToInt64 返回指向指定整数的指针，用于构造测试输入。
func ptrToInt64(value int64) *int64 {
	return &value
}
