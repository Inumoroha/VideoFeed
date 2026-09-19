package embedding

import (
	"hash/fnv"
	"math"
	"strings"
	"unicode"
)

const HashNgramModel = "hash-ngram-v1"
const HashNgramDimension = 128

// Vectorizer 把文本转换为固定维度向量。
type Vectorizer interface {
	Model() string
	Dimension() int
	Vectorize(text string) []float64
}

// HashNgramVectorizer 使用本地哈希特征将文本映射为固定维度向量。
type HashNgramVectorizer struct {
	dimension int
}

// NewHashNgramVectorizer 创建默认 128 维哈希向量器。
func NewHashNgramVectorizer() *HashNgramVectorizer {
	return &HashNgramVectorizer{dimension: HashNgramDimension}
}

// Model 返回固定的本地哈希向量模型标识。
func (v *HashNgramVectorizer) Model() string {
	return HashNgramModel
}

// Dimension 返回配置维度；空接收者或非正维度回退到默认 128 维。
func (v *HashNgramVectorizer) Dimension() int {
	if v == nil || v.dimension <= 0 {
		return HashNgramDimension
	}
	return v.dimension
}

// Vectorize 使用词项及二元、三元字符特征生成新向量并做 L2 归一化，可并发调用。
func (v *HashNgramVectorizer) Vectorize(text string) []float64 {
	dimension := v.Dimension()
	vector := make([]float64, dimension)
	normalized := normalizeText(text)
	if normalized == "" {
		return vector
	}

	tokens := strings.Fields(normalized)
	for _, token := range tokens {
		addFeature(vector, "tok:"+token, 1.0)
	}

	runes := []rune(strings.ReplaceAll(normalized, " ", ""))
	for n := 2; n <= 3; n++ {
		if len(runes) < n {
			continue
		}
		for i := 0; i+n <= len(runes); i++ {
			addFeature(vector, string(runes[i:i+n]), 1.0)
		}
	}

	normalizeVector(vector)
	return vector
}

// CosineSimilarity 计算两个已归一化或未归一化向量的余弦相似度。
func CosineSimilarity(left []float64, right []float64) (float64, error) {
	if len(left) != len(right) {
		return 0, ErrDimensionMismatch
	}
	var dot float64
	var leftNorm float64
	var rightNorm float64
	for i := range left {
		dot += left[i] * right[i]
		leftNorm += left[i] * left[i]
		rightNorm += right[i] * right[i]
	}
	if leftNorm == 0 || rightNorm == 0 {
		return 0, nil
	}
	return dot / (math.Sqrt(leftNorm) * math.Sqrt(rightNorm)), nil
}

// normalizeText 转小写并保留字母数字，将空白、标点和符号折叠为空格，忽略其他字符。
func normalizeText(text string) string {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return ""
	}

	var builder strings.Builder
	previousSpace := true
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
			previousSpace = false
			continue
		}
		if unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r) {
			if !previousSpace {
				builder.WriteRune(' ')
				previousSpace = true
			}
		}
	}
	return strings.TrimSpace(builder.String())
}

// addFeature 按 FNV-1a 哈希确定位置和符号并原地累加权重，空向量或空特征不处理。
func addFeature(vector []float64, feature string, weight float64) {
	if len(vector) == 0 || feature == "" {
		return
	}
	hasher := fnv.New64a()
	_, _ = hasher.Write([]byte(feature))
	hash := hasher.Sum64()
	index := int(hash % uint64(len(vector)))
	sign := 1.0
	if (hash>>63)&1 == 1 {
		sign = -1.0
	}
	vector[index] += sign * weight
}

// normalizeVector 按 L2 范数原地归一化向量，零范数时保持原值。
func normalizeVector(vector []float64) {
	var sum float64
	for _, value := range vector {
		sum += value * value
	}
	if sum == 0 {
		return
	}
	norm := math.Sqrt(sum)
	for i := range vector {
		vector[i] = vector[i] / norm
	}
}
