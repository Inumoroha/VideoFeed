package video

import (
	"errors"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	applicationvideo "github.com/inumoroha/video-feed/internal/application/video"
	domainvideo "github.com/inumoroha/video-feed/internal/domain/video"
	uploadhandler "github.com/inumoroha/video-feed/internal/handler/upload"
	"github.com/inumoroha/video-feed/internal/middleware"

	"github.com/gin-gonic/gin"
)

const defaultListLimit = 20

// Handler 提供视频查询、发布与删除接口，并在组合发布失败时清理本次上传的文件。
type Handler struct {
	service  *applicationvideo.Service
	uploader *uploadhandler.Handler
}

// NewHandler 创建使用指定视频应用服务的 HTTP 处理器。
func NewHandler(service *applicationvideo.Service) *Handler {
	return &Handler{service: service}
}

// WithUploader 为组合发布接口设置复用现有校验规则的文件处理器。
func (h *Handler) WithUploader(uploader *uploadhandler.Handler) *Handler {
	h.uploader = uploader
	return h
}

// PublishMultipart 在一次认证请求中保存视频和封面并发布作品，失败或幂等复用时删除本次文件。
func (h *Handler) PublishMultipart(c *gin.Context) {
	if h == nil || h.service == nil || h.uploader == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	userID, ok := userIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid access token"})
		return
	}
	uploadhandler.LimitBody(c)
	videoFile, err := c.FormFile("video")
	if c.Request.MultipartForm != nil {
		defer c.Request.MultipartForm.RemoveAll()
	}
	if err != nil {
		writeMultipartFileError(c, err, "video")
		return
	}
	coverFile, err := c.FormFile("cover")
	if err != nil {
		writeMultipartFileError(c, err, "cover")
		return
	}
	video, err := h.uploader.SaveFile(c, videoFile, "video")
	if err != nil {
		uploadhandler.WriteError(c, err)
		return
	}
	var cover *uploadhandler.SavedFile
	retain := false
	defer func() {
		if !retain {
			cleanupSavedFiles(video, cover)
		}
	}()
	cover, err = h.uploader.SaveFile(c, coverFile, "cover")
	if err != nil {
		uploadhandler.WriteError(c, err)
		return
	}
	result, err := h.service.CreatePublished(c.Request.Context(), userID,
		c.PostForm("title"), c.PostForm("description"), video.URL, cover.URL,
		c.GetHeader("Idempotency-Key"))
	if err != nil {
		writeVideoError(c, err)
		return
	}
	if result == nil || result.Video == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	if !result.Created {
		c.JSON(http.StatusOK, videoResponseFromDomain(result.Video))
		return
	}
	retain = true
	c.JSON(http.StatusCreated, videoResponseFromDomain(result.Video))
}

// writeMultipartFileError 将缺失文件和超出请求体上限分别映射为 400 与 413。
func writeMultipartFileError(c *gin.Context, err error, field string) {
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "upload too large"})
		return
	}
	c.JSON(http.StatusBadRequest, gin.H{"error": field + " is required"})
}

// cleanupSavedFiles 逐个删除未发布的上传文件，删除失败写入日志供运维排查。
func cleanupSavedFiles(files ...*uploadhandler.SavedFile) {
	for _, file := range files {
		if file == nil {
			continue
		}
		if err := os.Remove(file.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
			log.Printf("cleanup unpublished upload %s: %v", file.Path, err)
		}
	}
}

// Create 接收视频发布请求，校验认证信息并返回创建或幂等复用结果。
func (h *Handler) Create(c *gin.Context) {
	if h == nil || h.service == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	userID, ok := userIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid access token"})
		return
	}

	var req CreateVideoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	result, err := h.service.CreatePublished(
		c.Request.Context(),
		userID,
		req.Title,
		req.Description,
		req.MediaURL,
		req.CoverURL,
		c.GetHeader("Idempotency-Key"),
	)
	if err != nil {
		writeVideoError(c, err)
		return
	}
	if result == nil || result.Video == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	status := http.StatusCreated
	if !result.Created {
		status = http.StatusOK
	}
	c.JSON(status, videoResponseFromDomain(result.Video))
}

// Get 解析视频 ID 并返回公开视频详情。
func (h *Handler) Get(c *gin.Context) {
	if h == nil || h.service == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	videoID, err := parsePositiveInt64(c.Param("videoId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid video id"})
		return
	}

	video, err := h.service.Get(c.Request.Context(), videoID)
	if err != nil {
		writeVideoError(c, err)
		return
	}
	if video == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	c.JSON(http.StatusOK, videoResponseFromDomain(video))
}

