package exposure

import (
	"encoding/json"
	domainexposure "github.com/inumoroha/video-feed/internal/domain/exposure"
	"testing"
	"time"
)

// TestRecordedEventPayload 验证画像事件全部字段、唯一编号、UTC 时间以及可选曝光次数。
func TestRecordedEventPayload(t *testing.T) {
	if NewViewEventRecordedEvent(nil, nil) != nil {
		t.Fatal("空行为生成消息")
	}
	at := time.Date(2026, 5, 17, 10, 0, 0, 0, time.FixedZone("CST", 8*3600))
	event := domainexposure.RestoreViewEvent(3, 42, 1001, "hot", "req", "exposed", 123, true, at)
	message := NewViewEventRecordedEvent(event, &domainexposure.Exposure{ExposureCount: 2})
	if message.EventID == "" || message.ViewEventID != 3 || message.UserID != 42 || message.VideoID != 1001 || message.Scene != "hot" || message.RequestID != "req" || message.EventType != "exposed" || message.WatchMs != 123 || !message.Completed || message.ExposureCount != 2 || !message.RecordedAt.Equal(at) || message.RecordedAt.Location() != time.UTC || message.OccurredAt.Location() != time.UTC {
		t.Fatalf("%+v", message)
	}
	next := NewViewEventRecordedEvent(event, nil)
	if next.EventID == message.EventID || next.ExposureCount != 0 {
		t.Fatal("事件编号重复或错误携带聚合")
	}
	content, err := json.Marshal(next)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err = json.Unmarshal(content, &fields); err != nil {
		t.Fatal(err)
	}
	if _, ok := fields["exposure_count"]; ok {
		t.Fatal("未省略空聚合计数")
	}
}
