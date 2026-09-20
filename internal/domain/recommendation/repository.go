package recommendation

import "context"

// Repository 定义推荐候选、兴趣向量和曝光持久化能力。
type Repository interface {
	ListCandidatePool(ctx context.Context, userID int64, limit int) ([]*Candidate, error)
	LoadUserInterestVector(ctx context.Context, userID int64) ([]float64, bool, error)
	LoadVideoVectors(ctx context.Context, videoIDs []int64) (map[int64][]float64, error)
	SaveExposures(ctx context.Context, exposures []*ExposureWrite) ([]*Exposure, error)
}
