package playback

import "time"

// playbackConfigResponse 定义播放配置响应字段。
type playbackConfigResponse struct {
	ID           int64     `json:"id"`
	Platform     string    `json:"platform"`
	NetworkType  string    `json:"network_type"`
	PreloadCount int       `json:"preload_count"`
	BufferMs     int       `json:"buffer_ms"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// preloadVideoResponse 定义单个预加载资源的响应字段。
type preloadVideoResponse struct {
	VideoID  int64  `json:"video_id"`
	MediaURL string `json:"media_url"`
	CoverURL string `json:"cover_url"`
}

// preloadVideosResponse 定义预加载资源列表响应。
type preloadVideosResponse struct {
	Items []preloadVideoResponse `json:"items"`
}

// createQoSReportRequest 接收播放质量指标和内部调用用户标识。
type createQoSReportRequest struct {
	UserID       int64 `json:"user_id,omitempty"`
	VideoID      int64 `json:"video_id"`
	FirstFrameMs *int  `json:"first_frame_ms,omitempty"`
	StutterCount int   `json:"stutter_count"`
	WatchMs      int   `json:"watch_ms"`
}

// qosReportResponse 定义质量报告响应，首帧未采集时省略。
type qosReportResponse struct {
	ID           int64     `json:"id"`
	UserID       int64     `json:"user_id"`
	VideoID      int64     `json:"video_id"`
	FirstFrameMs *int      `json:"first_frame_ms,omitempty"`
	StutterCount int       `json:"stutter_count"`
	WatchMs      int       `json:"watch_ms"`
	CreatedAt    time.Time `json:"created_at"`
}
