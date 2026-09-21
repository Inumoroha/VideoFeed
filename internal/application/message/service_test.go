package message

import (
	"context"
	"errors"
	domainmessage "github.com/inumoroha/video-feed/internal/domain/message"
	"testing"
)

// failureRepository 为每个消息仓储操作返回可识别的底层错误。
type failureRepository struct{}

// Create 模拟消息保存失败。
func (failureRepository) Create(context.Context, *domainmessage.Message, string) (*domainmessage.Message, bool, error) {
	return nil, false, errors.New("save")
}

// ListByUser 模拟消息列表读取失败。
func (failureRepository) ListByUser(context.Context, int64, *domainmessage.Cursor, int) ([]*domainmessage.Message, error) {
	return nil, errors.New("list")
}

// CountUnread 模拟未读数读取失败。
func (failureRepository) CountUnread(context.Context, int64) (int, error) {
	return 0, errors.New("count")
}

// MarkRead 模拟已读更新失败。
func (failureRepository) MarkRead(context.Context, int64, []int64) (int, error) {
	return 0, errors.New("update")
}

// TestServiceMapsFailures 验证各仓储故障转换为对应应用错误，非法输入在调用仓储之前拒绝。
func TestServiceMapsFailures(t *testing.T) {
	s := New(failureRepository{})
	ctx := context.Background()
	if _, err := s.CreateFromEvent(ctx, 42, "SYSTEM", "title", "content", "", ""); !errors.Is(err, ErrSaveMessageFailed) {
		t.Fatal(err)
	}
	if _, err := s.List(ctx, 42, "", 20); !errors.Is(err, ErrLoadMessageFailed) {
		t.Fatal(err)
	}
	if _, err := s.CountUnread(ctx, 42); !errors.Is(err, ErrLoadMessageFailed) {
		t.Fatal(err)
	}
	if _, err := s.MarkRead(ctx, 42, []int64{1, 1}); !errors.Is(err, ErrUpdateMessageFailed) {
		t.Fatal(err)
	}
	if _, err := s.MarkRead(ctx, 42, []int64{-1}); !errors.Is(err, domainmessage.ErrInvalidMessageID) {
		t.Fatal(err)
	}
	if _, err := s.List(ctx, 0, "", 20); !errors.Is(err, domainmessage.ErrInvalidUserID) {
		t.Fatal(err)
	}
	if _, err := s.CountUnread(ctx, 0); !errors.Is(err, domainmessage.ErrInvalidUserID) {
		t.Fatal(err)
	}
	if _, err := s.MarkRead(ctx, 0, nil); !errors.Is(err, domainmessage.ErrInvalidUserID) {
		t.Fatal(err)
	}
}
