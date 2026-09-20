package recommendation

import (
	"github.com/gin-gonic/gin"
	applicationrecommendation "github.com/inumoroha/video-feed/internal/application/recommendation"
	"net/http"
	"time"
)

// exposureDecisionsRequest 接收曝光决策的用户、场景、请求标识和视频列表。
type exposureDecisionsRequest struct {
	UserID    int64   `json:"user_id"`
	Scene     string  `json:"scene"`
	RequestID string  `json:"request_id"`
	VideoIDs  []int64 `json:"video_ids"`
}

// exposureDecisionsResponse 返回请求元数据和去重后的曝光决策列表。
type exposureDecisionsResponse struct {
	UserID    int64                          `json:"user_id"`
	Scene     string                         `json:"scene"`
	RequestID string                         `json:"request_id,omitempty"`
	Decisions []exposureDecisionItemResponse `json:"decisions"`
}

// exposureDecisionItemResponse 返回单个视频的允许状态、原因和可选最近曝光时间。
type exposureDecisionItemResponse struct {
	VideoID       int64      `json:"video_id"`
	Allowed       bool       `json:"allowed"`
	Reason        string     `json:"reason"`
	LastExposedAt *time.Time `json:"last_exposed_at,omitempty"`
}

// DecideExposures 绑定曝光决策请求并返回查询结果，绑定或校验失败返回 400，仓储失败返回 500。
func (h *Handler) DecideExposures(c *gin.Context) {
	var req exposureDecisionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	result, err := h.service.DecideExposures(c.Request.Context(), applicationrecommendation.ExposureDecisionInput{
		UserID:    req.UserID,
		Scene:     req.Scene,
		RequestID: req.RequestID,
		VideoIDs:  req.VideoIDs,
	})
	if err != nil {
		writeRecommendationError(c, err)
		return
	}

	c.JSON(http.StatusOK, exposureDecisionsResponseFromResult(result))
}

// exposureDecisionsResponseFromResult 将有序曝光决策转换为响应，空请求标识和未曝光时间由序列化省略。
func exposureDecisionsResponseFromResult(result *applicationrecommendation.ExposureDecisionResult) exposureDecisionsResponse {
	items := make([]exposureDecisionItemResponse, 0, len(result.Decisions))
	for _, decision := range result.Decisions {
		items = append(items, exposureDecisionItemResponse{
			VideoID:       decision.VideoID,
			Allowed:       decision.Allowed,
			Reason:        decision.Reason,
			LastExposedAt: decision.LastExposedAt,
		})
	}
	return exposureDecisionsResponse{
		UserID:    result.UserID,
		Scene:     result.Scene,
		RequestID: result.RequestID,
		Decisions: items,
	}
}
