package feed

import (
	"context"
	domainrecommendation "github.com/inumoroha/video-feed/internal/domain/recommendation"
	"net/http"
	"testing"
	"time"
)

// recommendationExposureDecisionAPIResponse 接收曝光决策接口测试的元数据和结果。
type recommendationExposureDecisionAPIResponse struct {
	UserID    int64                                    `json:"user_id"`
	Scene     string                                   `json:"scene"`
	RequestID string                                   `json:"request_id"`
	Decisions []recommendationExposureDecisionResponse `json:"decisions"`
}

// recommendationExposureDecisionResponse 接收单个视频的决策测试字段。
type recommendationExposureDecisionResponse struct {
	VideoID       int64      `json:"video_id"`
	Allowed       bool       `json:"allowed"`
	Reason        string     `json:"reason"`
	LastExposedAt *time.Time `json:"last_exposed_at"`
}

// ListRecentExposures 加锁筛选指定用户和时间窗口内的曝光，返回副本以避免数据竞争。
func (r *memoryRecommendationRepo) ListRecentExposures(ctx context.Context, userID int64, videoIDs []int64, since time.Time) ([]*domainrecommendation.Exposure, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	exposures := make([]*domainrecommendation.Exposure, 0, len(videoIDs))
	for _, videoID := range videoIDs {
		exposure := r.exposures[recommendationExposureKey(userID, videoID)]
		if exposure == nil || exposure.LastExposedAt.Before(since) {
			continue
		}
		exposures = append(exposures, cloneRecommendationExposure(exposure))
	}
	return exposures, nil
}

// TestRecommendationExposureDecisionsAPI 验证曝光写入后的拒绝、未曝光放行、输入去重、元数据和非法视频校验。
func TestRecommendationExposureDecisionsAPI(t *testing.T) {
	router := newRecommendationRouter()

	saveResponse := performJSONRequest(
		router,
		http.MethodPost,
		"/internal/exposures",
		`{"user_id":42,"scene":"recommend","request_id":"req-exp","video_ids":[1]}`,
		"",
	)
	requireStatus(t, saveResponse, http.StatusCreated)

	response := performJSONRequest(
		router,
		http.MethodPost,
		"/internal/exposure-decisions",
		`{"user_id":42,"scene":"recommend","request_id":"req-decide","video_ids":[1,2,2,3]}`,
		"",
	)
	requireStatus(t, response, http.StatusOK)

	var result recommendationExposureDecisionAPIResponse
	decodeJSON(t, response, &result)
	if result.UserID != 42 || result.Scene != "recommend" || result.RequestID != "req-decide" {
		t.Fatalf("unexpected exposure decision metadata: %+v", result)
	}
	if len(result.Decisions) != 3 {
		t.Fatalf("unexpected exposure decision count: %+v", result)
	}
	if result.Decisions[0].VideoID != 1 ||
		result.Decisions[0].Allowed ||
		result.Decisions[0].Reason != domainrecommendation.ExposureDecisionReasonRecentlyExposed ||
		result.Decisions[0].LastExposedAt == nil {
		t.Fatalf("unexpected exposed decision: %+v", result.Decisions[0])
	}
	for _, decision := range result.Decisions[1:] {
		if !decision.Allowed || decision.Reason != domainrecommendation.ExposureDecisionReasonFresh || decision.LastExposedAt != nil {
			t.Fatalf("unexpected fresh decision: %+v", decision)
		}
	}

	badResponse := performJSONRequest(
		router,
		http.MethodPost,
		"/internal/exposure-decisions",
		`{"user_id":42,"scene":"recommend","video_ids":[0]}`,
		"",
	)
	requireStatus(t, badResponse, http.StatusBadRequest)
}
