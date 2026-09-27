package inframetrics

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// HTTPRequestsTotal 统计按方法、稳定路由和状态码分组的 HTTP 请求总量。
	HTTPRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "videofeed",
			Name:      "http_requests_total",
			Help:      "Total HTTP requests handled by the API.",
		},
		[]string{"method", "route", "status"},
	)

	// HTTPRequestDuration 统计按方法、稳定路由和状态码分组的 HTTP 请求耗时。
	HTTPRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "videofeed",
			Name:      "http_request_duration_seconds",
			Help:      "HTTP request duration in seconds.",
			Buckets:   []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
		},
		[]string{"method", "route", "status"},
	)

	// FeedRequestsTotal 统计按场景和结果分组的 Feed 请求总量。
	FeedRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "videofeed",
			Name:      "feed_requests_total",
			Help:      "Total feed requests by scene and result.",
		},
		[]string{"scene", "result"},
	)

	// FeedRequestDuration 统计按场景和结果分组的 Feed 请求耗时。
	FeedRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "videofeed",
			Name:      "feed_request_duration_seconds",
			Help:      "Feed request duration in seconds by scene.",
			Buckets:   []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
		},
		[]string{"scene", "result"},
	)

	// FeedItemsTotal 统计按场景分组的 Feed 返回条目总量。
	FeedItemsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "videofeed",
			Name:      "feed_items_total",
			Help:      "Total feed items returned by scene.",
		},
		[]string{"scene"},
	)

	// FeedCacheRequestsTotal 统计按缓存区域和结果分组的缓存读取总量。
	FeedCacheRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "videofeed",
			Name:      "feed_cache_requests_total",
			Help:      "Feed cache reads by cache area and result.",
		},
		[]string{"area", "result"},
	)

	// FeedCacheWritesTotal 统计按缓存区域和结果分组的缓存写入总量。
	FeedCacheWritesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "videofeed",
			Name:      "feed_cache_writes_total",
			Help:      "Feed cache writes by cache area and result.",
		},
		[]string{"area", "result"},
	)

	// VideoUploadTotal 统计按上传类型和结果分组的上传请求总量。
	VideoUploadTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "videofeed",
			Name:      "video_upload_total",
			Help:      "Upload requests by kind and result.",
		},
		[]string{"kind", "result"},
	)

	// VideoUploadDuration 统计按上传类型和结果分组的上传处理耗时。
	VideoUploadDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "videofeed",
			Name:      "video_upload_duration_seconds",
			Help:      "Upload request processing duration in seconds.",
			Buckets:   []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120},
		},
		[]string{"kind", "result"},
	)

	// VideoProcessingDuration 统计按处理步骤和结果分组的媒体处理耗时。
	VideoProcessingDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "videofeed",
			Name:      "video_processing_duration_seconds",
			Help:      "Video processing step duration in seconds.",
			Buckets:   []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120},
		},
		[]string{"step", "result"},
	)

	// WorkerJobsTotal 统计按任务名和结果分组的异步任务总量。
	WorkerJobsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "videofeed",
			Name:      "worker_jobs_total",
			Help:      "Worker jobs handled by job name and result.",
		},
		[]string{"job", "result"},
	)

	// WorkerJobDuration 统计按任务名和结果分组的异步任务耗时。
	WorkerJobDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "videofeed",
			Name:      "worker_job_duration_seconds",
			Help:      "Worker job processing duration in seconds.",
			Buckets:   []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		},
		[]string{"job", "result"},
	)
)

// init 将全部指标注册到默认注册表，供 /metrics 暴露。
func init() {
	prometheus.MustRegister(
		HTTPRequestsTotal,
		HTTPRequestDuration,
		FeedRequestsTotal,
		FeedRequestDuration,
		FeedItemsTotal,
		FeedCacheRequestsTotal,
		FeedCacheWritesTotal,
		VideoUploadTotal,
		VideoUploadDuration,
		VideoProcessingDuration,
		WorkerJobsTotal,
		WorkerJobDuration,
	)
}

