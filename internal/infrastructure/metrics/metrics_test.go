package inframetrics

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// scrapeMetrics 通过包内处理器读取指标文本。
func scrapeMetrics(t *testing.T) string {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	Handler().ServeHTTP(recorder, request)
	body, err := io.ReadAll(recorder.Result().Body)
	if err != nil {
		t.Fatalf("read metrics body: %v", err)
	}
	return string(body)
}

// metricValue 读取完整样本行的数值；样本不存在时返回零。
func metricValue(t *testing.T, body string, sample string) float64 {
	t.Helper()
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, sample+" ") {
			continue
		}
		value, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimPrefix(line, sample+" ")), 64)
		if err != nil {
			t.Fatalf("parse sample %q: %v", line, err)
		}
		return value
	}
	return 0
}

// TestMetricsEndpointExposesRegisteredCollectors 验证 /metrics 暴露已注册的仪表，且使用 videofeed 命名空间。
func TestMetricsEndpointExposesRegisteredCollectors(t *testing.T) {
	// 带标签的仪表在首次使用前不会出现在输出中，先触发全部采集器。
	ObserveFeed("timeline", time.Millisecond, 1, nil)
	ObserveCacheRead("page", 1, 1, nil)
	ObserveCacheWrite("page", 1, nil)
	ObserveUpload("video", time.Millisecond, nil)
	ObserveVideoProcessing("probe", time.Millisecond, nil)
	ObserveWorkerJob("video_fanout", time.Millisecond, nil)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(HTTPMiddleware())
	engine.GET("/health", func(c *gin.Context) { c.Status(http.StatusOK) })
	engine.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/health", nil))

	body := scrapeMetrics(t)

	for _, name := range []string{
		"videofeed_http_requests_total",
		"videofeed_http_request_duration_seconds",
		"videofeed_feed_requests_total",
		"videofeed_feed_cache_requests_total",
		"videofeed_video_upload_total",
		"videofeed_video_processing_duration_seconds",
		"videofeed_worker_jobs_total",
	} {
		if !strings.Contains(body, name) {
			t.Fatalf("metrics output missing %q", name)
		}
	}
	if strings.Contains(body, "gcfeed_") {
		t.Fatal("metrics output still uses the source namespace")
	}
}

// TestObserveFeedRecordsResultAndItems 验证 Feed 观测按结果计数，成功时累计条目数、记录耗时且不污染未知场景。
func TestObserveFeedRecordsResultAndItems(t *testing.T) {
	const successSample = `videofeed_feed_requests_total{result="success",scene="timeline"}`
	const errorSample = `videofeed_feed_requests_total{result="error",scene="timeline"}`
	const unknownSample = `videofeed_feed_requests_total{result="success",scene="unknown"}`
	const itemsSample = `videofeed_feed_items_total{scene="timeline"}`
	const durationSample = `videofeed_feed_request_duration_seconds_sum{result="success",scene="timeline"}`

	before := scrapeMetrics(t)
	beforeSuccess := metricValue(t, before, successSample)
	beforeError := metricValue(t, before, errorSample)
	beforeUnknown := metricValue(t, before, unknownSample)
	beforeItems := metricValue(t, before, itemsSample)
	beforeDuration := metricValue(t, before, durationSample)

	ObserveFeed("timeline", 12*time.Millisecond, 3, nil)
	ObserveFeed("timeline", 8*time.Millisecond, 0, errors.New("boom"))
	ObserveFeed("", time.Millisecond, 0, nil)

	after := scrapeMetrics(t)
	if got := metricValue(t, after, successSample) - beforeSuccess; got != 1 {
		t.Fatalf("success delta = %v, want 1", got)
	}
	if got := metricValue(t, after, errorSample) - beforeError; got != 1 {
		t.Fatalf("error delta = %v, want 1", got)
	}
	if got := metricValue(t, after, unknownSample) - beforeUnknown; got != 1 {
		t.Fatalf("unknown scene delta = %v, want 1", got)
	}
	if got := metricValue(t, after, itemsSample) - beforeItems; got != 3 {
		t.Fatalf("items delta = %v, want 3", got)
	}
	if got := metricValue(t, after, durationSample) - beforeDuration; got <= 0.011 || got >= 0.013 {
		t.Fatalf("duration delta = %v, want about 0.012", got)
	}
}

