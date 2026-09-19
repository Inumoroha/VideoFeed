package embedding

import (
	"math"
	"reflect"
	"sync"
	"testing"
)

// TestVectorizerDefaults 验证空接收者、零值及非正维度回退，并检查内部自定义维度。
func TestVectorizerDefaults(t *testing.T) {
	var nilVectorizer *HashNgramVectorizer
	for _, vectorizer := range []*HashNgramVectorizer{nilVectorizer, {}, {dimension: -1}, NewHashNgramVectorizer()} {
		var contract Vectorizer = vectorizer
		if contract.Model() != "hash-ngram-v1" || contract.Dimension() != 128 || len(contract.Vectorize("a")) != 128 {
			t.Fatal("默认模型或维度不符")
		}
	}
	custom := &HashNgramVectorizer{dimension: 17}
	if custom.Dimension() != 17 || len(custom.Vectorize("中文 123")) != 17 {
		t.Fatal("未保留内部指定维度")
	}
}

// TestUnicodeNormalization 验证中英文、数字、符号分隔与原算法忽略控制字符和组合标记的行为。
func TestUnicodeNormalization(t *testing.T) {
	for input, want := range map[string]string{
		"  HELLO，世界！123\tGo  ": "hello 世界 123 go",
		"✨\n，\t!!!":            "",
		"a\x00b":               "ab",
		"e\u0301":              "e",
		"中文—测试":                "中文 测试",
	} {
		if got := normalizeText(input); got != want {
			t.Errorf("normalizeText(%q) = %q, want %q", input, got, want)
		}
	}
	v := NewHashNgramVectorizer()
	if !reflect.DeepEqual(v.Vectorize("HELLO，世界"), v.Vectorize("hello 世界")) {
		t.Fatal("等价规范化文本产生不同向量")
	}
	if norm(v.Vectorize("✨！")) != 0 || math.Abs(norm(v.Vectorize("a"))-1) > 1e-12 {
		t.Fatal("纯符号或单字符边界错误")
	}
}

// TestCosineSimilarityEdges 验证零向量、空向量以及未归一化向量的同向、反向和正交结果。
func TestCosineSimilarityEdges(t *testing.T) {
	for _, tc := range []struct {
		left, right []float64
		want        float64
	}{
		{nil, nil, 0},
		{[]float64{}, nil, 0},
		{[]float64{0, 0}, []float64{1, 2}, 0},
		{[]float64{1, 2}, []float64{0, 0}, 0},
		{[]float64{3, 4}, []float64{6, 8}, 1},
		{[]float64{3, 4}, []float64{-6, -8}, -1},
		{[]float64{3, 4}, []float64{-4, 3}, 0},
	} {
		got, err := CosineSimilarity(tc.left, tc.right)
		if err != nil || math.Abs(got-tc.want) > 1e-12 {
			t.Fatalf("left=%v right=%v got=%v err=%v", tc.left, tc.right, got, err)
		}
	}
}

// TestFeatureAndNormalizationEdges 验证特征权重叠加、空特征不写入和零向量归一化不变。
func TestFeatureAndNormalizationEdges(t *testing.T) {
	addFeature(nil, "a", 1)
	v := make([]float64, 128)
	addFeature(v, "", 1)
	normalizeVector(v)
	if norm(v) != 0 {
		t.Fatal("空特征改变零向量")
	}
	addFeature(v, "feature", 2)
	addFeature(v, "feature", -2)
	if norm(v) != 0 {
		t.Fatal("同特征相反权重未抵消")
	}
	values := []float64{3, 4}
	normalizeVector(values)
	if values[0] != 0.6 || values[1] != 0.8 {
		t.Fatal(values)
	}
}

// TestVectorizerConcurrentIsolation 验证共享向量器并发调用结果稳定，修改返回切片不影响其他调用。
func TestVectorizerConcurrentIsolation(t *testing.T) {
	v := NewHashNgramVectorizer()
	input := "篮球投篮训练 中文 ABC 123"
	want := v.Vectorize(input)
	var workers sync.WaitGroup
	for i := 0; i < 32; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			got := v.Vectorize(input)
			if !reflect.DeepEqual(got, want) {
				t.Error("并发调用结果发生变化")
			}
			got[0] = 999
		}()
	}
	workers.Wait()
	if !reflect.DeepEqual(v.Vectorize(input), want) {
		t.Fatal("调用结果共享底层存储")
	}
}
