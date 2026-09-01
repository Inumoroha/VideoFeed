package feed

import (
	"testing"
	"time"
)

// TestRestoreFeedItemNormalizesValues 验证恢复信息流条目时会清理所有文本字段的首尾空白。
func TestRestoreFeedItemNormalizesValues(t *testing.T) {
	publishedAt := time.Date(2026, time.August, 30, 12, 0, 0, 0, time.UTC)
	item := RestoreFeedItem(
		7,
		42,
		" author ",
		" avatar ",
		" title ",
		" description ",
		" media ",
		" cover ",
		1,
		2,
		3,
		publishedAt,
	)

	if item.VideoID != 7 || item.AuthorID != 42 {
		t.Fatalf("RestoreFeedItem() identifiers = (%d, %d), want (7, 42)", item.VideoID, item.AuthorID)
	}
	if item.AuthorNickname != "author" || item.AuthorAvatarURL != "avatar" || item.Title != "title" || item.Description != "description" || item.MediaURL != "media" || item.CoverURL != "cover" {
		t.Fatalf("RestoreFeedItem() text fields = %+v, want trimmed values", item)
	}
	if item.LikeCount != 1 || item.CommentCount != 2 || item.FavoriteCount != 3 || !item.PublishedAt.Equal(publishedAt) {
		t.Fatalf("RestoreFeedItem() metadata = %+v, want preserved values", item)
	}
}

// TestTimelineCursorStoresPosition 验证时间线游标保存发布时间和视频 ID。
func TestTimelineCursorStoresPosition(t *testing.T) {
	publishedAt := time.Date(2026, time.August, 30, 12, 0, 0, 0, time.UTC)
	cursor := TimelineCursor{PublishedAt: publishedAt, VideoID: 7}

	if cursor.VideoID != 7 || !cursor.PublishedAt.Equal(publishedAt) {
		t.Fatalf("TimelineCursor = %+v, want stored position", cursor)
	}
}

// TestMaxLimitDefinesFeedPageCap 验证信息流单页数量上限为 100。
func TestMaxLimitDefinesFeedPageCap(t *testing.T) {
	if MaxLimit != 100 {
		t.Fatalf("MaxLimit = %d, want 100", MaxLimit)
	}
}
