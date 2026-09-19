package embedding

import (
	"testing"
	"time"
)

// TestVideoEmbeddingValueSemantics 验证文本拼接边界、切片复制、字段清洗和数据库恢复不隐式解析 JSON。
func TestVideoEmbeddingValueSemantics(t *testing.T) {
	for _, tc := range []struct{ title, description, want string }{{" ", " ", ""}, {" a ", " ", "a"}, {" ", " b ", "b"}, {" a ", " b ", "a\nb"}} {
		if got := BuildVideoText(tc.title, tc.description); got != tc.want {
			t.Fatal(got)
		}
	}
	if TextHash(" a ") != TextHash("a") || TextHash("A") == TextHash("a") {
		t.Fatal("哈希清洗改变了大小写语义")
	}
	vector := []float64{1, 2}
	value := NewVideoEmbedding(42, " model ", vector, " hash ", " [1,2] ")
	vector[0] = 99
	if value.VideoID != 42 || value.Model != "model" || value.Dimension != 2 || value.Embedding[0] != 1 || value.TextHash != "hash" || value.EmbeddingJSON != "[1,2]" {
		t.Fatal(value)
	}
	now := time.Now()
	restored := RestoreVideoEmbedding(42, " model ", 2, " [1,2] ", " hash ", now, now)
	if restored.Embedding != nil || restored.Dimension != 2 || restored.EmbeddingJSON != "[1,2]" || restored.Model != "model" || restored.TextHash != "hash" || !restored.CreatedAt.Equal(now) || !restored.UpdatedAt.Equal(now) {
		t.Fatal(restored)
	}
}
