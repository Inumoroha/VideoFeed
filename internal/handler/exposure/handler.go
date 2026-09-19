package exposure

import (
	"errors"
	applicationexposure "github.com/inumoroha/video-feed/internal/application/exposure"
	domainexposure "github.com/inumoroha/video-feed/internal/domain/exposure"
	middleware "github.com/inumoroha/video-feed/internal/middleware"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Handler 将观看行为 HTTP 请求交给曝光应用服务。
type Handler struct {
	service *applicationexposure.Service
}

// NewHandler 注入曝光应用服务。
func NewHandler(service *applicationexposure.Service) *Handler {
	return &Handler{service: service}
}

// CreateViewEvent 处理视频曝光和观看行为上报。
func (h *Handler) CreateViewEvent(c *gin.Context) {
	userID, ok := userIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid access token"})
		return
	}

	var req createViewEventRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	result, err := h.service.RecordViewEvent(
		c.Request.Context(),
		userID,
		req.VideoID,
		req.Scene,
		req.RequestID,
		req.EventType,
		req.WatchMs,
		req.Completed,
	)
	if err != nil {
		writeExposureError(c, err)
		return
	}

	c.JSON(http.StatusCreated, responseFromResult(result))
}

// responseFromResult 将保存结果转换为响应，只有曝光聚合存在时包含该字段。
func responseFromResult(result *applicationexposure.RecordViewEventResult) createViewEventResponse {
	response := createViewEventResponse{
		Event: viewEventResponse{
			ID:        result.Event.ID,
			UserID:    result.Event.UserID,
			VideoID:   result.Event.VideoID,
			Scene:     result.Event.Scene,
			RequestID: result.Event.RequestID,
			EventType: result.Event.EventType,
			WatchMs:   result.Event.WatchMs,
			Completed: result.Event.Completed,
			CreatedAt: result.Event.CreatedAt,
		},
	}
	if result.Exposure != nil {
		response.Exposure = &exposureResponse{
			UserID:         result.Exposure.UserID,
			VideoID:        result.Exposure.VideoID,
			FirstExposedAt: result.Exposure.FirstExposedAt,
			LastExposedAt:  result.Exposure.LastExposedAt,
			ExposureCount:  result.Exposure.ExposureCount,
			LastScene:      result.Exposure.LastScene,
		}
	}
	return response
}

// userIDFromContext 读取认证中间件写入的正整数用户编号。
func userIDFromContext(c *gin.Context) (int64, bool) {
	value, exists := c.Get(middleware.ContextUserIDKey)
	if !exists {
		return 0, false
	}
	userID, ok := value.(int64)
	return userID, ok && userID > 0
}

// writeExposureError 将参数、资源和内部错误映射为对应 HTTP 响应。
func writeExposureError(c *gin.Context, err error) {
	if isBadRequestError(err) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if errors.Is(err, domainexposure.ErrVideoNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "video not found"})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
}

// isBadRequestError 识别观看上报领域中的参数错误。
func isBadRequestError(err error) bool {
	return errors.Is(err, domainexposure.ErrInvalidUserID) ||
		errors.Is(err, domainexposure.ErrInvalidVideoID) ||
		errors.Is(err, domainexposure.ErrEmptyScene) ||
		errors.Is(err, domainexposure.ErrSceneTooLong) ||
		errors.Is(err, domainexposure.ErrInvalidEventType) ||
		errors.Is(err, domainexposure.ErrRequestIDTooLong) ||
		errors.Is(err, domainexposure.ErrWatchMsNegative)
}
