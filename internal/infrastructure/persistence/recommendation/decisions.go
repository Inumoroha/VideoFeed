package recommendation

import (
	"context"
	domainrecommendation "github.com/inumoroha/video-feed/internal/domain/recommendation"
	infraexposure "github.com/inumoroha/video-feed/internal/infrastructure/persistence/exposure"
	"time"
)

// ListRecentExposures 查询指定用户及视频在起始时间（含边界）之后的曝光，空列表不访问数据库，查询错误原样返回。
func (r *Repository) ListRecentExposures(ctx context.Context, userID int64, videoIDs []int64, since time.Time) ([]*domainrecommendation.Exposure, error) {
	if len(videoIDs) == 0 {
		return []*domainrecommendation.Exposure{}, nil
	}

	var models []infraexposure.ExposureModel
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND video_id IN ? AND last_exposed_at >= ?", userID, videoIDs, since).
		Find(&models).
		Error
	if err != nil {
		return nil, err
	}
	exposures := make([]*domainrecommendation.Exposure, 0, len(models))
	for _, model := range models {
		exposures = append(exposures, restoreExposure(model))
	}
	return exposures, nil
}
