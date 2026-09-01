package upload

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const maxUploadBytes = 1024 << 20

// Handler 处理文件上传请求，并将文件保存到配置的根目录。
type Handler struct {
	root string
}

// NewHandler 创建使用指定根目录保存上传文件的 Handler。
func NewHandler(root string) *Handler {
	return &Handler{root: root}
}

// Create 接收 multipart 文件和可选类型，保存文件并返回可访问的上传地址。
func (h *Handler) Create(c *gin.Context) {
	if h == nil || strings.TrimSpace(h.root) == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "upload root is not configured"})
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxUploadBytes)

	file, err := c.FormFile("file")
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "upload too large"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
		return
	}

	kind, ok := normalizeKind(c.PostForm("kind"))
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid upload kind"})
		return
	}

	ext := normalizeExtension(file.Filename)

	filename := fmt.Sprintf("%d-%s%s", time.Now().UnixNano(), randomSuffix(), ext)
	targetDir := filepath.Join(h.root, kind)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to prepare upload directory"})
		return
	}

	targetPath := filepath.Join(targetDir, filename)
	if err := c.SaveUploadedFile(file, targetPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save upload"})
		return
	}

	c.JSON(http.StatusCreated, uploadResponse{
		URL:      "/uploads/" + kind + "/" + filename,
		Kind:     kind,
		Filename: filename,
		Size:     file.Size,
	})
}

// normalizeKind 将客户端传入的上传类型限制为系统支持的目录名称。
func normalizeKind(raw string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "video":
		return "video", true
	case "cover":
		return "cover", true
	case "avatar":
		return "avatar", true
	case "":
		return "file", true
	case "file":
		return "file", true
	default:
		return "", false
	}
}

// normalizeExtension 清理客户端文件名扩展名，异常扩展名统一使用二进制后缀。
func normalizeExtension(filename string) string {
	ext := strings.ToLower(filepath.Ext(filepath.Base(filename)))
	if len(ext) <= 1 || len(ext) > 16 {
		return ".bin"
	}
	for _, char := range ext[1:] {
		if (char < 'a' || char > 'z') && (char < '0' || char > '9') {
			return ".bin"
		}
	}
	return ext
}

// randomSuffix 生成随机文件名片段，随机源不可用时回退到时间值。
func randomSuffix() string {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err == nil {
		return hex.EncodeToString(buf)
	}
	return strconv.FormatInt(time.Now().UnixNano(), 36)
}
