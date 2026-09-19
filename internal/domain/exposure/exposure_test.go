package exposure

import (
	"errors"
	"strings"
	"testing"
)

// TestViewEventValidation 验证所有领域参数边界、按字节计算的长度限制和事件规范化。
func TestViewEventValidation(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		user, video          int64
		scene, request, kind string
		watch                int
		want                 error
	}{
		{"user", 0, 1, "hot", "", "play", 0, ErrInvalidUserID},
		{"video", 1, 0, "hot", "", "play", 0, ErrInvalidVideoID},
		{"scene", 1, 1, "  ", "", "play", 0, ErrEmptyScene},
		{"scene_length", 1, 1, strings.Repeat("a", 33), "", "play", 0, ErrSceneTooLong},
		{"scene_bytes", 1, 1, strings.Repeat("中", 11), "", "play", 0, ErrSceneTooLong},
		{"request", 1, 1, "hot", strings.Repeat("a", 65), "play", 0, ErrRequestIDTooLong},
		{"watch", 1, 1, "hot", "", "play", -1, ErrWatchMsNegative},
		{"kind", 1, 1, "hot", "", "progress", 0, ErrInvalidEventType},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewViewEvent(tc.user, tc.video, tc.scene, tc.request, tc.kind, tc.watch, false); !errors.Is(err, tc.want) {
				t.Fatalf("err=%v", err)
			}
		})
	}
	for _, kind := range []string{EventTypeExposed, EventTypePlay, EventTypeComplete, EventTypeSkip} {
		event, err := NewViewEvent(1, 2, " HOT ", " req ", " "+strings.ToUpper(kind)+" ", 0, false)
		if err != nil || event.Scene != "hot" || event.RequestID != "req" || event.EventType != kind || event.Completed != (kind == EventTypeComplete) || event.CountsAsExposure() != (kind == EventTypeExposed) {
			t.Fatalf("event=%+v err=%v", event, err)
		}
	}
	if _, err := NewViewEvent(1, 2, strings.Repeat("a", 32), strings.Repeat("b", 64), EventTypeSkip, 0, true); err != nil {
		t.Fatal(err)
	}
	var nilEvent *ViewEvent
	if nilEvent.CountsAsExposure() {
		t.Fatal("空行为被视为曝光")
	}
}
