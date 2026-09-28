package video

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	applicationvideo "github.com/inumoroha/video-feed/internal/application/video"
	domainvideo "github.com/inumoroha/video-feed/internal/domain/video"
	uploadhandler "github.com/inumoroha/video-feed/internal/handler/upload"
	"github.com/inumoroha/video-feed/internal/middleware"
)

// acceptingUploadProcessor 让接口测试通过视频工具校验并覆盖后续发布与回收逻辑。
type acceptingUploadProcessor struct{}

// ValidateVideo 在接口测试中接纳已保存的视频文件。
func (acceptingUploadProcessor) ValidateVideo(context.Context, string) error { return nil }

// Faststart 在接口测试中跳过实际转码。
func (acceptingUploadProcessor) Faststart(context.Context, string) error { return nil }

// multipartPublishRequest 创建含视频、封面和作品字段的测试请求。
func multipartPublishRequest(t *testing.T, cover []byte, title string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	videoPart, err := writer.CreateFormFile("video", "test.mp4")
	if err != nil {
		t.Fatal(err)
	}
	video := []byte{0, 0, 0, 24, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm', 0, 0, 0, 0, 'i', 's', 'o', 'm', 'm', 'p', '4', '2'}
	if _, err := videoPart.Write(video); err != nil {
		t.Fatal(err)
	}
	coverPart, err := writer.CreateFormFile("cover", "cover.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := coverPart.Write(cover); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("title", title); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("description", "integration upload"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/videos/upload", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("Idempotency-Key", "upload-publish-test")
	return request
}

// invokeMultipartPublish 使用真实临时目录调用组合发布处理器。
func invokeMultipartPublish(t *testing.T, root string, repo *handlerTestRepository, cover []byte, title string) (*httptest.ResponseRecorder, *gin.Context) {
	t.Helper()
	handler := NewHandler(applicationvideo.NewService(repo)).WithUploader(uploadhandler.NewWithProcessor(root, acceptingUploadProcessor{}))
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = multipartPublishRequest(t, cover, title)
	ctx.Set(middleware.ContextUserIDKey, int64(7))
	handler.PublishMultipart(ctx)
	return recorder, ctx
}

// pngUploadBytes 返回足以通过 MIME 探测的 PNG 文件头。
func pngUploadBytes() []byte {
	return []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 0}
}

// assertUploadFiles 验证指定上传类型下的文件数。
func assertUploadFiles(t *testing.T, root, kind string, want int) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(root, kind, "*"))
	if err != nil || len(files) != want {
		t.Fatalf("%s files=%v error=%v, want %d", kind, files, err, want)
	}
}

// TestPublishMultipartRetainsFilesOnSuccess 验证发布成功后视频和封面均可访问。
func TestPublishMultipartRetainsFilesOnSuccess(t *testing.T) {
	root := t.TempDir()
	repo := &handlerTestRepository{}
	recorder, _ := invokeMultipartPublish(t, root, repo, pngUploadBytes(), "live title")
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var result videoResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if repo.savedVideo == nil || result.ID != 1 || result.MediaURL == "" || result.CoverURL == "" {
		t.Fatalf("result=%+v saved=%+v", result, repo.savedVideo)
	}
	assertUploadFiles(t, root, "video", 1)
	assertUploadFiles(t, root, "cover", 1)
}

// TestPublishMultipartCleansVideoWhenCoverFails 验证封面格式错误时回收已保存的视频。
func TestPublishMultipartCleansVideoWhenCoverFails(t *testing.T) {
	root := t.TempDir()
	recorder, _ := invokeMultipartPublish(t, root, &handlerTestRepository{}, []byte("not an image"), "live title")
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	assertUploadFiles(t, root, "video", 0)
	assertUploadFiles(t, root, "cover", 0)
}

// TestPublishMultipartCleansFilesWhenSaveFails 验证数据库保存失败时回收两份文件。
func TestPublishMultipartCleansFilesWhenSaveFails(t *testing.T) {
	root := t.TempDir()
	repo := &handlerTestRepository{saveErr: errors.New("database unavailable")}
	recorder, _ := invokeMultipartPublish(t, root, repo, pngUploadBytes(), "live title")
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	assertUploadFiles(t, root, "video", 0)
	assertUploadFiles(t, root, "cover", 0)
}

// TestPublishMultipartCleansFilesWhenValidationFails 验证发布字段无效时回收两份文件。
func TestPublishMultipartCleansFilesWhenValidationFails(t *testing.T) {
	root := t.TempDir()
	recorder, _ := invokeMultipartPublish(t, root, &handlerTestRepository{}, pngUploadBytes(), "")
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	assertUploadFiles(t, root, "video", 0)
	assertUploadFiles(t, root, "cover", 0)
}

// TestPublishMultipartCleansFilesOnIdempotentReplay 验证复用旧作品时丢弃重试产生的文件。
func TestPublishMultipartCleansFilesOnIdempotentReplay(t *testing.T) {
	root := t.TempDir()
	repo := &handlerTestRepository{existingVideo: &domainvideo.Video{ID: 42, AuthorID: 7, Title: "old", MediaURL: "/uploads/video/old.mp4", CoverURL: "/uploads/cover/old.png"}}
	recorder, _ := invokeMultipartPublish(t, root, repo, pngUploadBytes(), "live title")
	if recorder.Code != http.StatusOK || repo.savedVideo != nil {
		t.Fatalf("status=%d body=%s saved=%+v", recorder.Code, recorder.Body.String(), repo.savedVideo)
	}
	assertUploadFiles(t, root, "video", 0)
	assertUploadFiles(t, root, "cover", 0)
}

// TestPublishMultipartRequiresAuthentication 验证缺少认证用户时不保存文件。
func TestPublishMultipartRequiresAuthentication(t *testing.T) {
	root := t.TempDir()
	handler := NewHandler(applicationvideo.NewService(&handlerTestRepository{})).WithUploader(uploadhandler.NewWithProcessor(root, acceptingUploadProcessor{}))
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = multipartPublishRequest(t, pngUploadBytes(), "live title")
	handler.PublishMultipart(ctx)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	assertUploadFiles(t, root, "video", 0)
	assertUploadFiles(t, root, "cover", 0)
}

// TestCleanupSavedFilesIgnoresMissingFile 验证清理已不存在的文件不会中断后续清理。
func TestCleanupSavedFilesIgnoresMissingFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "remaining.mp4")
	if err := os.WriteFile(path, []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}
	cleanupSavedFiles(&uploadhandler.SavedFile{Path: filepath.Join(root, "missing.mp4")}, &uploadhandler.SavedFile{Path: path})
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("file remains: %v", err)
	}
}
