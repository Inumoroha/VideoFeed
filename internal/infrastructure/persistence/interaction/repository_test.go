package interaction

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	domaininteraction "github.com/inumoroha/video-feed/internal/domain/interaction"
	"gorm.io/gorm"
)

// TestInteractionModelsUsePostgresCompatibleStatusType 验证互动模型未使用 MySQL 专用状态类型。
func TestInteractionModelsUsePostgresCompatibleStatusType(t *testing.T) {
	for _, model := range []any{ActionModel{}, CommentModel{}} {
		field, ok := reflect.TypeOf(model).FieldByName("Status")
		if !ok {
			t.Fatalf("%T is missing Status field", model)
		}
		if strings.Contains(strings.ToLower(field.Tag.Get("gorm")), "tinyint") {
			t.Fatalf("%T Status GORM tag = %q, must not contain tinyint", model, field.Tag.Get("gorm"))
		}
	}
}

// TestRepositoryRejectsUninitializedRepository 验证未初始化仓储不会触发空指针异常。
func TestRepositoryRejectsUninitializedRepository(t *testing.T) {
	var repository *Repository
	if _, _, err := repository.ToggleAction(context.Background(), 1, 1, domaininteraction.ActionTypeLike, ""); err == nil || err.Error() != "interaction repository is not initialized" {
		t.Fatalf("ToggleAction() error = %v, want uninitialized repository error", err)
	}
	if _, err := repository.FindCommentByID(context.Background(), 1); err == nil || err.Error() != "interaction repository is not initialized" {
		t.Fatalf("FindCommentByID() error = %v, want uninitialized repository error", err)
	}
}

// TestRepositoryRejectsInvalidInput 验证仓储入口会拒绝无效标识、分页参数和游标。
func TestRepositoryRejectsInvalidInput(t *testing.T) {
	repository := New(&gorm.DB{})

	if _, _, err := repository.ToggleAction(context.Background(), 0, 1, domaininteraction.ActionTypeLike, ""); !errors.Is(err, domaininteraction.ErrInvalidUserID) {
		t.Fatalf("ToggleAction() error = %v, want %v", err, domaininteraction.ErrInvalidUserID)
	}
	if _, err := repository.FindCommentByID(context.Background(), 0); !errors.Is(err, domaininteraction.ErrInvalidCommentID) {
		t.Fatalf("FindCommentByID() error = %v, want %v", err, domaininteraction.ErrInvalidCommentID)
	}
	if _, err := repository.ListComments(context.Background(), 1, nil, 0); !errors.Is(err, domaininteraction.ErrInvalidLimit) {
		t.Fatalf("ListComments() error = %v, want %v", err, domaininteraction.ErrInvalidLimit)
	}
	if _, err := repository.ListComments(context.Background(), 1, &domaininteraction.CommentCursor{CommentID: 1}, 10); !errors.Is(err, domaininteraction.ErrInvalidCursor) {
		t.Fatalf("ListComments() error = %v, want %v", err, domaininteraction.ErrInvalidCursor)
	}
}

// TestValidateCommentCountsUnicodeCharacters 验证持久化入口与领域层一致地按字符校验评论长度。
func TestValidateCommentCountsUnicodeCharacters(t *testing.T) {
	comment := &domaininteraction.Comment{
		VideoID: 1,
		UserID:  2,
		Content: strings.Repeat("中", domaininteraction.MaxCommentContentLength),
	}
	if err := validateComment(comment); err != nil {
		t.Fatalf("validateComment() error = %v", err)
	}
	comment.Content += "中"
	if !errors.Is(validateComment(comment), domaininteraction.ErrCommentContentTooLong) {
		t.Fatalf("validateComment() error = %v, want %v", validateComment(comment), domaininteraction.ErrCommentContentTooLong)
	}
}
