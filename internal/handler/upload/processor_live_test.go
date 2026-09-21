package upload

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// requireVideoTools 检查真实媒体工具，未安装时明确跳过集成验证。
func requireVideoTools(t *testing.T) {
	t.Helper()
	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip("未安装真实媒体工具：" + tool)
		}
	}
}

// createVideoFixture 使用真实编码器生成短视频，命令受超时限制，文件由测试目录负责清理。
func createVideoFixture(t *testing.T, extension string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "clip"+extension)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ffmpeg", "-v", "error", "-f", "lavfi", "-i", "color=c=black:s=64x64:r=5", "-t", "0.4", "-an", "-c:v", "libx264", "-pix_fmt", "yuv420p", path)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create fixture: %v %s", err, output)
	}
	return path
}

// mediaAtomOffset 按容器顶层块读取指定媒体元数据位置，避免将压缩数据误认为块名。
func mediaAtomOffset(t *testing.T, data []byte, kind string) int {
	t.Helper()
	for offset := 0; offset+8 <= len(data); {
		size := int(binary.BigEndian.Uint32(data[offset : offset+4]))
		if string(data[offset+4:offset+8]) == kind {
			return offset
		}
		if size < 8 || size > len(data)-offset {
			t.Fatalf("invalid atom at %d", offset)
		}
		offset += size
	}
	t.Fatalf("missing atom %s", kind)
	return -1
}

// TestVideoProcessorLive 验证真实 MP4 与 MOV 的元数据读取和无重编码快启，并在取消及失败时保留原文件且清理临时文件。
func TestVideoProcessorLive(t *testing.T) {
	requireVideoTools(t)
	for _, ext := range []string{".mp4", ".mov"} {
		t.Run(ext, func(t *testing.T) {
			path := createVideoFixture(t, ext)
			original, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if mediaAtomOffset(t, original, "moov") < mediaAtomOffset(t, original, "mdat") {
				t.Fatal("fixture already faststart")
			}
			processor := defaultUploadProcessor{}
			if err := processor.ValidateVideo(context.Background(), path); err != nil {
				t.Fatal(err)
			}
			if err := processor.Faststart(context.Background(), path); err != nil {
				t.Fatal(err)
			}
			rewritten, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if mediaAtomOffset(t, rewritten, "moov") > mediaAtomOffset(t, rewritten, "mdat") {
				t.Fatal("metadata not moved before media")
			}
			if err := processor.ValidateVideo(context.Background(), path); err != nil {
				t.Fatal(err)
			}
			files, _ := os.ReadDir(filepath.Dir(path))
			if len(files) != 1 {
				t.Fatalf("temporary files: %v", files)
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if err := processor.Faststart(ctx, path); !errors.Is(err, errFaststartFailed) {
				t.Fatalf("cancel: %v", err)
			}
			after, _ := os.ReadFile(path)
			files, _ = os.ReadDir(filepath.Dir(path))
			if !bytes.Equal(after, rewritten) || len(files) != 1 {
				t.Fatal("cancel changed source or leaked temporary file")
			}
		})
	}
	path := filepath.Join(t.TempDir(), "invalid.mp4")
	if err := os.WriteFile(path, []byte("invalid"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := probeVideo(context.Background(), path); !errors.Is(err, errInvalidVideoMetadata) {
		t.Fatal(err)
	}
	if err := faststartVideo(context.Background(), path); !errors.Is(err, errFaststartFailed) {
		t.Fatal(err)
	}
	files, _ := os.ReadDir(filepath.Dir(path))
	if len(files) != 1 {
		t.Fatal(files)
	}
}

// TestUploadRealVideoLive 验证默认上传处理器的完整文件写入、元数据校验、快启和响应大小语义。
func TestUploadRealVideoLive(t *testing.T) {
	requireVideoTools(t)
	gin.SetMode(gin.TestMode)
	path := createVideoFixture(t, ".mp4")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = multipartRequest(t, "clip.mp4", "video", data)
	NewHandler(root).Create(c)
	if rec.Code != 201 {
		t.Fatalf("%d: %s", rec.Code, rec.Body.String())
	}
	var response uploadResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(filepath.Join(root, "video", response.Filename))
	if err != nil {
		t.Fatal(err)
	}
	if response.Size != int64(len(data)) || mediaAtomOffset(t, saved, "moov") > mediaAtomOffset(t, saved, "mdat") {
		t.Fatalf("unexpected response: %+v", response)
	}
}