// HTTPMiddleware 记录请求总量和耗时；路由标签优先使用 Gin 完整路由，未命中时回退原始路径。
func HTTPMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		route := c.FullPath()
		if route == "" {
			route = c.Request.URL.Path
		}
		status := strconv.Itoa(c.Writer.Status())
		method := c.Request.Method
		duration := time.Since(start).Seconds()

		HTTPRequestsTotal.WithLabelValues(method, route, status).Inc()
		HTTPRequestDuration.WithLabelValues(method, route, status).Observe(duration)
	}
}

// Handler 返回暴露默认注册表指标的 HTTP 处理器。
func Handler() http.Handler {
	return promhttp.Handler()
}

// RunServer 在指定地址启动指标服务，并在上下文取消时优雅关闭。
func RunServer(ctx context.Context, addr string) error {
	server := &http.Server{
		Addr:    addr,
		Handler: Handler(),
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	err := server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// ObserveFeed 记录一次 Feed 请求的场景、结果、耗时和成功返回的条目数量。
func ObserveFeed(scene string, duration time.Duration, itemCount int, err error) {
	scene = normalizeLabel(scene, "unknown")
	result := resultLabel(err)
	FeedRequestsTotal.WithLabelValues(scene, result).Inc()
	FeedRequestDuration.WithLabelValues(scene, result).Observe(duration.Seconds())
	if err == nil && itemCount > 0 {
		FeedItemsTotal.WithLabelValues(scene).Add(float64(itemCount))
	}
}

// ObserveCacheRead 记录一次批量缓存读取的命中与未命中数量；读取失败只计错误。
func ObserveCacheRead(area string, requested int, hit int, err error) {
	area = normalizeLabel(area, "unknown")
	if err != nil {
		FeedCacheRequestsTotal.WithLabelValues(area, "error").Inc()
		return
	}
	if hit > 0 {
		FeedCacheRequestsTotal.WithLabelValues(area, "hit").Add(float64(hit))
	}
	miss := requested - hit
	if miss > 0 {
		FeedCacheRequestsTotal.WithLabelValues(area, "miss").Add(float64(miss))
	}
}

// ObserveCacheWrite 记录一次缓存写入，数量非正时按一次计算。
func ObserveCacheWrite(area string, count int, err error) {
	area = normalizeLabel(area, "unknown")
	if count <= 0 {
		count = 1
	}
	FeedCacheWritesTotal.WithLabelValues(area, resultLabel(err)).Add(float64(count))
}

// ObserveUpload 记录一次上传请求的类型、结果和处理耗时。
func ObserveUpload(kind string, duration time.Duration, err error) {
	kind = normalizeLabel(kind, "unknown")
	result := resultLabel(err)
	VideoUploadTotal.WithLabelValues(kind, result).Inc()
	VideoUploadDuration.WithLabelValues(kind, result).Observe(duration.Seconds())
}

// ObserveVideoProcessing 记录一次媒体处理步骤的结果和耗时。
func ObserveVideoProcessing(step string, duration time.Duration, err error) {
	step = normalizeLabel(step, "unknown")
	VideoProcessingDuration.WithLabelValues(step, resultLabel(err)).Observe(duration.Seconds())
}

// ObserveWorkerJob 记录一次异步任务的结果和耗时。
func ObserveWorkerJob(job string, duration time.Duration, err error) {
	job = normalizeLabel(job, "unknown")
	result := resultLabel(err)
	WorkerJobsTotal.WithLabelValues(job, result).Inc()
	WorkerJobDuration.WithLabelValues(job, result).Observe(duration.Seconds())
}

// resultLabel 将错误状态映射为指标标签。
func resultLabel(err error) string {
	if err != nil {
		return "error"
	}
	return "success"
}

// normalizeLabel 清理标签值，空值使用回退值。
func normalizeLabel(value string, fallback string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return fallback
	}
	return value
}
