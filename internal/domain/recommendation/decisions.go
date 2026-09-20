package recommendation

import (
	"strings"
	"time"
)

const RecentExposureWindow = 7 * 24 * time.Hour
const ExposureDecisionReasonFresh = "fresh"
const ExposureDecisionReasonRecentlyExposed = "recently_exposed"

// ExposureDecisionRequest 保存曝光决策的用户、场景、请求标识和去重视频列表。
type ExposureDecisionRequest struct {
	UserID    int64
	Scene     string
	RequestID string
	VideoIDs  []int64
}

// ExposureDecision 保存视频是否允许曝光、判定原因及可选最近曝光时间。
type ExposureDecision struct {
	VideoID       int64
	Allowed       bool
	Reason        string
	LastExposedAt *time.Time
}

// NewExposureDecisionRequest 校验用户和视频标识、规范化场景与请求标识，并按首次出现顺序去重；空列表仍校验元数据。
func NewExposureDecisionRequest(userID int64, scene string, requestID string, videoIDs []int64) (*ExposureDecisionRequest, error) {
	if userID <= 0 {
		return nil, ErrInvalidUserID
	}
	scene = strings.TrimSpace(strings.ToLower(scene))
	requestID = strings.TrimSpace(requestID)
	if scene == "" {
		return nil, ErrEmptyScene
	}
	if len(scene) > MaxSceneLength {
		return nil, ErrSceneTooLong
	}
	if len(requestID) > MaxRequestIDLength {
		return nil, ErrRequestIDTooLong
	}
	deduped := make([]int64, 0, len(videoIDs))
	seen := map[int64]struct{}{}
	for _, videoID := range videoIDs {
		if videoID <= 0 {
			return nil, ErrInvalidVideoID
		}
		if _, exists := seen[videoID]; exists {
			continue
		}
		seen[videoID] = struct{}{}
		deduped = append(deduped, videoID)
	}
	return &ExposureDecisionRequest{
		UserID:    userID,
		Scene:     scene,
		RequestID: requestID,
		VideoIDs:  deduped,
	}, nil
}

// RestoreExposureDecision 复制可选曝光时间并去除原因两端空白，避免共享时间指针。
func RestoreExposureDecision(videoID int64, allowed bool, reason string, lastExposedAt *time.Time) *ExposureDecision {
	var exposedAt *time.Time
	if lastExposedAt != nil {
		value := *lastExposedAt
		exposedAt = &value
	}
	return &ExposureDecision{
		VideoID:       videoID,
		Allowed:       allowed,
		Reason:        strings.TrimSpace(reason),
		LastExposedAt: exposedAt,
	}
}
