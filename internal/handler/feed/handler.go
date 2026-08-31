package feed

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	applicationfeed "github.com/inumoroha/video-feed/internal/application/feed"
	domainfeed "github.com/inumoroha/video-feed/internal/domain/feed"
	"github.com/inumoroha/video-feed/internal/middleware"

	"github.com/gin-gonic/gin"
)

// Handler 处理信息流查询、刷新和观看事件上报请求。
type Handler struct {
	service *applicationfeed.Service
}

// NewHandler 创建使用指定信息流应用服务的 HTTP 处理器。
func NewHandler(service *applicationfeed.Service) *Handler {
	return &Handler{service: service}
}

// Time 查询按发布时间排序的信息流，并返回游标分页结果。
func (h *Handler) Time(c *gin.Context) {
	if h == nil || h.service == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	limit, err := parseLimit(c.Query("limit"))
	if err != nil {
		writeFeedError(c, err)
		return
	}

	result, err := h.service.GetTimeFeed(c.Request.Context(), c.Query("cursor"), limit)
	if err != nil {
		writeFeedError(c, err)
		return
	}

	c.JSON(http.StatusOK, timeFeedResponseFromResult(result))
}

// Refresh 从信息流起点重新查询数据，用于刷新首页信息流。
func (h *Handler) Refresh(c *gin.Context) {
	if h == nil || h.service == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	limit, err := parseLimit(c.Query("limit"))
	if err != nil {
		writeFeedError(c, err)
		return
	}

	result, err := h.service.RefreshTimeFeed(c.Request.Context(), limit)
	if err != nil {
		writeFeedError(c, err)
		return
	}

	c.JSON(http.StatusOK, timeFeedResponseFromResult(result))
}

// ReportViewEvent 接收访客或登录用户的观看事件，并处理幂等重试。
func (h *Handler) ReportViewEvent(c *gin.Context) {
	if h == nil || h.service == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	var req viewEventRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	result, err := h.service.ReportViewEvent(
		c.Request.Context(),
		userIDFromContext(c),
		req.VisitorID,
		req.VideoID,
		req.EventType,
		req.WatchMS,
		c.GetHeader("Idempotency-Key"),
	)
	if err != nil {
		writeFeedError(c, err)
		return
	}
	if result == nil || result.Event == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	status := http.StatusCreated
	if !result.Created {
		status = http.StatusOK
	}
	c.JSON(status, viewEventResponseFromDomain(result.Event))
}

// parseLimit 解析信息流分页数量，空值交由应用服务使用默认值。
func parseLimit(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}

	limit, err := strconv.Atoi(raw)
	if err != nil || limit <= 0 {
		return 0, domainfeed.ErrInvalidLimit
	}
	return limit, nil
}

// timeFeedResponseFromResult 将应用层信息流结果转换为 HTTP 响应，并跳过空条目。
func timeFeedResponseFromResult(result *applicationfeed.TimeFeedResult) timeFeedResponse {
	if result == nil {
		return timeFeedResponse{Items: []feedItemResponse{}}
	}
	items := make([]feedItemResponse, 0, len(result.Items))
	for _, item := range result.Items {
		if item == nil {
			continue
		}
		items = append(items, feedItemResponse{
			VideoID:       item.VideoID,
			AuthorID:      item.AuthorID,
			Title:         item.Title,
			MediaURL:      item.MediaURL,
			CoverURL:      item.CoverURL,
			LikeCount:     item.LikeCount,
			CommentCount:  item.CommentCount,
			FavoriteCount: item.FavoriteCount,
			PublishedAt:   item.PublishedAt,
		})
	}
	return timeFeedResponse{
		Items:      items,
		NextCursor: result.NextCursor,
		HasMore:    result.HasMore,
	}
}

// viewEventResponseFromDomain 将领域观看事件转换为 HTTP 响应。
func viewEventResponseFromDomain(event *domainfeed.ViewEvent) viewEventResponse {
	if event == nil {
		return viewEventResponse{}
	}
	return viewEventResponse{
		ID:        event.ID,
		VisitorID: event.VisitorID,
		VideoID:   event.VideoID,
		EventType: event.EventType,
		WatchMS:   event.WatchMS,
		CreatedAt: event.CreatedAt,
	}
}

// userIDFromContext 读取认证中间件写入的用户 ID，未登录时返回 nil。
func userIDFromContext(c *gin.Context) *int64 {
	value, exists := c.Get(middleware.ContextUserIDKey)
	if !exists {
		return nil
	}
	userID, ok := value.(int64)
	if !ok || userID <= 0 {
		return nil
	}
	return &userID
}

// writeFeedError 将信息流领域或应用错误转换为统一的 HTTP 错误响应。
func writeFeedError(c *gin.Context, err error) {
	if isBadRequestError(err) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
}

// isBadRequestError 判断错误是否属于客户端输入或参数校验失败。
func isBadRequestError(err error) bool {
	return errors.Is(err, domainfeed.ErrInvalidVideoID) ||
		errors.Is(err, domainfeed.ErrInvalidUserID) ||
		errors.Is(err, domainfeed.ErrInvalidLimit) ||
		errors.Is(err, domainfeed.ErrInvalidCursor) ||
		errors.Is(err, domainfeed.ErrMissingActor) ||
		errors.Is(err, domainfeed.ErrEmptyEventType) ||
		errors.Is(err, domainfeed.ErrInvalidEventType) ||
		errors.Is(err, domainfeed.ErrInvalidWatchMS) ||
		errors.Is(err, domainfeed.ErrVisitorIDTooLong) ||
		errors.Is(err, domainfeed.ErrIdempotencyKeyTooLong)
}
