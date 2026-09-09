package relation

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	applicationrelation "github.com/inumoroha/video-feed/internal/application/relation"
	domainrelation "github.com/inumoroha/video-feed/internal/domain/relation"
	"github.com/inumoroha/video-feed/internal/middleware"

	"github.com/gin-gonic/gin"
)

// Handler 处理关注、取关、关注列表和粉丝列表 HTTP 请求。
type Handler struct {
	service *applicationrelation.Service
}

// NewHandler 创建使用指定关系应用服务的 HTTP 处理器。
func NewHandler(service *applicationrelation.Service) *Handler {
	return &Handler{service: service}
}

// Follow 处理关注用户接口。
func (h *Handler) Follow(c *gin.Context) {
	h.setFollow(c, true)
}

// Unfollow 处理取消关注用户接口。
func (h *Handler) Unfollow(c *gin.Context) {
	h.setFollow(c, false)
}

// ListFollowing 查询当前用户关注列表。
func (h *Handler) ListFollowing(c *gin.Context) {
	if h == nil || h.service == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	userID, ok := userIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid access token"})
		return
	}

	limit, err := parseLimit(c.Query("limit"))
	if err != nil {
		writeRelationError(c, err)
		return
	}

	result, err := h.service.ListFollowing(c.Request.Context(), userID, c.Query("cursor"), limit)
	if err != nil {
		writeRelationError(c, err)
		return
	}
	if result == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	c.JSON(http.StatusOK, relationListResponseFromResult(result))
}

// ListFollowers 查询当前用户粉丝列表。
func (h *Handler) ListFollowers(c *gin.Context) {
	if h == nil || h.service == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	userID, ok := userIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid access token"})
		return
	}

	limit, err := parseLimit(c.Query("limit"))
	if err != nil {
		writeRelationError(c, err)
		return
	}

	result, err := h.service.ListFollowers(c.Request.Context(), userID, c.Query("cursor"), limit)
	if err != nil {
		writeRelationError(c, err)
		return
	}
	if result == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	c.JSON(http.StatusOK, relationListResponseFromResult(result))
}

// setFollow 根据 active 处理关注或取关请求。
func (h *Handler) setFollow(c *gin.Context, active bool) {
	if h == nil || h.service == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	userID, ok := userIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid access token"})
		return
	}

	targetUserID, err := parsePositiveInt64(c.Param("targetUserId"), domainrelation.ErrInvalidTargetUserID)
	if err != nil {
		writeRelationError(c, err)
		return
	}

	var result *applicationrelation.FollowResult
	if active {
		result, err = h.service.Follow(c.Request.Context(), userID, targetUserID, c.GetHeader("Idempotency-Key"))
	} else {
		result, err = h.service.Unfollow(c.Request.Context(), userID, targetUserID, c.GetHeader("Idempotency-Key"))
	}
	if err != nil {
		writeRelationError(c, err)
		return
	}
	if result == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	c.JSON(http.StatusOK, followResponseFromResult(result))
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
func parsePositiveInt64(raw string, fallback error) (int64, error) {
	value, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || value <= 0 {
		return 0, fallback
	}
	return value, nil
}

// parseLimit 解析关系列表分页大小，空值交由应用服务使用默认值。
func parseLimit(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit <= 0 {
		return 0, domainrelation.ErrInvalidLimit
	}
	return limit, nil
}

// followResponseFromResult 将关注应用结果转换为 HTTP 响应。
func followResponseFromResult(result *applicationrelation.FollowResult) followResponse {
	if result == nil {
		return followResponse{}
	}
	return followResponse{
		UserID:         result.UserID,
		TargetUserID:   result.TargetUserID,
		Status:         result.Status,
		Following:      result.Following,
		FollowingCount: result.FollowingCount,
		FollowerCount:  result.FollowerCount,
	}
}

// relationListResponseFromResult 将关系列表应用结果转换为 HTTP 响应。
func relationListResponseFromResult(result *applicationrelation.ListResult) relationListResponse {
	if result == nil {
		return relationListResponse{Items: []relationUserResponse{}}
	}
	items := make([]relationUserResponse, 0, len(result.Items))
	for _, item := range result.Items {
		if item == nil {
			continue
		}
		items = append(items, relationUserResponse{
			UserID:     item.UserID,
			Nickname:   item.Nickname,
			AvatarURL:  item.AvatarURL,
			Bio:        item.Bio,
			FollowedAt: item.FollowedAt,
		})
	}
	return relationListResponse{
		Items:      items,
		NextCursor: result.NextCursor,
		HasMore:    result.HasMore,
	}
}

// writeRelationError 将关系领域或应用错误转换为统一 HTTP 错误响应。
func writeRelationError(c *gin.Context, err error) {
	if isBadRequestError(err) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if errors.Is(err, domainrelation.ErrTargetUserNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "target user not found"})
		return
	}
	if errors.Is(err, domainrelation.ErrUserNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
}

// isBadRequestError 判断错误是否来自客户端输入或参数校验。
func isBadRequestError(err error) bool {
	return errors.Is(err, domainrelation.ErrInvalidUserID) ||
		errors.Is(err, domainrelation.ErrInvalidTargetUserID) ||
		errors.Is(err, domainrelation.ErrFollowSelfForbidden) ||
		errors.Is(err, domainrelation.ErrInvalidLimit) ||
		errors.Is(err, domainrelation.ErrInvalidCursor) ||
		errors.Is(err, domainrelation.ErrIdempotencyKeyTooLong)
}
