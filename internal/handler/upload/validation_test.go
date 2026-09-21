package upload

import (
	"bytes"
	"context"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestUploadSizeBoundaries 验证各类文件上限、超限和空内容，避免创建大体积测试文件。
func TestUploadSizeBoundaries(t *testing.T) {
	for _, tc := range []struct {
		kind string
		size int64
		want error
	}{
		{"video", maxVideoBytes, nil}, {"video", maxVideoBytes + 1, errUploadTooLarge},
		{"cover", maxImageBytes, nil}, {"cover", maxImageBytes + 1, errUploadTooLarge},
		{"avatar", maxImageBytes + 1, errUploadTooLarge}, {"file", maxUploadBytes, nil},
		{"file", maxUploadBytes + 1, errUploadTooLarge}, {"file", 0, errInvalidUploadMIME},
	} {
		if err := validateUploadSize(tc.size, tc.kind); !errors.Is(err, tc.want) {
			t.Fatalf("%+v: %v", tc, err)
		}
	}
}

// TestUploadFileRules 验证扩展名、真实内容探测、空文件及普通文件的兼容清理规则。
func TestUploadFileRules(t *testing.T) {
	for _, tc := range []struct {
		name, kind string
		data       []byte
		want       error
		ext        string
	}{
		{"clip.MP4", "video", []byte{0, 0, 0, 0, 0, 0, 0, 0}, nil, ".mp4"},
		{"clip", "video", []byte("x"), errInvalidUploadExtension, ""},
		{"clip.exe", "video", []byte("x"), errInvalidUploadExtension, ""},
		{"clip.mp4", "video", []byte("text"), errInvalidUploadMIME, ""},
		{"cover.jpg", "cover", []byte("text"), errInvalidUploadMIME, ""},
		{"cover.png", "cover", []byte{137, 80, 78, 71, 13, 10, 26, 10}, nil, ".png"},
		{"file.bad!", "file", []byte("text"), nil, ".bin"},
		{"file", "file", []byte("text"), nil, ".bin"},
		{"file.txt", "file", nil, errInvalidUploadMIME, ""},
	} {
		req := multipartRequest(t, tc.name, tc.kind, tc.data)
		if err := req.ParseMultipartForm(1024); err != nil {
			t.Fatal(err)
		}
		got, err := validateUploadFile(req.MultipartForm.File["file"][0], tc.kind)
		req.MultipartForm.RemoveAll()
		if !errors.Is(err, tc.want) {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if err == nil && got.Ext != tc.ext {
			t.Fatalf("%s: %+v", tc.name, got)
		}
	}
	if _, err := sniffUploadMIME(&multipart.FileHeader{}); !errors.Is(err, errInvalidUploadMIME) {
		t.Fatal(err)
	}
	for ext := range allowedVideoExt {
		if err := validateUploadExtension(ext, "video"); err != nil {
			t.Fatal(err)
		}
	}
	for ext := range allowedImageExt {
		if err := validateUploadExtension(ext, "avatar"); err != nil {
			t.Fatal(err)
		}
	}
	for mime := range allowedVideoMIME {
		if err := validateUploadMIME(mime, "video"); err != nil {
			t.Fatal(err)
		}
	}
	for mime := range allowedImageMIME {
		if err := validateUploadMIME(mime, "avatar"); err != nil {
			t.Fatal(err)
		}
	}
	if !shouldFaststart(" .MOV ") || !shouldFaststart(".mp4") || shouldFaststart(".webm") {
		t.Fatal("faststart selection")
	}
}

// TestVideoMetadataBoundaries 验证空元数据、非有限时长、全部编码白名单及缺失或非法视频流。
func TestVideoMetadataBoundaries(t *testing.T) {
	if err := validateVideoMetadata(nil); !errors.Is(err, errInvalidVideoMetadata) {
		t.Fatal(err)
	}
	for _, duration := range []string{"", "0", "-1", "NaN", "+Inf", "-Inf", "bad"} {
		if err := validateVideoMetadata(&probeResult{Format: probeFormat{Duration: duration}}); !errors.Is(err, errInvalidVideoMetadata) {
			t.Fatalf("%s: %v", duration, err)
		}
	}
	for codec := range allowedVideoCodec {
		for audio := range allowedAudioCodec {
			data := &probeResult{Format: probeFormat{Duration: "600"}, Streams: []probeStream{{CodecType: "video", CodecName: codec, Width: 3840, Height: 3840}, {CodecType: "audio", CodecName: audio}}}
			if err := validateVideoMetadata(data); err != nil {
				t.Fatalf("%s %s: %v", codec, audio, err)
			}
		}
	}
	for _, tc := range []struct {
		stream probeStream
		want   error
	}{
		{probeStream{CodecType: "video", CodecName: "h264", Width: 0, Height: 1}, errInvalidVideoMetadata},
		{probeStream{CodecType: "video", CodecName: "h264", Width: 1, Height: 3841}, errVideoTooLargeDimension},
		{probeStream{CodecType: "audio", CodecName: "aac"}, errInvalidVideoMetadata},
		{probeStream{CodecType: "audio", CodecName: "pcm_s16le"}, errUnsupportedVideoCodec},
	} {
		if err := validateVideoMetadata(&probeResult{Format: probeFormat{Duration: "1"}, Streams: []probeStream{tc.stream}}); !errors.Is(err, tc.want) {
			t.Fatal(err)
		}
	}
}

// failingUploadProcessor 注入视频校验或快启失败并统计调用顺序。
type failingUploadProcessor struct {
	validation, faststart error
	calls                 []string
}

// ValidateVideo 记录验证调用并返回指定错误。
func (p *failingUploadProcessor) ValidateVideo(context.Context, string) error {
	p.calls = append(p.calls, "validate")
	return p.validation
}

// Faststart 记录快启调用并返回指定错误。
func (p *failingUploadProcessor) Faststart(context.Context, string) error {
	p.calls = append(p.calls, "faststart")
	return p.faststart
}

// TestProcessingFailureCleanupAndRetry 验证失败状态、失败后文件清理、调用短路以及重新上传成功。
func TestProcessingFailureCleanupAndRetry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		validation, faststart error
		status, calls         int
	}{
		{errVideoTooLong, nil, 400, 1}, {errVideoToolUnavailable, nil, 500, 1}, {nil, errFaststartFailed, 500, 2},
	} {
		root := t.TempDir()
		p := &failingUploadProcessor{validation: tc.validation, faststart: tc.faststart}
		h := NewWithProcessor(root, p)
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = multipartRequest(t, "clip.mp4", "video", make([]byte, 32))
		h.Create(c)
		if rec.Code != tc.status || len(p.calls) != tc.calls {
			t.Fatalf("%d %s %v", rec.Code, rec.Body.String(), p.calls)
		}
		files, err := os.ReadDir(filepath.Join(root, "video"))
		if err != nil || len(files) != 0 {
			t.Fatalf("cleanup: %v %v", files, err)
		}
		p.validation = nil
		p.faststart = nil
		p.calls = nil
		rec = httptest.NewRecorder()
		c, _ = gin.CreateTestContext(rec)
		c.Request = multipartRequest(t, "clip.mp4", "video", make([]byte, 32))
		h.Create(c)
		if rec.Code != http.StatusCreated {
			t.Fatal(rec.Body.String())
		}
	}
}

