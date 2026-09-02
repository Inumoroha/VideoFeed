package interaction

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	applicationinteraction "github.com/inumoroha/video-feed/internal/application/interaction"
	domaininteraction "github.com/inumoroha/video-feed/internal/domain/interaction"
	"github.com/inumoroha/video-feed/internal/middleware"
)

// Handler 处理点赞、收藏和评论相关的 HTTP 请求。
type Handler struct {
	service *applicationinteraction.Service
}

// NewHandler 创建使用指定互动应用服务的 HTTP 处理器。
func NewHandler(service *applicationinteraction.Service) *Handler {
	return &Handler{service: service}
}

// ToggleLike 切换当前用户对视频的点赞状态并返回最新互动统计。
func (h *Handler) ToggleLike(c *gin.Context) {
	if h == nil || h.service == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	userID, ok := userIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid access token"})
		return
	}

	var req toggleActionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	result, err := h.service.ToggleLike(c.Request.Context(), userID, req.VideoID, c.GetHeader("Idempotency-Key"))
	if err != nil {
		writeInteractionError(c, err)
		return
	}
	if result == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	c.JSON(http.StatusOK, toggleActionResponseFromResult(result))
}

// ToggleFavorite 切换当前用户对视频的收藏状态并返回最新互动统计。
func (h *Handler) ToggleFavorite(c *gin.Context) {
	if h == nil || h.service == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	userID, ok := userIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid access token"})
		return
	}

	var req toggleActionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	result, err := h.service.ToggleFavorite(c.Request.Context(), userID, req.VideoID, c.GetHeader("Idempotency-Key"))
	if err != nil {
		writeInteractionError(c, err)
		return
	}
	if result == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	c.JSON(http.StatusOK, toggleActionResponseFromResult(result))
}

// CreateComment 创建评论并返回评论内容及视频评论统计。
func (h *Handler) CreateComment(c *gin.Context) {
	if h == nil || h.service == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	userID, ok := userIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid access token"})
		return
	}

	var req createCommentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	result, err := h.service.CreateComment(c.Request.Context(), userID, req.VideoID, req.Content, c.GetHeader("Idempotency-Key"))
	if err != nil {
		writeInteractionError(c, err)
		return
	}
	if result == nil || result.Comment == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	c.JSON(http.StatusCreated, commentResponseFromResult(result))
}

// ListComments 按游标分页返回视频下的正常评论。
func (h *Handler) ListComments(c *gin.Context) {
	if h == nil || h.service == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	videoID, err := parsePositiveInt64(c.Query("video_id"), domaininteraction.ErrInvalidVideoID)
	if err != nil {
		writeInteractionError(c, err)
		return
	}

	limit, err := parseLimit(c.Query("limit"))
	if err != nil {
		writeInteractionError(c, err)
		return
	}

	result, err := h.service.ListComments(c.Request.Context(), videoID, c.Query("cursor"), limit)
	if err != nil {
		writeInteractionError(c, err)
		return
	}
	if result == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	c.JSON(http.StatusOK, commentListResponseFromResult(result))
}

// DeleteComment 软删除评论，并返回删除后的评论统计。
func (h *Handler) DeleteComment(c *gin.Context) {
	if h == nil || h.service == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	userID, ok := userIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid access token"})
		return
	}
	role := roleFromContext(c)

	commentID, err := parsePositiveInt64(c.Param("commentId"), domaininteraction.ErrInvalidCommentID)
	if err != nil {
		writeInteractionError(c, err)
		return
	}

	result, err := h.service.DeleteComment(c.Request.Context(), commentID, userID, role)
	if err != nil {
		writeInteractionError(c, err)
		return
	}
	if result == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	c.JSON(http.StatusOK, deleteCommentResponse{
		CommentID:    result.CommentID,
		Status:       result.Status,
		CommentCount: result.CommentCount,
	})
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

