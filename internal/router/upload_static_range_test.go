package router

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestUploadStaticRangeRequest 使用临时文件验证静态视频的分段状态、范围头和精确响应内容。
func TestUploadStaticRangeRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	uploadDir := t.TempDir()
	videoDir := filepath.Join(uploadDir, "video")
	if err := os.MkdirAll(videoDir, 0o755); err != nil {
		t.Fatalf("create video dir: %v", err)
	}

	content := []byte("0123456789abcdef0123456789abcdef")
	if err := os.WriteFile(filepath.Join(videoDir, "range.mp4"), content, 0o644); err != nil {
		t.Fatalf("write test video: %v", err)
	}

	router := gin.New()
	router.Static("/uploads", uploadDir)

	req := httptest.NewRequest(http.MethodGet, "/uploads/video/range.mp4", nil)
	req.Header.Set("Range", "bytes=0-15")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	requireStatus(t, resp, http.StatusPartialContent)
	if got := resp.Header().Get("Content-Range"); got != "bytes 0-15/32" {
		t.Fatalf("expected content range bytes 0-15/32, got %q", got)
	}
	if got := resp.Body.String(); got != string(content[:16]) {
		t.Fatalf("expected ranged body %q, got %q", string(content[:16]), got)
	}
}

// TestUploadStaticRangeBoundaries 验证完整文件、尾部和开放范围、越界范围以及无正文的头部请求。
func TestUploadStaticRangeBoundaries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	content := "0123456789abcdef0123456789abcdef"
	if err := os.WriteFile(filepath.Join(dir, "range.mp4"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.Static("/uploads", dir)
	cases := []struct {
		name, method, rangeHeader string
		status                    int
		contentRange, body        string
	}{
		{"完整文件", http.MethodGet, "", http.StatusOK, "", content},
		{"尾部范围", http.MethodGet, "bytes=-4", http.StatusPartialContent, "bytes 28-31/32", "cdef"},
		{"开放范围", http.MethodGet, "bytes=16-", http.StatusPartialContent, "bytes 16-31/32", content[16:]},
		{"越界范围", http.MethodGet, "bytes=32-", http.StatusRequestedRangeNotSatisfiable, "bytes */32", ""},
		{"头部请求", http.MethodHead, "", http.StatusOK, "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "/uploads/range.mp4", nil)
			if tc.rangeHeader != "" {
				req.Header.Set("Range", tc.rangeHeader)
			}
			resp := httptest.NewRecorder()
			router.ServeHTTP(resp, req)
			requireStatus(t, resp, tc.status)
			if got := resp.Header().Get("Content-Range"); got != tc.contentRange {
				t.Fatalf("content range: %q", got)
			}
			if tc.status != http.StatusRequestedRangeNotSatisfiable && resp.Body.String() != tc.body {
				t.Fatalf("body: %q", resp.Body.String())
			}
			if tc.status == http.StatusOK && resp.Header().Get("Content-Length") != "32" {
				t.Fatalf("content length: %q", resp.Header().Get("Content-Length"))
			}
		})
	}
}
