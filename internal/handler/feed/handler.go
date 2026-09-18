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

// Handler 处理信息流时间线查询和刷新请求。
type Handler struct {
	service *applicationfeed.Service
}

// NewHandler 创建使用指定信息流应用服务的 HTTP 处理器。
func NewHandler(service *applicationfeed.Service) *Handler {
	return &Handler{service: service}
}

// ListFeedItems 读取指定 scene 的 Feed，cursor 和 limit 来自 query 参数。
func (h *Handler) ListFeedItems(c *gin.Context) {
	if h == nil || h.service == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	limit, err := parseLimit(c.Query("limit"))
	if err != nil {
		writeFeedError(c, err)
		return
	}

	viewerID, _ := viewerIDFromContext(c)
	result, err := h.service.GetFeed(c.Request.Context(), applicationfeed.FeedRequest{
		Scene:    domainfeed.Scene(c.Query("scene")),
		Cursor:   c.Query("cursor"),
		Limit:    limit,
		ViewerID: viewerID,
	})
	if err != nil {
		writeFeedError(c, err)
		return
	}

	c.JSON(http.StatusOK, feedItemsResponseFromResult(result))
}

// Timeline 读取时间线 Feed，兼容既有路由调用方。
func (h *Handler) Timeline(c *gin.Context) { h.ListFeedItems(c) }

// Query 通过请求体接收复杂 Feed 查询参数，适合推荐上下文逐步扩展。
func (h *Handler) Query(c *gin.Context) {
	if h == nil || h.service == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	var req feedQueryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	limit, err := parseBodyLimit(req.Limit)
	if err != nil {
		writeFeedError(c, err)
		return
	}

	viewerID, _ := viewerIDFromContext(c)
	result, err := h.service.GetFeed(c.Request.Context(), applicationfeed.FeedRequest{
		Scene:         domainfeed.Scene(req.Scene),
		Cursor:        req.Cursor,
		Limit:         limit,
		ViewerID:      viewerID,
		ClientContext: req.ClientContext,
	})
	if err != nil {
		writeFeedError(c, err)
		return
	}

	c.JSON(http.StatusOK, feedItemsResponseFromResult(result))
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

	result, err := h.service.RefreshFeed(c.Request.Context(), limit)
	if err != nil {
		writeFeedError(c, err)
		return
	}

	c.JSON(http.StatusOK, feedItemsResponseFromResult(result))
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

// parseBodyLimit 校验 JSON 请求体中的 limit，空值交给应用服务使用默认页大小。
func parseBodyLimit(value *int) (int, error) {
	if value == nil {
		return 0, nil
	}
	if *value <= 0 {
		return 0, domainfeed.ErrInvalidLimit
	}
	return *value, nil
}

// feedItemsResponseFromResult 将应用层信息流结果转换为 HTTP 响应，并跳过空条目。
func feedItemsResponseFromResult(result *applicationfeed.FeedResult) feedItemsResponse {
	if result == nil {
		return feedItemsResponse{Items: []feedItemResponse{}}
	}
	items := make([]feedItemResponse, 0, len(result.Items))
	for _, item := range result.Items {
		if item == nil {
			continue
		}
		items = append(items, feedItemResponse{
			VideoID:         item.VideoID,
			AuthorID:        item.AuthorID,
			AuthorNickname:  item.AuthorNickname,
			AuthorAvatarURL: item.AuthorAvatarURL,
			Title:           item.Title,
			Description:     item.Description,
			MediaURL:        item.MediaURL,
			CoverURL:        item.CoverURL,
			LikeCount:       item.LikeCount,
			CommentCount:    item.CommentCount,
			FavoriteCount:   item.FavoriteCount,
			PublishedAt:     item.PublishedAt,
		})
	}
	return feedItemsResponse{
		Scene:      string(result.Scene),
		Items:      items,
		NextCursor: result.NextCursor,
		HasMore:    result.HasMore,
	}
}

// writeFeedError 将信息流领域或应用错误转换为统一的 HTTP 错误响应。
func writeFeedError(c *gin.Context, err error) {
	if errors.Is(err, domainfeed.ErrViewerRequired) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	if isBadRequestError(err) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
}

// isBadRequestError 判断错误是否属于客户端输入或参数校验失败。
func isBadRequestError(err error) bool {
	return errors.Is(err, domainfeed.ErrInvalidLimit) ||
		errors.Is(err, domainfeed.ErrInvalidCursor) ||
		errors.Is(err, domainfeed.ErrUnsupportedScene)
}

// viewerIDFromContext 读取可选登录用户 ID，个性化 Feed 策略可以使用。
func viewerIDFromContext(c *gin.Context) (int64, bool) {
	value, exists := c.Get(middleware.ContextUserIDKey)
	if !exists {
		return 0, false
	}
	userID, ok := value.(int64)
	return userID, ok && userID > 0
}
