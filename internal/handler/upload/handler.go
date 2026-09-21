package upload

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const maxUploadBytes = 1024 << 20
const maxVideoBytes = 512 << 20
const maxImageBytes = 20 << 20
const maxVideoDurationSeconds = 10 * 60
const maxVideoDimension = 3840
const sniffBytes = 512

var allowedVideoExt = map[string]struct{}{
	".mp4":  {},
	".mov":  {},
	".webm": {},
}

var allowedImageExt = map[string]struct{}{
	".jpg":  {},
	".jpeg": {},
	".png":  {},
	".webp": {},
}

var allowedVideoMIME = map[string]struct{}{
	"application/octet-stream": {},
	"video/mp4":                {},
	"video/quicktime":          {},
	"video/webm":               {},
}

var allowedImageMIME = map[string]struct{}{
	"image/jpeg": {},
	"image/png":  {},
	"image/webp": {},
}

var allowedVideoCodec = map[string]struct{}{
	"h264": {},
	"h265": {},
	"hevc": {},
	"vp8":  {},
	"vp9":  {},
	"av1":  {},
}

var allowedAudioCodec = map[string]struct{}{
	"aac":    {},
	"mp3":    {},
	"opus":   {},
	"vorbis": {},
}

var errInvalidUploadExtension = errors.New("unsupported upload extension")
var errInvalidUploadMIME = errors.New("unsupported upload content type")
var errUploadTooLarge = errors.New("upload file is too large")
var errVideoTooLong = errors.New("video duration is too long")
var errVideoTooLargeDimension = errors.New("video resolution is too large")
var errUnsupportedVideoCodec = errors.New("video codec is unsupported")
var errInvalidVideoMetadata = errors.New("video metadata is invalid")
var errVideoToolUnavailable = errors.New("video tool is unavailable")
var errFaststartFailed = errors.New("video faststart failed")

// UploadProcessor 负责视频元数据校验和 MP4 faststart 处理。
type UploadProcessor interface {
	// ValidateVideo 校验已保存视频的元数据。
	ValidateVideo(ctx context.Context, path string) error
	// Faststart 对已保存视频执行快启处理并写回文件。
	Faststart(ctx context.Context, path string) error
}

// Handler 处理文件上传请求，并将文件保存到配置的根目录。
type Handler struct {
	root      string
	processor UploadProcessor
}

// NewHandler 创建使用指定根目录保存上传文件的 Handler。
func NewHandler(root string) *Handler {
	return NewWithProcessor(root, defaultUploadProcessor{})
}

// NewWithProcessor 创建可注入视频处理器的上传 Handler，便于测试和替换实现。
func NewWithProcessor(root string, processor UploadProcessor) *Handler {
	if processor == nil {
		processor = defaultUploadProcessor{}
	}
	return &Handler{root: root, processor: processor}
}

