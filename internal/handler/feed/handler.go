package feed

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	applicationfeed "github.com/inumoroha/video-feed/internal/application/feed"
	domainfeed "github.com/inumoroha/video-feed/internal/domain/feed"

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

// Timeline 查询按发布时间排序的信息流，并返回游标分页结果。
func (h *Handler) Timeline(c *gin.Context) {
	if h == nil || h.service == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	limit, err := parseLimit(c.Query("limit"))
	if err != nil {
		writeFeedError(c, err)
		return
	}

	result, err := h.service.GetTimelineFeed(c.Request.Context(), c.Query("cursor"), limit)
	if err != nil {
		writeFeedError(c, err)
		return
	}

	c.JSON(http.StatusOK, timelineFeedResponseFromResult(result))
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

	result, err := h.service.RefreshTimelineFeed(c.Request.Context(), limit)
	if err != nil {
		writeFeedError(c, err)
		return
	}

	c.JSON(http.StatusOK, timelineFeedResponseFromResult(result))
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

// timelineFeedResponseFromResult 将应用层信息流结果转换为 HTTP 响应，并跳过空条目。
func timelineFeedResponseFromResult(result *applicationfeed.TimelineFeedResult) timelineFeedResponse {
	if result == nil {
		return timelineFeedResponse{Items: []feedItemResponse{}}
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
	return timelineFeedResponse{
		Items:      items,
		NextCursor: result.NextCursor,
		HasMore:    result.HasMore,
	}
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
	return errors.Is(err, domainfeed.ErrInvalidLimit) ||
		errors.Is(err, domainfeed.ErrInvalidCursor)
}
