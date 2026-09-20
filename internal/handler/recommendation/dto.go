package recommendation

import "time"

// candidateRequest 接收内部推荐候选查询参数。
type candidateRequest struct {
	UserID    int64  `json:"user_id"`
	Scene     string `json:"scene"`
	RequestID string `json:"request_id"`
	Cursor    string `json:"cursor"`
	Limit     *int   `json:"limit"`
}

// candidateResponse 返回推荐候选和分页元数据。
type candidateResponse struct {
	UserID     int64                   `json:"user_id"`
	Scene      string                  `json:"scene"`
	RequestID  string                  `json:"request_id,omitempty"`
	Candidates []candidateItemResponse `json:"candidates"`
	NextCursor string                  `json:"next_cursor"`
	HasMore    bool                    `json:"has_more"`
}

// candidateItemResponse 返回候选视频及其评分和推荐原因。
type candidateItemResponse struct {
	VideoID        int64     `json:"video_id"`
	AuthorID       int64     `json:"author_id"`
	RankScore      float64   `json:"rank_score"`
	Similarity     float64   `json:"similarity"`
	HotScore       int       `json:"hot_score"`
	FreshnessScore float64   `json:"freshness_score"`
	Reason         string    `json:"reason"`
	PublishedAt    time.Time `json:"published_at"`
}

// exposuresRequest 接收用户、场景和待记录曝光的视频列表。
type exposuresRequest struct {
	UserID    int64   `json:"user_id"`
	Scene     string  `json:"scene"`
	RequestID string  `json:"request_id"`
	VideoIDs  []int64 `json:"video_ids"`
}

// exposuresResponse 返回批量曝光聚合列表。
type exposuresResponse struct {
	Exposures []exposureItemResponse `json:"exposures"`
}

// exposureItemResponse 返回单个视频的曝光聚合信息。
type exposureItemResponse struct {
	UserID         int64     `json:"user_id"`
	VideoID        int64     `json:"video_id"`
	FirstExposedAt time.Time `json:"first_exposed_at"`
	LastExposedAt  time.Time `json:"last_exposed_at"`
	ExposureCount  int       `json:"exposure_count"`
	LastScene      string    `json:"last_scene"`
}