// roleFromContext 读取认证中间件写入的用户角色。
func roleFromContext(c *gin.Context) string {
	value, exists := c.Get(middleware.ContextUserRoleKey)
	if !exists {
		return ""
	}
	role, _ := value.(string)
	return role
}

// parsePositiveInt64 将查询或路径参数解析为正整数。
func parsePositiveInt64(raw string, fallback error) (int64, error) {
	value, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || value <= 0 {
		return 0, fallback
	}
	return value, nil
}

// parseLimit 解析评论分页大小，空值交由应用服务使用默认值。
func parseLimit(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit <= 0 {
		return 0, domaininteraction.ErrInvalidLimit
	}
	return limit, nil
}

// toggleActionResponseFromResult 将互动应用结果转换为 HTTP 响应。
func toggleActionResponseFromResult(result *applicationinteraction.ToggleActionResult) toggleActionResponse {
	if result == nil {
		return toggleActionResponse{}
	}
	return toggleActionResponse{
		VideoID:       result.VideoID,
		ActionType:    result.ActionType,
		Active:        result.Active,
		LikeCount:     result.LikeCount,
		FavoriteCount: result.FavoriteCount,
	}
}

// commentResponseFromResult 将评论创建结果转换为 HTTP 响应。
func commentResponseFromResult(result *applicationinteraction.CreateCommentResult) commentResponse {
	if result == nil {
		return commentResponse{}
	}
	response := commentResponseFromDomain(result.Comment)
	response.CommentCount = result.CommentCount
	return response
}

// commentListResponseFromResult 将评论分页结果转换为 HTTP 响应。
func commentListResponseFromResult(result *applicationinteraction.CommentListResult) commentListResponse {
	if result == nil {
		return commentListResponse{Items: []commentResponse{}}
	}
	items := make([]commentResponse, 0, len(result.Items))
	for _, item := range result.Items {
		if item == nil {
			continue
		}
		items = append(items, commentResponseFromDomain(item))
	}
	return commentListResponse{
		Items:      items,
		NextCursor: result.NextCursor,
		HasMore:    result.HasMore,
	}
}

// commentResponseFromDomain 将评论领域对象转换为 HTTP 响应。
func commentResponseFromDomain(comment *domaininteraction.Comment) commentResponse {
	if comment == nil {
		return commentResponse{}
	}
	return commentResponse{
		ID:            comment.ID,
		VideoID:       comment.VideoID,
		UserID:        comment.UserID,
		UserNickname:  comment.UserNickname,
		UserAvatarURL: comment.UserAvatarURL,
		Content:       comment.Content,
		CreatedAt:     comment.CreatedAt,
	}
}

// writeInteractionError 将互动领域或应用错误转换为统一 HTTP 错误响应。
func writeInteractionError(c *gin.Context, err error) {
	if isBadRequestError(err) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if errors.Is(err, domaininteraction.ErrVideoNotFound) || errors.Is(err, domaininteraction.ErrCommentNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "resource not found"})
		return
	}
	if errors.Is(err, domaininteraction.ErrCommentPermissionDenied) {
		c.JSON(http.StatusForbidden, gin.H{"error": "comment permission denied"})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
}

// isBadRequestError 判断错误是否来自客户端输入或参数校验。
func isBadRequestError(err error) bool {
	return errors.Is(err, domaininteraction.ErrInvalidUserID) ||
		errors.Is(err, domaininteraction.ErrInvalidVideoID) ||
		errors.Is(err, domaininteraction.ErrInvalidCommentID) ||
		errors.Is(err, domaininteraction.ErrInvalidActionType) ||
		errors.Is(err, domaininteraction.ErrInvalidLimit) ||
		errors.Is(err, domaininteraction.ErrInvalidCursor) ||
		errors.Is(err, domaininteraction.ErrEmptyCommentContent) ||
		errors.Is(err, domaininteraction.ErrCommentContentTooLong) ||
		errors.Is(err, domaininteraction.ErrIdempotencyKeyTooLong)
}