// TestMultipartTemporaryCleanup 验证文件表单溢出到临时磁盘后，即使视频处理失败也释放临时文件。
func TestMultipartTemporaryCleanup(t *testing.T) {
	req := multipartRequest(t, "clip.mp4", "video", bytes.Repeat([]byte{0}, 1024))
	if err := req.ParseMultipartForm(1); err != nil {
		t.Fatal(err)
	}
	file, err := req.MultipartForm.File["file"][0].Open()
	if err != nil {
		t.Fatal(err)
	}
	disk, ok := file.(*os.File)
	if !ok {
		file.Close()
		t.Fatal("expected disk spill")
	}
	name := disk.Name()
	file.Close()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	NewWithProcessor(t.TempDir(), &failingUploadProcessor{validation: errInvalidVideoMetadata}).Create(c)
	if rec.Code != 400 {
		t.Fatal(rec.Code)
	}
	if _, err := os.Stat(name); !os.IsNotExist(err) {
		t.Fatalf("temporary form retained: %v", err)
	}
}

// TestMissingTools 验证默认处理器回退、缺失工具错误及快启临时文件清理。
func TestMissingTools(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	h := NewWithProcessor(t.TempDir(), nil)
	if _, ok := h.processor.(defaultUploadProcessor); !ok {
		t.Fatal("default processor missing")
	}
	if err := h.processor.ValidateVideo(context.Background(), "missing.mp4"); !errors.Is(err, errVideoToolUnavailable) {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "clip.mp4")
	if err := os.WriteFile(path, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := h.processor.Faststart(context.Background(), path); !errors.Is(err, errVideoToolUnavailable) {
		t.Fatal(err)
	}
	files, _ := os.ReadDir(dir)
	data, _ := os.ReadFile(path)
	if len(files) != 1 || string(data) != "original" {
		t.Fatal("failed processing changed source or leaked temporary file")
	}
}
