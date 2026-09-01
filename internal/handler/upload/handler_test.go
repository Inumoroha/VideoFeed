package upload

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestCreate 保存 multipart 文件，并返回对应的上传信息。
func TestCreate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	root := t.TempDir()
	handler := NewHandler(root)
	request := multipartRequest(t, "video.mp4", "video", []byte("video data"))
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = request

	handler.Create(context)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, recorder.Code, recorder.Body.String())
	}
	var response uploadResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Kind != "video" || response.Size != int64(len("video data")) {
		t.Fatalf("unexpected response: %+v", response)
	}
	if !strings.HasPrefix(response.URL, "/uploads/video/") || filepath.Ext(response.Filename) != ".mp4" {
		t.Fatalf("unexpected file metadata: %+v", response)
	}
	content, err := os.ReadFile(filepath.Join(root, "video", response.Filename))
	if err != nil {
		t.Fatalf("read saved file: %v", err)
	}
	if string(content) != "video data" {
		t.Fatalf("unexpected saved content: %q", content)
	}
}

// TestCreateRejectsInvalidRequests 验证请求校验失败时返回对应的错误状态。
func TestCreateRejectsInvalidRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name       string
		handler    *Handler
		request    *http.Request
		statusCode int
	}{
		{
			name:       "nil handler",
			handler:    nil,
			request:    multipartRequest(t, "file.txt", "file", []byte("data")),
			statusCode: http.StatusInternalServerError,
		},
		{
			name:       "empty root",
			handler:    NewHandler("  "),
			request:    multipartRequest(t, "file.txt", "file", []byte("data")),
			statusCode: http.StatusInternalServerError,
		},
		{
			name:       "missing file",
			handler:    NewHandler(t.TempDir()),
			request:    httptest.NewRequest(http.MethodPost, "/upload", strings.NewReader("kind=file")),
			statusCode: http.StatusBadRequest,
		},
		{
			name:       "invalid kind",
			handler:    NewHandler(t.TempDir()),
			request:    multipartRequest(t, "file.txt", "document", []byte("data")),
			statusCode: http.StatusBadRequest,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			context.Request = test.request
			test.handler.Create(context)
			if recorder.Code != test.statusCode {
				t.Fatalf("expected status %d, got %d: %s", test.statusCode, recorder.Code, recorder.Body.String())
			}
		})
	}
}

// TestCreateDefaultsKindAndNormalizesExtension 验证类型默认值和异常扩展名回退逻辑。
func TestCreateDefaultsKindAndNormalizesExtension(t *testing.T) {
	gin.SetMode(gin.TestMode)
	root := t.TempDir()
	request := multipartRequest(t, `..\outside<script>`, "", []byte("data"))
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = request

	NewHandler(root).Create(context)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, recorder.Code, recorder.Body.String())
	}
	var response uploadResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Kind != "file" || filepath.Ext(response.Filename) != ".bin" {
		t.Fatalf("unexpected response: %+v", response)
	}
	if _, err := os.Stat(filepath.Join(root, "file", response.Filename)); err != nil {
		t.Fatalf("saved file not found: %v", err)
	}
}

// TestNormalizeKind 验证支持的上传类型及默认类型。
func TestNormalizeKind(t *testing.T) {
	for _, test := range []struct {
		input string
		kind  string
		ok    bool
	}{
		{input: "video", kind: "video", ok: true},
		{input: " COVER ", kind: "cover", ok: true},
		{input: "avatar", kind: "avatar", ok: true},
		{input: "", kind: "file", ok: true},
		{input: "file", kind: "file", ok: true},
		{input: "document", ok: false},
	} {
		kind, ok := normalizeKind(test.input)
		if kind != test.kind || ok != test.ok {
			t.Errorf("normalizeKind(%q) = (%q, %t), want (%q, %t)", test.input, kind, ok, test.kind, test.ok)
		}
	}
}

// multipartRequest 创建包含文件和类型字段的 multipart 请求。
func multipartRequest(t *testing.T, filename, kind string, content []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := writer.WriteField("kind", kind); err != nil {
		t.Fatalf("write kind field: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/upload", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}
