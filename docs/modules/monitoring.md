# 监控告警模块设计

> 状态：指标采集与看板已实现（Prometheus + Grafana）；下述告警规则、告警事件和运营看板接口仍为设计目标，当前没有对应实现，不能直接调用。数据类型按 PostgreSQL 适配。

## 0. 已实现部分

Prometheus 指标与 Grafana 看板已接入，实现位于 `internal/infrastructure/metrics`，命名空间为 `videofeed_`。

| 暴露位置 | 内容 |
| --- | --- |
| API `GET /metrics` | 默认注册表全部指标，由 `internal/router` 注册 |
| worker `:9091` | 独立指标服务，上下文取消时优雅关闭 |

| 指标 | 类型 | 标签 | 采集点 |
| --- | --- | --- | --- |
| `videofeed_http_requests_total` | counter | `method`、`route`、`status` | Gin 中间件，路由使用 `c.FullPath()`，未命中回退原始路径 |
| `videofeed_http_request_duration_seconds` | histogram | `method`、`route`、`status` | 同上 |
| `videofeed_feed_requests_total` | counter | `scene`、`result` | `internal/application/feed` 的 `GetFeed`，不支持场景计为错误 |
| `videofeed_feed_request_duration_seconds` | histogram | `scene`、`result` | 同上 |
| `videofeed_feed_items_total` | counter | `scene` | 同上，仅成功且有条目时累计 |
| `videofeed_feed_cache_requests_total` | counter | `area`、`result` | Feed 缓存页、卡片、计数读取；`area` 为 `page`、`card`、`stat` |
| `videofeed_feed_cache_writes_total` | counter | `area`、`result` | 同上，写入数量非正按一次计算 |
| `videofeed_video_upload_total` | counter | `kind`、`result` | 独立上传按文件类型记录，组合发布使用 `publish`；未知类型使用 `unknown` |
| `videofeed_video_upload_duration_seconds` | histogram | `kind`、`result` | 同上 |
| `videofeed_video_processing_duration_seconds` | histogram | `step`、`result` | `probe` 与 `faststart` 两个处理步骤 |
| `videofeed_worker_jobs_total` | counter | `job`、`result` | 互动、分发、向量 worker 与 RabbitMQ 解码/消费 |
| `videofeed_worker_job_duration_seconds` | histogram | `job`、`result` | 同上 |

标签值统一去除首尾空白并转小写，空值使用 `unknown`；结果标签只有 `success` 和 `error`。

Grafana 看板 `monitoring/grafana/dashboards/videofeed-overview.json` 提供 API QPS、API 5xx 比例、API P95、Feed 各场景 P95、缓存命中率、上传耗时、视频处理耗时和 worker 成功率八个面板。部署方式、端口与限制见 [部署说明](../deployment.md)。

## 1. 模块职责

监控告警模块负责业务指标采集、核心看板、告警规则和告警事件查询。

## 2. 接口设计

| 方法 | 接口路径 | 作用 | 鉴权 | 幂等键 |
| --- | --- | --- | --- | --- |
| POST | `/internal/metric-points` | 写入业务指标点 | 服务鉴权 | 支持 |
| GET | `/api/admin/metric-dashboard` | 查询核心看板数据 | Bearer JWT(运营角色) | 无 |
| POST | `/api/admin/alerts/rules` | 新建告警规则 | Bearer JWT(管理员) | 支持 |
| GET | `/api/admin/alerts/events` | 查询告警事件 | Bearer JWT(运营角色) | 无 |

## 3. 数据表设计

### `monitor_metric_point`

| 字段 | 类型 | 约束 | 说明 |
| --- | --- | --- | --- |
| `id` | BIGINT | PK | 记录 ID |
| `metric_name` | VARCHAR(128) | NOT NULL | 指标名 |
| `labels` | JSON | NULLABLE | 标签 |
| `value` | DOUBLE | NOT NULL | 指标值 |
| `ts` | TIMESTAMPTZ | NOT NULL | 指标时间 |

索引建议：`idx_metric_ts(metric_name, ts)`。

### `monitor_alert_rule`

| 字段 | 类型 | 约束 | 说明 |
| --- | --- | --- | --- |
| `id` | BIGINT | PK | 规则 ID |
| `rule_name` | VARCHAR(128) | UNIQUE, NOT NULL | 规则名 |
| `metric_name` | VARCHAR(128) | NOT NULL | 指标名 |
| `condition_expr` | VARCHAR(255) | NOT NULL | 告警条件 |
| `enabled` | SMALLINT | NOT NULL, DEFAULT 1 | 是否启用 |
| `updated_at` | TIMESTAMPTZ | NOT NULL | 更新时间 |

索引建议：`idx_metric_enabled(metric_name, enabled)`。

### `monitor_alert_event`

| 字段 | 类型 | 约束 | 说明 |
| --- | --- | --- | --- |
| `id` | BIGINT | PK | 事件 ID |
| `rule_id` | BIGINT | NOT NULL | 规则 ID |
| `status` | VARCHAR(16) | NOT NULL | `FIRING` / `RESOLVED` |
| `trigger_value` | DOUBLE | NOT NULL | 触发值 |
| `triggered_at` | TIMESTAMPTZ | NOT NULL | 触发时间 |
| `resolved_at` | TIMESTAMPTZ | NULLABLE | 恢复时间 |

索引建议：`idx_status_triggered(status, triggered_at)`。

## 4. 业务规则

| 规则 | 说明 |
| --- | --- |
| 指标点支持标签 | 标签保存接口、场景、实例等维度 |
| 看板按时间窗口聚合 | 支持最近分钟、小时和天级查询 |
| 告警规则可启停 | 停用规则不产生新事件 |
| 告警事件保留状态 | 触发和恢复都可查询 |
| 内部写入保持幂等 | 同一批指标重复写入结果稳定 |

## 5. 测试建议

| 场景 | 期望 |
| --- | --- |
| 写入指标点 | 数据入库 |
| 查询看板 | 返回聚合指标 |
| 创建告警规则 | 规则启用 |
| 查询告警事件 | 按状态和时间返回 |
| 非管理员创建规则 | 返回 403 |

## 5.1 已实现部分的测试

`internal/infrastructure/metrics/metrics_test.go` 覆盖：`/metrics` 暴露全部采集器且不含 `gcfeed_` 命名空间、Feed 观测按结果计数与条目累计、批量缓存读取按命中/未命中/错误分别计数、非正写入数量按一次记录、HTTP 中间件使用完整路由与状态码标签、上下文取消后指标服务优雅关闭、标签清洗与结果映射。

## 6. 前端接入点

| 页面 | 接入能力 |
| --- | --- |
| 监控看板 | Feed、互动、播放、队列和数据库指标 |
| 告警规则页 | 创建和启停规则 |
| 告警事件页 | 查看触发和恢复记录 |
