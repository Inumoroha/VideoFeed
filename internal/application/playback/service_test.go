package playback

import (
	"context"
	"errors"
	domain "github.com/inumoroha/video-feed/internal/domain/playback"
	"strings"
	"testing"
)

// stubRepository 记录仓储调用并注入配置缺失或故障。
type stubRepository struct {
	calls   []string
	config  *domain.Config
	failure error
	limit   int
	created bool
}

// FindConfig 仅对默认网络返回配置并记录查询顺序。
func (r *stubRepository) FindConfig(_ context.Context, platform, network string) (*domain.Config, error) {
	r.calls = append(r.calls, platform+":"+network)
	if network == domain.NetworkDefault {
		return r.config, r.failure
	}
	return nil, r.failure
}

// ListPreloadVideos 保存服务归一化后的数量并返回指定错误。
func (r *stubRepository) ListPreloadVideos(_ context.Context, _ int64, limit int) ([]*domain.PreloadVideo, error) {
	r.limit = limit
	return []*domain.PreloadVideo{}, r.failure
}

// CreateQoSReport 返回输入报告和指定创建状态或错误。
func (r *stubRepository) CreateQoSReport(_ context.Context, report *domain.QoSReport) (*domain.QoSReport, bool, error) {
	return report, r.created, r.failure
}

// TestConfigFallback 验证逐级查询、默认值、上限修正、复制隔离以及错误不被默认值掩盖。
func TestConfigFallback(t *testing.T) {
	ctx := context.Background()
	repo := &stubRepository{}
	service := New(repo)
	result, err := service.GetConfig(ctx, " Web ", "wifi")
	if err != nil || result.Config.PreloadCount != 3 || result.Config.NetworkType != "WiFi" || strings.Join(repo.calls, ",") != "Web:WiFi,Web:DEFAULT" {
		t.Fatalf("%+v %v %v", result, repo.calls, err)
	}
	repo.config = &domain.Config{PreloadCount: 99}
	result, err = service.GetConfig(ctx, "Web", "4G")
	if err != nil || result.Config.PreloadCount != 10 || result.Config.BufferMs != 1200 || repo.config.BufferMs != 0 {
		t.Fatalf("%+v %v", result, err)
	}
	repo.config.PreloadCount = -1
	result, _ = service.GetConfig(ctx, "Web", "")
	if result.Config.PreloadCount != 3 {
		t.Fatal(result)
	}
	repo.failure = errors.New("offline")
	if _, err = service.GetConfig(ctx, "Web", "5G"); !errors.Is(err, ErrLoadPlaybackFailed) {
		t.Fatal(err)
	}
	if _, err = service.GetConfig(ctx, strings.Repeat("x", 17), ""); !errors.Is(err, domain.ErrInvalidPlatform) {
		t.Fatal(err)
	}
	if _, err = service.GetConfig(ctx, "", strings.Repeat("x", 17)); !errors.Is(err, domain.ErrInvalidNetworkType) {
		t.Fatal(err)
	}
}

// TestPreloadAndQoSFailureRetry 验证预加载边界、失败映射和调用方重试后的报告结果。
func TestPreloadAndQoSFailureRetry(t *testing.T) {
	ctx := context.Background()
	repo := &stubRepository{}
	service := New(repo)
	for input, want := range map[int]int{-1: 3, 0: 3, 1: 1, 11: 10} {
		if _, err := service.ListPreloadVideos(ctx, 0, input); err != nil || repo.limit != want {
			t.Fatalf("%d %v", repo.limit, err)
		}
	}
	if _, err := service.ListPreloadVideos(ctx, -1, 1); !errors.Is(err, domain.ErrInvalidVideoID) {
		t.Fatal(err)
	}
	repo.failure = errors.New("offline")
	if _, err := service.ListPreloadVideos(ctx, 0, 1); !errors.Is(err, ErrLoadPlaybackFailed) {
		t.Fatal(err)
	}
	if _, err := service.CreateQoSReport(ctx, 1, 1, nil, 0, 0, "k"); !errors.Is(err, ErrSaveQoSReportFailed) {
		t.Fatal(err)
	}
	repo.failure = nil
	repo.created = true
	result, err := service.CreateQoSReport(ctx, 1, 1, nil, 0, 0, " k ")
	if err != nil || !result.Created || result.Report.IdempotencyKey != "k" {
		t.Fatalf("%+v %v", result, err)
	}
	repo.created = false
	result, err = service.CreateQoSReport(ctx, 1, 1, nil, 0, 0, "k")
	if err != nil || result.Created {
		t.Fatalf("%+v %v", result, err)
	}
}
