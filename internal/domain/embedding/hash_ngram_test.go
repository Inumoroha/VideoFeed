package embedding

import (
	"math"
	"testing"
)

// TestHashNgramVectorizerDeterministic 验证相同输入产生相同模型、维度及单位范数向量。
func TestHashNgramVectorizerDeterministic(t *testing.T) {
	vectorizer := NewHashNgramVectorizer()

	left := vectorizer.Vectorize("篮球训练\n投篮技巧")
	right := vectorizer.Vectorize("篮球训练\n投篮技巧")

	if vectorizer.Model() != HashNgramModel {
		t.Fatalf("unexpected model: %s", vectorizer.Model())
	}
	if len(left) != HashNgramDimension || len(right) != HashNgramDimension {
		t.Fatalf("unexpected dimension: left=%d right=%d", len(left), len(right))
	}
	for i := range left {
		if left[i] != right[i] {
			t.Fatalf("vector changed at %d: %f != %f", i, left[i], right[i])
		}
	}
	if norm(left) < 0.999 || norm(left) > 1.001 {
		t.Fatalf("unexpected normalized vector norm: %f", norm(left))
	}
}

// TestHashNgramVectorizerSimilarity 验证给定相关文本的余弦得分高于无关文本。
func TestHashNgramVectorizerSimilarity(t *testing.T) {
	vectorizer := NewHashNgramVectorizer()

	base := vectorizer.Vectorize("篮球训练 投篮技巧")
	similar := vectorizer.Vectorize("篮球投篮训练 技巧")
	different := vectorizer.Vectorize("美食探店 火锅推荐")

	similarScore, err := CosineSimilarity(base, similar)
	if err != nil {
		t.Fatalf("similar cosine: %v", err)
	}
	differentScore, err := CosineSimilarity(base, different)
	if err != nil {
		t.Fatalf("different cosine: %v", err)
	}
	if similarScore <= differentScore {
		t.Fatalf("expected similar text score to be higher: similar=%f different=%f", similarScore, differentScore)
	}
}

// TestHashNgramVectorizerEmptyText 验证空白文本生成默认维度的零向量。
func TestHashNgramVectorizerEmptyText(t *testing.T) {
	vectorizer := NewHashNgramVectorizer()

	vector := vectorizer.Vectorize("   \n\t")
	if len(vector) != HashNgramDimension {
		t.Fatalf("unexpected dimension: %d", len(vector))
	}
	if norm(vector) != 0 {
		t.Fatalf("expected zero vector for empty text: %f", norm(vector))
	}
}

// TestCosineSimilarityDimensionMismatch 验证不同维度返回约定的维度错误。
func TestCosineSimilarityDimensionMismatch(t *testing.T) {
	_, err := CosineSimilarity([]float64{1, 0}, []float64{1})
	if err != ErrDimensionMismatch {
		t.Fatalf("expected dimension mismatch, got %v", err)
	}
}

// norm 计算测试向量的 L2 范数。
func norm(vector []float64) float64 {
	var sum float64
	for _, value := range vector {
		sum += value * value
	}
	return math.Sqrt(sum)
}
