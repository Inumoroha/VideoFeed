package recommendation

import (
	"context"
	"errors"
	domainrecommendation "github.com/inumoroha/video-feed/internal/domain/recommendation"
)

// ErrLoadExposureDecisionsFailed 表示读取曝光决策失败。
var ErrLoadExposureDecisionsFailed = errors.New("failed to load exposure decisions")

// ExposureDecisionInput 保存曝光决策查询的原始输入。
type ExposureDecisionInput struct {
	UserID    int64
	Scene     string
	RequestID string
	VideoIDs  []int64
}

// ExposureDecisionResult 保存规范化后的请求信息及有序视频决策。
type ExposureDecisionResult struct {
	UserID    int64
	Scene     string
	RequestID string
	Decisions []*domainrecommendation.ExposureDecision
}

// DecideExposures 只读查询用户近七天曝光并按去重顺序返回决策；空列表跳过仓储，查询失败转换错误且不自动重试。
func (s *Service) DecideExposures(ctx context.Context, input ExposureDecisionInput) (*ExposureDecisionResult, error) {
	req, err := domainrecommendation.NewExposureDecisionRequest(input.UserID, input.Scene, input.RequestID, input.VideoIDs)
	if err != nil {
		return nil, err
	}
	if len(req.VideoIDs) == 0 {
		return &ExposureDecisionResult{
			UserID:    req.UserID,
			Scene:     req.Scene,
			RequestID: req.RequestID,
			Decisions: []*domainrecommendation.ExposureDecision{},
		}, nil
	}

	exposures, err := s.repo.ListRecentExposures(ctx, req.UserID, req.VideoIDs, s.now().Add(-domainrecommendation.RecentExposureWindow))
	if err != nil {
		return nil, ErrLoadExposureDecisionsFailed
	}
	exposureByVideoID := make(map[int64]*domainrecommendation.Exposure, len(exposures))
	for _, exposure := range exposures {
		if exposure != nil {
			exposureByVideoID[exposure.VideoID] = exposure
		}
	}

	decisions := make([]*domainrecommendation.ExposureDecision, 0, len(req.VideoIDs))
	for _, videoID := range req.VideoIDs {
		if exposure := exposureByVideoID[videoID]; exposure != nil {
			decisions = append(decisions, domainrecommendation.RestoreExposureDecision(
				videoID,
				false,
				domainrecommendation.ExposureDecisionReasonRecentlyExposed,
				&exposure.LastExposedAt,
			))
			continue
		}
		decisions = append(decisions, domainrecommendation.RestoreExposureDecision(
			videoID,
			true,
			domainrecommendation.ExposureDecisionReasonFresh,
			nil,
		))
	}
	return &ExposureDecisionResult{
		UserID:    req.UserID,
		Scene:     req.Scene,
		RequestID: req.RequestID,
		Decisions: decisions,
	}, nil
}
