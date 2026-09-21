package playback

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// TestQoSValidation 验证所有质量指标边界、空键及内部匿名用户，保留可空首帧语义。
func TestQoSValidation(t *testing.T) {
	negative := -1
	cases := []struct {
		user, video    int64
		frame          *int
		stutter, watch int
		key            string
		want           error
	}{
		{-1, 1, nil, 0, 0, "", ErrInvalidUserID}, {0, 0, nil, 0, 0, "", ErrInvalidVideoID},
		{0, 1, &negative, 0, 0, "", ErrInvalidFirstFrameMs}, {0, 1, nil, -1, 0, "", ErrInvalidStutterCount},
		{0, 1, nil, 0, -1, "", ErrInvalidWatchMs}, {0, 1, nil, 0, 0, strings.Repeat("x", 129), ErrIdempotencyKeyTooLong},
		{0, 1, nil, 0, 0, "  key  ", nil}, {1, 1, nil, 0, 0, "", nil},
	}
	for _, c := range cases {
		report, err := NewQoSReport(c.user, c.video, c.frame, c.stutter, c.watch, c.key)
		if !errors.Is(err, c.want) {
			t.Fatalf("%+v: %v", c, err)
		}
		if err == nil && (report.IdempotencyKey != strings.TrimSpace(c.key) || report.FirstFrameMs != c.frame) {
			t.Fatalf("%+v", report)
		}
	}
}

// TestNormalizationAndRestore 验证所有网络规范化分支、默认配置和持久化恢复字段。
func TestNormalizationAndRestore(t *testing.T) {
	for raw, want := range map[string]string{"": "DEFAULT", " wi-fi ": "WiFi", "wifi": "WiFi", "3g": "3G", "4g": "4G", "5g": "5G", "default": "DEFAULT", " ethernet ": "ETHERNET"} {
		if got := NormalizeNetworkType(raw); got != want {
			t.Fatalf("%q: %q", raw, got)
		}
	}
	if NormalizePlatform(" ") != "Web" || NormalizePlatform(" iOS ") != "iOS" {
		t.Fatal("平台规范化失败")
	}
	cfg := DefaultConfig("", "")
	if cfg.PreloadCount != 3 || cfg.BufferMs != 1200 || cfg.UpdatedAt.IsZero() {
		t.Fatal(cfg)
	}
	now := time.Now()
	cfg = RestoreConfig(2, " Web ", "wifi", 4, 900, now)
	if cfg.ID != 2 || cfg.NetworkType != "WiFi" || cfg.PreloadCount != 4 || cfg.BufferMs != 900 || cfg.UpdatedAt != now {
		t.Fatal(cfg)
	}
	video := RestorePreloadVideo(3, " media ", " cover ")
	if video.VideoID != 3 || video.MediaURL != "media" || video.CoverURL != "cover" {
		t.Fatal(video)
	}
	frame := 10
	report := RestoreQoSReport(1, 2, 3, &frame, 4, 5, " key ", now)
	if report.ID != 1 || report.UserID != 2 || report.VideoID != 3 || *report.FirstFrameMs != 10 || report.StutterCount != 4 || report.WatchMs != 5 || report.IdempotencyKey != "key" || report.CreatedAt != now {
		t.Fatal(report)
	}
}
