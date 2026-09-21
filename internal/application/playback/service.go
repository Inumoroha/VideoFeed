package playback

import (
	"context"
	"errors"
	domainplayback "github.com/inumoroha/video-feed/internal/domain/playback"
	"strings"
)

var ErrLoadPlaybackFailed = errors.New("failed to load playback data")
var ErrSaveQoSReportFailed = errors.New("failed to save playback qos report")

// Service 协调播放配置、预加载和质量上报仓储。
type Service struct {
	repo domainplayback.Repository
}

// ConfigResult 承载规范化后的播放配置。
type ConfigResult struct {
	Config *domainplayback.Config
}

// PreloadResult 承载预加载视频资源列表。
type PreloadResult struct {
	Items []*domainplayback.PreloadVideo
}

// QoSReportResult 承载持久化质量报告和是否新建的标记。
type QoSReportResult struct {
	Report  *domainplayback.QoSReport
	Created bool
}

// New 创建使用指定仓储的播放服务。
func New(repo domainplayback.Repository) *Service {
	return &Service{repo: repo}
}

// GetConfig 查询端侧播放配置，配置缺失时返回领域默认值。
func (s *Service) GetConfig(ctx context.Context, platform string, networkType string) (*ConfigResult, error) {
	platform = domainplayback.NormalizePlatform(platform)
	networkType = domainplayback.NormalizeNetworkType(networkType)
	if len(platform) > domainplayback.MaxPlatformLength {
		return nil, domainplayback.ErrInvalidPlatform
	}
	if len(networkType) > domainplayback.MaxNetworkTypeLength {
		return nil, domainplayback.ErrInvalidNetworkType
	}

	config, err := s.repo.FindConfig(ctx, platform, networkType)
	if err != nil {
		return nil, ErrLoadPlaybackFailed
	}
	if config == nil && networkType != domainplayback.NetworkDefault {
		config, err = s.repo.FindConfig(ctx, platform, domainplayback.NetworkDefault)
		if err != nil {
			return nil, ErrLoadPlaybackFailed
		}
	}
	if config == nil {
		config = domainplayback.DefaultConfig(platform, networkType)
	}
	return &ConfigResult{Config: normalizeConfig(config, platform, networkType)}, nil
}

// ListPreloadVideos 查询当前视频后续资源，供端侧预加载。
func (s *Service) ListPreloadVideos(ctx context.Context, currentVideoID int64, limit int) (*PreloadResult, error) {
	if currentVideoID < 0 {
		return nil, domainplayback.ErrInvalidVideoID
	}
	limit = normalizeLimit(limit)

	items, err := s.repo.ListPreloadVideos(ctx, currentVideoID, limit)
	if err != nil {
		return nil, ErrLoadPlaybackFailed
	}
	return &PreloadResult{Items: items}, nil
}

// CreateQoSReport 写入播放质量流水。
func (s *Service) CreateQoSReport(ctx context.Context, userID int64, videoID int64, firstFrameMs *int, stutterCount int, watchMs int, idempotencyKey string) (*QoSReportResult, error) {
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	report, err := domainplayback.NewQoSReport(userID, videoID, firstFrameMs, stutterCount, watchMs, idempotencyKey)
	if err != nil {
		return nil, err
	}

	created, inserted, err := s.repo.CreateQoSReport(ctx, report)
	if err != nil {
		return nil, ErrSaveQoSReportFailed
	}
	return &QoSReportResult{Report: created, Created: inserted}, nil
}

// normalizeConfig 复制配置并修正预加载数量和缓冲时长边界。
func normalizeConfig(config *domainplayback.Config, platform string, networkType string) *domainplayback.Config {
	normalized := *config
	normalized.Platform = domainplayback.NormalizePlatform(normalized.Platform)
	normalized.NetworkType = domainplayback.NormalizeNetworkType(normalized.NetworkType)
	if normalized.Platform == "" {
		normalized.Platform = platform
	}
	if normalized.NetworkType == "" {
		normalized.NetworkType = networkType
	}
	if normalized.PreloadCount <= 0 {
		normalized.PreloadCount = domainplayback.DefaultPreloadCount
	}
	if normalized.PreloadCount > domainplayback.MaxPreloadLimit {
		normalized.PreloadCount = domainplayback.MaxPreloadLimit
	}
	if normalized.BufferMs <= 0 {
		normalized.BufferMs = domainplayback.DefaultBufferMs
	}
	return &normalized
}

// normalizeLimit 将预加载数量限制为默认值或最大允许值。
func normalizeLimit(limit int) int {
	if limit <= 0 {
		return domainplayback.DefaultPreloadCount
	}
	if limit > domainplayback.MaxPreloadLimit {
		return domainplayback.MaxPreloadLimit
	}
	return limit
}
