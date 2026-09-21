package message

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// TestMessageValidationBoundaries 验证所有长度常量按 UTF-8 字节执行，类型归一化且空值按原规则处理。
func TestMessageValidationBoundaries(t *testing.T) {
	if TypeLike != "LIKE" || TypeComment != "COMMENT" || TypeFollow != "FOLLOW" || TypeSystem != "SYSTEM" || MaxTitleLength != 128 || MaxContentLength != 1024 || MaxEventIDLength != 64 || MaxIdempotencyKeyLength != 128 || MaxLimit != 100 {
		t.Fatal("迁移常量变化")
	}
	for _, tc := range []struct {
		u                          int64
		typ, title, content, event string
		want                       error
	}{
		{0, "LIKE", "a", "b", "", ErrInvalidUserID}, {1, "bad", "a", "b", "", ErrInvalidMessageType},
		{1, "LIKE", " ", "b", "", ErrEmptyTitle}, {1, "LIKE", strings.Repeat("中", 43), "b", "", ErrTitleTooLong},
		{1, "LIKE", "a", " ", "", ErrEmptyContent}, {1, "LIKE", "a", strings.Repeat("x", 1025), "", ErrContentTooLong},
		{1, "LIKE", "a", "b", strings.Repeat("x", 65), ErrEventIDTooLong},
		{1, " like ", strings.Repeat("x", 128), strings.Repeat("x", 1024), strings.Repeat("x", 64), nil},
	} {
		m, err := New(tc.u, tc.typ, tc.title, tc.content, tc.event)
		if !errors.Is(err, tc.want) {
			t.Fatalf("err=%v want=%v", err, tc.want)
		}
		if err == nil && (m.Type != TypeLike || m.IsRead) {
			t.Fatal(m)
		}
	}
	now := time.Now()
	m := Restore(1, 42, " system ", " a ", " b ", " e ", true, now, &now)
	if m.Type != TypeSystem || m.Title != "a" || m.Content != "b" || m.EventID != "e" || !m.IsRead || m.ReadAt == nil {
		t.Fatal(m)
	}
}
