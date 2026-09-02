package interaction

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// TestNormalizeActionType 验证动作类型会被清理并统一为大写。
func TestNormalizeActionType(t *testing.T) {
	got, err := NormalizeActionType(" favorite ")
	if err != nil {
		t.Fatalf("NormalizeActionType() error = %v", err)
	}
	if got != ActionTypeFavorite {
		t.Fatalf("NormalizeActionType() = %q, want %q", got, ActionTypeFavorite)
	}

	if _, err := NormalizeActionType("share"); !errors.Is(err, ErrInvalidActionType) {
		t.Fatalf("NormalizeActionType() error = %v, want %v", err, ErrInvalidActionType)
	}
}

// TestNewCommentCountsUnicodeCharacters 验证评论长度按 Unicode 字符而非字节计算。
func TestNewCommentCountsUnicodeCharacters(t *testing.T) {
	content := strings.Repeat("中", MaxCommentContentLength)
	comment, err := NewComment(1, 2, content, " key ")
	if err != nil {
		t.Fatalf("NewComment() error = %v", err)
	}
	if comment.Content != content || comment.IdempotencyKey != "key" || comment.Status != CommentStatusNormal {
		t.Fatalf("NewComment() = %+v, want trimmed normal comment", comment)
	}

	if _, err := NewComment(1, 2, content+"中", ""); !errors.Is(err, ErrCommentContentTooLong) {
		t.Fatalf("NewComment() error = %v, want %v", err, ErrCommentContentTooLong)
	}
}

// TestRestoreActionValidatesActionType 验证动作恢复会拒绝非法动作类型并设置默认状态。
func TestRestoreActionValidatesActionType(t *testing.T) {
	action, err := RestoreAction(1, 2, 3, " like ", 0, " key ", time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("RestoreAction() error = %v", err)
	}
	if action.ActionType != ActionTypeLike || action.Status != ActionStatusActive || action.IdempotencyKey != "key" {
		t.Fatalf("RestoreAction() = %+v, want normalized active action", action)
	}

	if _, err := RestoreAction(1, 2, 3, "share", ActionStatusActive, "", time.Time{}, time.Time{}); !errors.Is(err, ErrInvalidActionType) {
		t.Fatalf("RestoreAction() error = %v, want %v", err, ErrInvalidActionType)
	}
}

// TestRestoreCommentAndStatusPredicates 验证评论恢复和状态判断行为。
func TestRestoreCommentAndStatusPredicates(t *testing.T) {
	comment := RestoreComment(1, 2, 3, " author ", " avatar ", " content ", 0, " key ", time.Time{}, time.Time{})
	if comment.UserNickname != "author" || comment.UserAvatarURL != "avatar" || comment.Content != "content" || comment.Status != CommentStatusNormal {
		t.Fatalf("RestoreComment() = %+v, want normalized normal comment", comment)
	}
	if !(&Action{Status: ActionStatusActive}).Active() {
		t.Fatal("Active() = false, want true")
	}
	if !(&Comment{Status: CommentStatusDeleted}).Deleted() {
		t.Fatal("Deleted() = false, want true")
	}
	var action *Action
	var nilComment *Comment
	if action.Active() || nilComment.Deleted() {
		t.Fatal("nil status predicates returned an unexpected true result")
	}
}
