package video

import (
	"errors"
	"strings"
	"testing"
)

// TestNewPublishedCreatesTrimmedPublishedVideo 验证创建视频会清理输入并设置已发布状态。
func TestNewPublishedCreatesTrimmedPublishedVideo(t *testing.T) {
	video, err := NewPublished(1, " title ", " description ", " media-url ", " cover-url ", " key ")
	if err != nil {
		t.Fatalf("NewPublished() error = %v", err)
	}

	if video.AuthorID != 1 || video.Title != "title" || video.Description != "description" || video.MediaURL != "media-url" || video.CoverURL != "cover-url" || video.IdempotencyKey != "key" {
		t.Fatalf("NewPublished() video = %+v, want trimmed input values", video)
	}
	if video.Status != StatusPublished || video.PublishedAt == nil {
		t.Fatalf("NewPublished() status = %d, publishedAt = %v, want published video", video.Status, video.PublishedAt)
	}
}

// TestNewPublishedValidatesInput 验证创建视频会拒绝无效的发布参数。
func TestNewPublishedValidatesInput(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want error
	}{
		{name: "invalid author", args: []string{"title", "description", "media-url", "cover-url", "key"}, want: ErrInvalidAuthorID},
		{name: "empty title", args: []string{" ", "description", "media-url", "cover-url", "key"}, want: ErrEmptyTitle},
		{name: "empty media URL", args: []string{"title", "description", " ", "cover-url", "key"}, want: ErrEmptyMediaURL},
		{name: "empty cover URL", args: []string{"title", "description", "media-url", " ", "key"}, want: ErrEmptyCoverURL},
		{name: "idempotency key too long", args: []string{"title", "description", "media-url", "cover-url", strings.Repeat("k", MaxIdempotencyKeyLength+1)}, want: ErrIdempotencyKeyTooLong},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			authorID := int64(1)
			if test.name == "invalid author" {
				authorID = 0
			}

			_, err := NewPublished(authorID, test.args[0], test.args[1], test.args[2], test.args[3], test.args[4])
			if !errors.Is(err, test.want) {
				t.Fatalf("NewPublished() error = %v, want %v", err, test.want)
			}
		})
	}
}

// TestNewPublishedCountsTitleByCharacters 验证标题长度按字符而非 UTF-8 字节数计算。
func TestNewPublishedCountsTitleByCharacters(t *testing.T) {
	validTitle := strings.Repeat("中", MaxTitleLength)
	if _, err := NewPublished(1, validTitle, "", "media-url", "cover-url", "key"); err != nil {
		t.Fatalf("NewPublished() error = %v, want 128-character title accepted", err)
	}

	tooLongTitle := strings.Repeat("中", MaxTitleLength+1)
	if _, err := NewPublished(1, tooLongTitle, "", "media-url", "cover-url", "key"); !errors.Is(err, ErrTitleTooLong) {
		t.Fatalf("NewPublished() error = %v, want %v", err, ErrTitleTooLong)
	}
}

// TestNewPublishedCountsDescriptionByCharacters 验证描述长度按字符而非 UTF-8 字节数计算。
func TestNewPublishedCountsDescriptionByCharacters(t *testing.T) {
	validDescription := strings.Repeat("中", MaxDescriptionLength)
	if _, err := NewPublished(1, "title", validDescription, "media-url", "cover-url", "key"); err != nil {
		t.Fatalf("NewPublished() error = %v, want 512-character description accepted", err)
	}

	tooLongDescription := strings.Repeat("中", MaxDescriptionLength+1)
	if _, err := NewPublished(1, "title", tooLongDescription, "media-url", "cover-url", "key"); !errors.Is(err, ErrDescriptionTooLong) {
		t.Fatalf("NewPublished() error = %v, want %v", err, ErrDescriptionTooLong)
	}
}

// TestDeleteByValidatesAuthorAndMarksDeleted 验证删除操作的作者校验和幂等状态变更。
func TestDeleteByValidatesAuthorAndMarksDeleted(t *testing.T) {
	video := &Video{AuthorID: 1, Status: StatusPublished}

	if err := video.DeleteBy(0); !errors.Is(err, ErrInvalidAuthorID) {
		t.Fatalf("DeleteBy() error = %v, want %v", err, ErrInvalidAuthorID)
	}
	if err := video.DeleteBy(2); !errors.Is(err, ErrVideoPermissionDenied) {
		t.Fatalf("DeleteBy() error = %v, want %v", err, ErrVideoPermissionDenied)
	}
	if video.Status != StatusPublished {
		t.Fatalf("DeleteBy() changed status to %d after denied request", video.Status)
	}
	if err := video.DeleteBy(1); err != nil {
		t.Fatalf("DeleteBy() error = %v", err)
	}
	if video.Status != StatusDeleted {
		t.Fatalf("DeleteBy() status = %d, want %d", video.Status, StatusDeleted)
	}
	if err := video.DeleteBy(1); err != nil {
		t.Fatalf("DeleteBy() on deleted video error = %v", err)
	}
}