// Delete 校验当前用户身份并删除指定视频。
func (h *Handler) Delete(c *gin.Context) {
	if h == nil || h.service == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	userID, ok := userIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid access token"})
		return
	}

	videoID, err := parsePositiveInt64(c.Param("videoId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid video id"})
		return
	}

	if err := h.service.Delete(c.Request.Context(), userID, videoID); err != nil {
		writeVideoError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

// ListByAuthor 分页返回指定作者公开发布的视频。
func (h *Handler) ListByAuthor(c *gin.Context) {
	if h == nil || h.service == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	authorID, err := parsePositiveInt64(c.Param("userId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	limit, offset, err := parsePagination(c)
	if err != nil {
		writeVideoError(c, err)
		return
	}
	if limit > applicationvideo.MaxListLimit {
		limit = applicationvideo.MaxListLimit
	}

	videos, err := h.service.ListByAuthor(c.Request.Context(), authorID, limit, offset)
	if err != nil {
		writeVideoError(c, err)
		return
	}

	c.JSON(http.StatusOK, videoListResponseFromDomain(videos, limit, offset))
}

// ListMine 分页返回当前认证用户发布的视频。
func (h *Handler) ListMine(c *gin.Context) {
	if h == nil || h.service == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	userID, ok := userIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid access token"})
		return
	}

	limit, offset, err := parsePagination(c)
	if err != nil {
		writeVideoError(c, err)
		return
	}
	if limit > applicationvideo.MaxListLimit {
		limit = applicationvideo.MaxListLimit
	}

	videos, err := h.service.ListByAuthor(c.Request.Context(), userID, limit, offset)
	if err != nil {
		writeVideoError(c, err)
		return
	}

	c.JSON(http.StatusOK, videoListResponseFromDomain(videos, limit, offset))
}

// userIDFromContext 读取认证中间件写入的正数用户 ID。
func userIDFromContext(c *gin.Context) (int64, bool) {
	value, exists := c.Get(middleware.ContextUserIDKey)
	if !exists {
		return 0, false
	}
	userID, ok := value.(int64)
	return userID, ok && userID > 0
}

// parsePositiveInt64 将路径参数解析为正整数。
func parsePositiveInt64(raw string) (int64, error) {
	value, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || value <= 0 {
		return 0, domainvideo.ErrInvalidVideoID
	}
	return value, nil
}

// parsePagination 解析分页查询参数并返回默认分页值。
func parsePagination(c *gin.Context) (int, int, error) {
	limit := defaultListLimit
	offset := 0

	rawLimit := strings.TrimSpace(c.Query("limit"))
	if rawLimit != "" {
		value, err := strconv.Atoi(rawLimit)
		if err != nil || value <= 0 {
			return 0, 0, domainvideo.ErrInvalidLimit
		}
		limit = value
	}

	rawOffset := strings.TrimSpace(c.Query("offset"))
	if rawOffset != "" {
		value, err := strconv.Atoi(rawOffset)
		if err != nil || value < 0 {
			return 0, 0, domainvideo.ErrInvalidOffset
		}
		offset = value
	}

	return limit, offset, nil
}

// videoResponseFromDomain 将视频领域对象转换为 HTTP 响应结构。
func videoResponseFromDomain(video *domainvideo.Video) videoResponse {
	return videoResponse{
		ID:            video.ID,
		AuthorID:      video.AuthorID,
		Title:         video.Title,
		Description:   video.Description,
		MediaURL:      video.MediaURL,
		CoverURL:      video.CoverURL,
		Status:        video.Status,
		LikeCount:     video.LikeCount,
		CommentCount:  video.CommentCount,
		FavoriteCount: video.FavoriteCount,
		PublishedAt:   video.PublishedAt,
		CreatedAt:     video.CreatedAt,
		UpdatedAt:     video.UpdatedAt,
	}
}

// videoListResponseFromDomain 将视频领域对象列表转换为分页 HTTP 响应。
func videoListResponseFromDomain(videos []*domainvideo.Video, limit, offset int) videoListResponse {
	items := make([]videoResponse, 0, len(videos))
	for _, video := range videos {
		if video == nil {
			continue
		}
		items = append(items, videoResponseFromDomain(video))
	}
	return videoListResponse{
		Items:  items,
		Limit:  limit,
		Offset: offset,
	}
}

// writeVideoError 将视频应用层错误转换为统一的 HTTP 响应。
func writeVideoError(c *gin.Context, err error) {
	if isBadRequestError(err) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if errors.Is(err, domainvideo.ErrVideoNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "video not found"})
		return
	}
	if errors.Is(err, domainvideo.ErrVideoPermissionDenied) {
		c.JSON(http.StatusForbidden, gin.H{"error": "video permission denied"})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
}

// isBadRequestError 判断错误是否属于客户端参数错误。
func isBadRequestError(err error) bool {
	return errors.Is(err, domainvideo.ErrInvalidVideoID) ||
		errors.Is(err, domainvideo.ErrInvalidAuthorID) ||
		errors.Is(err, domainvideo.ErrEmptyTitle) ||
		errors.Is(err, domainvideo.ErrTitleTooLong) ||
		errors.Is(err, domainvideo.ErrEmptyMediaURL) ||
		errors.Is(err, domainvideo.ErrEmptyCoverURL) ||
		errors.Is(err, domainvideo.ErrIdempotencyKeyTooLong) ||
		errors.Is(err, domainvideo.ErrInvalidLimit) ||
		errors.Is(err, domainvideo.ErrInvalidOffset) ||
		errors.Is(err, domainvideo.ErrDescriptionTooLong)
}