// Create 校验并保存上传文件，视频校验及快启处理失败时删除文件，最后返回原始大小和访问地址。
func (h *Handler) Create(c *gin.Context) {
	if h == nil || strings.TrimSpace(h.root) == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "upload root is not configured"})
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxUploadBytes)

	file, err := c.FormFile("file")
	if c.Request.MultipartForm != nil {
		defer c.Request.MultipartForm.RemoveAll()
	}
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

	validation, err := validateUploadFile(file, kind)
	if err != nil {
		writeUploadValidationError(c, err)
		return
	}
	ext := validation.Ext

	filename := fmt.Sprintf("%d-%s%s", time.Now().UnixNano(), randomSuffix(), ext)
	targetDir := filepath.Join(h.root, kind)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to prepare upload directory"})
		return
	}

	targetPath := filepath.Join(targetDir, filename)
	if err := c.SaveUploadedFile(file, targetPath); err != nil {
		_ = os.Remove(targetPath)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save upload"})
		return
	}

	if kind == "video" {
		if err := h.processor.ValidateVideo(c.Request.Context(), targetPath); err != nil {
			_ = os.Remove(targetPath)
			writeUploadProcessingError(c, err)
			return
		}
		if shouldFaststart(validation.Ext) {
			if err := h.processor.Faststart(c.Request.Context(), targetPath); err != nil {
				_ = os.Remove(targetPath)
				writeUploadProcessingError(c, err)
				return
			}
		}
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

// uploadValidation 保存通过校验的扩展名和探测内容类型。
type uploadValidation struct {
	Ext  string
	MIME string
}

// validateUploadFile 按类型校验文件大小、扩展名和内容，普通文件保留原有扩展名清理规则。
func validateUploadFile(file *multipart.FileHeader, kind string) (*uploadValidation, error) {
	if err := validateUploadSize(file.Size, kind); err != nil {
		return nil, err
	}

	ext := strings.ToLower(filepath.Ext(filepath.Base(file.Filename)))
	if ext == "" {
		if kind == "file" {
			ext = ".bin"
		} else {
			return nil, errInvalidUploadExtension
		}
	}

	if err := validateUploadExtension(ext, kind); err != nil {
		return nil, err
	}
	if kind == "file" {
		return &uploadValidation{Ext: normalizeExtension(file.Filename)}, nil
	}

	mimeType, err := sniffUploadMIME(file)
	if err != nil {
		return nil, err
	}
	if err := validateUploadMIME(mimeType, kind); err != nil {
		return nil, err
	}

	return &uploadValidation{Ext: ext, MIME: mimeType}, nil
}

// validateUploadSize 拒绝空文件和超过对应类型大小上限的文件。
func validateUploadSize(size int64, kind string) error {
	if size <= 0 {
		return errInvalidUploadMIME
	}
	if kind == "video" && size > maxVideoBytes {
		return errUploadTooLarge
	}
	if (kind == "cover" || kind == "avatar") && size > maxImageBytes {
		return errUploadTooLarge
	}
	if size > maxUploadBytes {
		return errUploadTooLarge
	}
	return nil
}

// validateUploadExtension 校验视频和图片扩展名白名单，普通文件不受白名单限制。
func validateUploadExtension(ext string, kind string) error {
	switch kind {
	case "video":
		if _, ok := allowedVideoExt[ext]; !ok {
			return errInvalidUploadExtension
		}
	case "cover", "avatar":
		if _, ok := allowedImageExt[ext]; !ok {
			return errInvalidUploadExtension
		}
	}
	return nil
}

// sniffUploadMIME 读取文件开头最多五百一十二字节探测内容类型，并关闭文件句柄。
func sniffUploadMIME(fileHeader *multipart.FileHeader) (string, error) {
	file, err := fileHeader.Open()
	if err != nil {
		return "", errInvalidUploadMIME
	}
	defer file.Close()

	header := make([]byte, sniffBytes)
	n, err := io.ReadFull(file, header)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return "", errInvalidUploadMIME
	}
	if n == 0 {
		return "", errInvalidUploadMIME
	}
	return http.DetectContentType(header[:n]), nil
}

// validateUploadMIME 校验探测到的视频或图片内容类型白名单。
func validateUploadMIME(mimeType string, kind string) error {
	switch kind {
	case "video":
		if _, ok := allowedVideoMIME[mimeType]; !ok {
			return errInvalidUploadMIME
		}
	case "cover", "avatar":
		if _, ok := allowedImageMIME[mimeType]; !ok {
			return errInvalidUploadMIME
		}
	}
	return nil
}

// shouldFaststart 判断扩展名是否需要执行 MP4 或 MOV 快启处理。
func shouldFaststart(ext string) bool {
	ext = strings.ToLower(strings.TrimSpace(ext))
	return ext == ".mp4" || ext == ".mov"
}

// defaultUploadProcessor 使用本机 ffprobe 和 ffmpeg 校验视频并执行无重编码快启处理。
type defaultUploadProcessor struct{}

// ValidateVideo 探测视频元数据并校验时长、分辨率及音视频编码。
func (defaultUploadProcessor) ValidateVideo(ctx context.Context, path string) error {
	metadata, err := probeVideo(ctx, path)
	if err != nil {
		return err
	}
	return validateVideoMetadata(metadata)
}

// Faststart 调用受上下文限制的视频快启处理。
func (defaultUploadProcessor) Faststart(ctx context.Context, path string) error {
	return faststartVideo(ctx, path)
}

