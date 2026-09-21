package message

import (
	"errors"
	applicationmessage "github.com/inumoroha/video-feed/internal/application/message"
	domainmessage "github.com/inumoroha/video-feed/internal/domain/message"
	interfaceshttpmiddleware "github.com/inumoroha/video-feed/internal/middleware"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// Handler 将消息 HTTP 请求交给应用服务并转换响应。
type Handler struct {
	service *applicationmessage.Service
}

// New 注入消息应用服务。
func New(service *applicationmessage.Service) *Handler {
	return &Handler{service: service}
}

// List 查询当前登录用户的消息列表。
func (h *Handler) List(c *gin.Context) {
	userID, ok := userIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid access token"})
		return
	}

	limit, err := parseLimit(c.Query("limit"))
	if err != nil {
		writeMessageError(c, err)
		return
	}

	result, err := h.service.List(c.Request.Context(), userID, c.Query("cursor"), limit)
	if err != nil {
		writeMessageError(c, err)
		return
	}
	c.JSON(http.StatusOK, listResponseFromResult(result))
}

// CountUnread 查询当前登录用户未读消息数。
func (h *Handler) CountUnread(c *gin.Context) {
	userID, ok := userIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid access token"})
		return
	}

	stat, err := h.service.CountUnread(c.Request.Context(), userID)
	if err != nil {
		writeMessageError(c, err)
		return
	}
	c.JSON(http.StatusOK, unreadStatResponse{UnreadCount: stat.UnreadCount})
}

// MarkRead 将当前登录用户的指定消息标记为已读。
func (h *Handler) MarkRead(c *gin.Context) {
	userID, ok := userIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid access token"})
		return
	}

	var req markReadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	result, err := h.service.MarkRead(c.Request.Context(), userID, req.MessageIDs)
	if err != nil {
		writeMessageError(c, err)
		return
	}
	c.JSON(http.StatusOK, markReadResponse{UpdatedCount: result.UpdatedCount})
}

// Create 供内部事件链路生成用户消息。
func (h *Handler) Create(c *gin.Context) {
	var req createMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	result, err := h.service.CreateFromEvent(
		c.Request.Context(),
		req.UserID,
		req.Type,
		req.Title,
		req.Content,
		req.EventID,
		c.GetHeader("Idempotency-Key"),
	)
	if err != nil {
		writeMessageError(c, err)
		return
	}

	status := http.StatusCreated
	if !result.Created {
		status = http.StatusOK
	}
	c.JSON(status, responseFromDomain(result.Message))
}

// userIDFromContext 读取并校验认证上下文中的正整数用户标识。
func userIDFromContext(c *gin.Context) (int64, bool) {
	value, exists := c.Get(interfaceshttpmiddleware.ContextUserIDKey)
	if !exists {
		return 0, false
	}
	userID, ok := value.(int64)
	return userID, ok && userID > 0
}

// parseLimit 解析分页数量，空值使用默认值，非正整数返回参数错误。
func parseLimit(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit <= 0 {
		return 0, domainmessage.ErrInvalidLimit
	}
	return limit, nil
}

// listResponseFromResult 将领域消息页转换为接口响应，空列表保持数组形式。
func listResponseFromResult(result *applicationmessage.ListResult) messageListResponse {
	items := make([]messageResponse, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, responseFromDomain(item))
	}
	return messageListResponse{
		Items:      items,
		NextCursor: result.NextCursor,
		HasMore:    result.HasMore,
	}
}

// responseFromDomain 将领域消息转换为接口字段。
func responseFromDomain(message *domainmessage.Message) messageResponse {
	return messageResponse{
		ID:        message.ID,
		UserID:    message.UserID,
		Type:      message.Type,
		Title:     message.Title,
		Content:   message.Content,
		EventID:   message.EventID,
		IsRead:    message.IsRead,
		CreatedAt: message.CreatedAt,
		ReadAt:    message.ReadAt,
	}
}

// writeMessageError 将参数错误映射为 400，其余故障返回统一 500。
func writeMessageError(c *gin.Context, err error) {
	if isBadRequestError(err) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
}

// isBadRequestError 判断错误是否属于消息参数校验失败。
func isBadRequestError(err error) bool {
	return errors.Is(err, domainmessage.ErrInvalidUserID) ||
		errors.Is(err, domainmessage.ErrInvalidMessageID) ||
		errors.Is(err, domainmessage.ErrInvalidLimit) ||
		errors.Is(err, domainmessage.ErrInvalidCursor) ||
		errors.Is(err, domainmessage.ErrInvalidMessageType) ||
		errors.Is(err, domainmessage.ErrEmptyTitle) ||
		errors.Is(err, domainmessage.ErrTitleTooLong) ||
		errors.Is(err, domainmessage.ErrEmptyContent) ||
		errors.Is(err, domainmessage.ErrContentTooLong) ||
		errors.Is(err, domainmessage.ErrEventIDTooLong) ||
		errors.Is(err, domainmessage.ErrIdempotencyKeyTooLong)
}
