package feed

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	domainvideo "github.com/inumoroha/video-feed/internal/domain/video"
)

// TestBatchGetFeedCardsUsesUsersTable 验证卡片回源关联本项目真实用户表，并保留作者与视频展示字段。
func TestBatchGetFeedCardsUsesUsersTable(t *testing.T) {
	repo, mock := mockRepo(t)
	query := `SELECT v.id AS video_id, v.author_id, a.nickname AS author_nickname, a.avatar_url AS author_avatar_url, v.title, v.description, v.media_url, v.cover_url FROM videos AS v LEFT JOIN users AS a ON a.id = v.author_id WHERE v.id IN ($1) AND v.status = $2 AND v.published_at IS NOT NULL`
	mock.ExpectQuery(regexp.QuoteMeta(query)).WithArgs(7, domainvideo.StatusPublished).
		WillReturnRows(sqlmock.NewRows([]string{"video_id", "author_id", "author_nickname", "author_avatar_url", "title", "description", "media_url", "cover_url"}).
			AddRow(7, 42, "测试作者", "/avatar.png", "测试视频", "视频说明", "/video.mp4", "/cover.png"))
	cards, err := repo.BatchGetFeedCards(context.Background(), []int64{7})
	if err != nil {
		t.Fatal(err)
	}
	card := cards[7]
	if len(cards) != 1 || card == nil || card.AuthorID != 42 || card.AuthorNickname != "测试作者" || card.AuthorAvatarURL != "/avatar.png" || card.Title != "测试视频" || card.Description != "视频说明" || card.MediaURL != "/video.mp4" || card.CoverURL != "/cover.png" {
		t.Fatalf("unexpected cards: %+v", card)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