// probeResult 接收视频流列表和容器元数据。
type probeResult struct {
	Streams []probeStream `json:"streams"`
	Format  probeFormat   `json:"format"`
}

// probeStream 接收流类型、编码名称和视频尺寸。
type probeStream struct {
	CodecType string `json:"codec_type"`
	CodecName string `json:"codec_name"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
}

// probeFormat 接收容器时长字符串。
type probeFormat struct {
	Duration string `json:"duration"`
}

// probeVideo 在三十秒超时内执行 ffprobe 并解析元数据，缺少工具和无效视频分别返回错误。
func probeVideo(ctx context.Context, path string) (*probeResult, error) {
	probeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(
		probeCtx,
		"ffprobe",
		"-v",
		"error",
		"-show_entries",
		"stream=codec_type,codec_name,width,height:format=duration",
		"-of",
		"json",
		path,
	)
	output, err := cmd.Output()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, errVideoToolUnavailable
		}
		return nil, errInvalidVideoMetadata
	}

	var result probeResult
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, errInvalidVideoMetadata
	}
	return &result, nil
}

// validateVideoMetadata 检查有限正时长、视频流、分辨率及音视频编码白名单。
func validateVideoMetadata(metadata *probeResult) error {
	if metadata == nil {
		return errInvalidVideoMetadata
	}

	duration, err := strconv.ParseFloat(strings.TrimSpace(metadata.Format.Duration), 64)
	if err != nil || math.IsNaN(duration) || math.IsInf(duration, 0) || duration <= 0 {
		return errInvalidVideoMetadata
	}
	if duration > maxVideoDurationSeconds {
		return errVideoTooLong
	}

	var hasVideo bool
	for _, stream := range metadata.Streams {
		codecType := strings.ToLower(strings.TrimSpace(stream.CodecType))
		codecName := strings.ToLower(strings.TrimSpace(stream.CodecName))
		switch codecType {
		case "video":
			hasVideo = true
			if stream.Width <= 0 || stream.Height <= 0 {
				return errInvalidVideoMetadata
			}
			if stream.Width > maxVideoDimension || stream.Height > maxVideoDimension {
				return errVideoTooLargeDimension
			}
			if _, ok := allowedVideoCodec[codecName]; !ok {
				return errUnsupportedVideoCodec
			}
		case "audio":
			if codecName != "" {
				if _, ok := allowedAudioCodec[codecName]; !ok {
					return errUnsupportedVideoCodec
				}
			}
		}
	}
	if !hasVideo {
		return errInvalidVideoMetadata
	}
	return nil
}

// faststartVideo 在两分钟内无重编码重排媒体元数据，成功后替换原文件，始终清理临时文件。
func faststartVideo(ctx context.Context, path string) error {
	tmp, err := createFaststartTempFile(path)
	if err != nil {
		return errFaststartFailed
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(tmpPath)

	format := "mp4"
	if strings.EqualFold(filepath.Ext(path), ".mov") {
		format = "mov"
	}
	ffmpegCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(
		ffmpegCtx,
		"ffmpeg",
		"-y",
		"-i",
		path,
		"-map",
		"0",
		"-c",
		"copy",
		"-movflags",
		"+faststart",
		"-f",
		format,
		tmpPath,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return errVideoToolUnavailable
		}
		return fmt.Errorf("%w: %s", errFaststartFailed, strings.TrimSpace(stderr.String()))
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return errFaststartFailed
	}
	return nil
}

// writeUploadValidationError 将上传文件校验失败映射为参数错误。
func writeUploadValidationError(c *gin.Context, err error) {
	c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
}

// writeUploadProcessingError 工具缺失或快启失败返回服务错误，其余视频校验失败返回参数错误。
func writeUploadProcessingError(c *gin.Context, err error) {
	if errors.Is(err, errVideoToolUnavailable) || errors.Is(err, errFaststartFailed) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
}

// createFaststartTempFile 在原文件目录创建带 MP4 后缀的唯一临时文件，返回由调用方关闭和清理的句柄。
func createFaststartTempFile(path string) (*os.File, error) {
	targetDir := filepath.Dir(path)
	return os.CreateTemp(targetDir, "*.faststart.mp4")
}