// TestObserveCacheReadSplitsHitAndMiss 验证批量缓存读取按命中与未命中分别计数，错误只计错误。
func TestObserveCacheReadSplitsHitAndMiss(t *testing.T) {
	const hitSample = `videofeed_feed_cache_requests_total{area="card",result="hit"}`
	const missSample = `videofeed_feed_cache_requests_total{area="card",result="miss"}`
	const errorSample = `videofeed_feed_cache_requests_total{area="card",result="error"}`

	before := scrapeMetrics(t)
	beforeHit := metricValue(t, before, hitSample)
	beforeMiss := metricValue(t, before, missSample)
	beforeError := metricValue(t, before, errorSample)

	ObserveCacheRead("card", 5, 2, nil)
	ObserveCacheRead("card", 1, 0, errors.New("redis down"))

	after := scrapeMetrics(t)
	if got := metricValue(t, after, hitSample) - beforeHit; got != 2 {
		t.Fatalf("hit delta = %v, want 2", got)
	}
	if got := metricValue(t, after, missSample) - beforeMiss; got != 3 {
		t.Fatalf("miss delta = %v, want 3", got)
	}
	if got := metricValue(t, after, errorSample) - beforeError; got != 1 {
		t.Fatalf("error delta = %v, want 1", got)
	}
}

// TestObserveCacheWriteTreatsNonPositiveCountAsOne 验证非正写入数量按一次记录，写入错误计入结果标签。
func TestObserveCacheWriteTreatsNonPositiveCountAsOne(t *testing.T) {
	const zeroCountSample = `videofeed_feed_cache_writes_total{area="zero",result="success"}`
	const errorSample = `videofeed_feed_cache_writes_total{area="failing",result="error"}`

	before := scrapeMetrics(t)
	beforeZero := metricValue(t, before, zeroCountSample)
	beforeError := metricValue(t, before, errorSample)

	ObserveCacheWrite("zero", 0, nil)
	ObserveCacheWrite("failing", 1, errors.New("redis down"))

	after := scrapeMetrics(t)
	if got := metricValue(t, after, zeroCountSample) - beforeZero; got != 1 {
		t.Fatalf("zero count write delta = %v, want 1", got)
	}
	if got := metricValue(t, after, errorSample) - beforeError; got != 1 {
		t.Fatalf("failing write delta = %v, want 1", got)
	}
}

// TestHTTPMiddlewareUsesRouteAndResultLabels 验证中间件按完整路由、方法和状态码记录请求。
func TestHTTPMiddlewareUsesRouteAndResultLabels(t *testing.T) {
	const sample = `videofeed_http_requests_total{method="GET",route="/api/videos/:videoId",status="404"}`

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(HTTPMiddleware())
	engine.GET("/api/videos/:videoId", func(c *gin.Context) {
		c.Status(http.StatusNotFound)
	})

	before := metricValue(t, scrapeMetrics(t), sample)

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/videos/9", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}

	after := metricValue(t, scrapeMetrics(t), sample)
	if got := after - before; got != 1 {
		t.Fatalf("middleware counter delta = %v, want 1", got)
	}
}

// TestRunServerStopsOnContextCancel 验证取消上下文后指标服务优雅关闭并返回空错误。
func TestRunServerStopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- RunServer(ctx, "127.0.0.1:0")
	}()

	// 等待监听启动后取消，避免在监听前关闭导致非优雅退出。
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunServer() error = %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("RunServer() did not stop after context cancellation")
	}
}

// TestNormalizeLabelAndResultLabel 验证标签清洗与结果映射。
func TestNormalizeLabelAndResultLabel(t *testing.T) {
	if got := normalizeLabel("  TIMELINE ", "unknown"); got != "timeline" {
		t.Fatalf("normalizeLabel() = %q, want timeline", got)
	}
	if got := normalizeLabel("   ", "unknown"); got != "unknown" {
		t.Fatalf("normalizeLabel() = %q, want unknown", got)
	}
	if got := resultLabel(nil); got != "success" {
		t.Fatalf("resultLabel(nil) = %q, want success", got)
	}
	if got := resultLabel(errors.New("boom")); got != "error" {
		t.Fatalf("resultLabel(err) = %q, want error", got)
	}
}
