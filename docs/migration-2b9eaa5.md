# Prometheus 与 Grafana 监控迁移审计

来源：`C:/Users/HP/Desktop/GCFeed`，提交 `2b9eaa563f6434e4469ee63f0832dde18f5f964e`，标题为 `feat(monitoring): 接入 Prometheus 和 Grafana 监控`，作者与提交时间均为 2026-06-06 21:07:50 +0800。目标为当前 VideoFeed，迁移前工作区干净，基线为提交 `d557ad7`。

## 提交完整变更

已确认该提交只存在于来源仓库，并执行 `git show --stat`、`git show --name-only`、`git show --format=fuller` 以及逐文件完整 diff。

| 项 | 结果 |
| --- | --- |
| 变更文件 | 22 个，覆盖依赖、指标模块、应用与基础设施埋点、HTTP 接口、部署编排、监控配置、看板、压测脚本和 README |
| 变更规模 | `1036 insertions(+), 13 deletions(-)` |
| 变更类型 | 新增可观测能力，不改变既有接口路径、请求与响应契约 |

来源仓库使用 `apps/api`、`apps/monitoring`、MySQL 和 `GCFeed` 模块路径；VideoFeed 使用仓库根目录的 `internal`、根目录 `monitoring`、PostgreSQL 和 `github.com/inumoroha/video-feed` 模块路径，因此下表按真实结构重新映射。

## 逐文件迁移表

| 原文件 | 当前文件 | 新建 | 声明与适配 | 测试和文档 |
| --- | --- | --- | --- | --- |
| `apps/api/internal/infra/metrics/metrics.go` | `internal/infrastructure/metrics/metrics.go` | 是 | 包名 `inframetrics`，模块路径与命名空间改为 `videofeed`；12 个指标、注册、中间件、指标服务、6 个观测函数保持一致；补充全部中文注释 | `internal/infrastructure/metrics/metrics_test.go` |
| `apps/api/go.mod` | `go.mod` | 否 | 新增 `github.com/prometheus/client_golang v1.23.2` 及 5 个间接依赖，与来源版本一致 | `go.sum` |
| `apps/api/go.sum` | `go.sum` | 否 | 由 `go mod tidy` 生成 | 同上 |
| `apps/api/internal/infra/httpgin/gin.go` | `internal/router/router.go` | 否 | 目标没有独立 httpgin 包，Gin 引擎在 `Setup` 创建，因此在 `Setup` 内挂载 `inframetrics.HTTPMiddleware()` | `internal/router` 既有测试 |
| `apps/api/internal/interfaces/http/router/router.go` | `internal/router/register.go` | 否 | 在既有 `/health` 旁注册 `GET /metrics`，改用 `gin.WrapH(inframetrics.Handler())` | 同上 |
| `apps/api/internal/application/feed/service.go` | `internal/application/feed/service.go` | 否 | `GetFeed` 记录场景、结果、耗时与条目数；不支持场景按错误计数；顺带去掉重复的空服务判断 | `internal/application/feed` 既有测试 |
| `apps/api/internal/application/interaction/worker.go` | `internal/application/interaction/worker.go` | 否 | `HandleActionChanged` 记录 `interaction_action_changed` 任务；改用命名返回值以便 defer 上报 | `internal/application/interaction` 既有测试 |
| `apps/api/internal/application/video/fanout_worker.go` | `internal/application/video/fanout_worker.go` | 否 | `HandleVideoPublished` 记录 `video_fanout` 任务；循环内错误变量改名避免遮蔽命名返回值 | `internal/application/video` 既有测试 |
| `apps/api/internal/application/embedding/worker.go` | `internal/application/embedding/worker.go` | 否 | `HandleVideoPublished` 记录 `video_embedding` 任务 | `internal/application/embedding` 既有测试 |
| `apps/api/internal/infra/cache/feed_cache.go` | `internal/infrastructure/cache/feed/feed.go` | 否 | 页、卡片、计数三类缓存的读写埋点；`getCards` 内部函数与 `getStats` 包级函数同样埋点，`area` 取值 `page`、`card`、`stat` | `internal/infrastructure/cache/feed/feed_test.go` |
| `apps/api/internal/infra/mq/rabbitmq.go` | `internal/infrastructure/mq/rabbitmq.go` | 否 | 互动、视频发布、画像三类消费的解码与消费分别记录；解码失败记 `*_decode`，处理失败记 `*_consume` 并保持原有 Nack/Ack 语义 | `internal/infrastructure/mq` 既有测试 |
| `apps/api/internal/interfaces/http/upload/handler.go` | `internal/handler/upload/handler.go` | 否 | `Create` 记录上传类型、结果与耗时，`kind` 合法后回填；`probe` 与 `faststart` 两步记录处理耗时；保存失败时保留目标仓库原有的不删除文件行为 | `internal/handler/upload` 既有测试 |
| `apps/api/cmd/worker/main.go` | `cmd/worker/main.go` | 否 | 新增 `metricsAddr = ":9091"` 并在 `main` 中启动指标服务；保持 `run` 签名不变 | `cmd/worker/main_test.go` |
| `apps/docker-compose.yml` | `docker-compose.yaml` | 否 | 新增 `prometheus` 与 `grafana` 服务及 `prometheus_data`、`grafana_data` 卷；抓取目标端口按 VideoFeed 改为 `api:9000`、`worker:9091`；容器名改为 `videofeed-*` | 本记录、[部署说明](deployment.md) |
| `apps/monitoring/prometheus.yml` | `monitoring/prometheus.yml` | 是 | 作业名改为 `videofeed-api`、`videofeed-worker`；API 目标端口由来源 8080 适配为 9000 | 同上 |
| `apps/monitoring/grafana/provisioning/datasources/prometheus.yml` | `monitoring/grafana/provisioning/datasources/prometheus.yml` | 是 | 数据源配置一致，显式声明 `uid: Prometheus` 供看板引用 | 同上 |
| `apps/monitoring/grafana/provisioning/dashboards/gcfeed.yml` | `monitoring/grafana/provisioning/dashboards/videofeed.yml` | 是 | provider 名称与目录改为 VideoFeed | 同上 |
| `apps/monitoring/grafana/provisioning/alerting/.gitkeep` | 未迁移 | — | 空占位文件，不含配置内容，无可迁移声明 | — |
| `apps/monitoring/grafana/provisioning/plugins/.gitkeep` | 未迁移 | — | 同上 | — |
| `apps/monitoring/grafana/dashboards/gcfeed-overview.json` | `monitoring/grafana/dashboards/videofeed-overview.json` | 是 | 7 个面板与全部 PromQL 保留，指标前缀 `gcfeed_` 改为 `videofeed_`，看板 uid、标题和文件夹改为 VideoFeed；JSON 重新解析校验通过 | 同上 |
| `scripts/feed-load.js` | `scripts/feed-load.js` | 是 | 保留 k6 场景与阈值，登录沿用 `/api/sessions`，推荐场景路径由来源 `/api/feed-queries` 适配为当前 `/api/feed/query`，默认 BASE_URL 由 8080 适配为 9000 | 语法检查 |
| `README.md` | `README.md` | 否 | 已实现能力新增监控条目、待补能力改为告警规则与告警事件、Compose 服务数与服务地址表补充 Prometheus 与 Grafana | 本记录 |

该提交不新增或改变业务接口，因此 `api/openapi.yaml` 无需修改；但新增了 `GET /metrics` 运维端点，已在 [部署说明](deployment.md) 与 [监控模块](modules/monitoring.md) 记录，未写入 OpenAPI 是因为该端点不属于业务 API 契约，与仓库既有 `/health` 的处理方式一致。

## 逐项行为核对

- **指标清单**：来源 12 个指标全部保留，名称除命名空间前缀外完全一致；histogram 桶边界逐项相同（HTTP 与 Feed 为 `0.005…5`，上传与处理为 `0.05…120`，worker 为 `0.005…10`）。
- **观测语义**：`ObserveFeed` 仅在成功且有条目时累计条目数；`ObserveCacheRead` 读取失败只计 `error`、命中与未命中分别计数；`ObserveCacheWrite` 非正数量按一次计算；`ObserveWorkerJob` 同时计数与观测耗时。以上分支与来源逐条一致。
- **标签清洗**：`normalizeLabel` 去空白转小写并对空值回退 `unknown`，`resultLabel` 只返回 `success` 或 `error`，与来源一致。
- **中间件**：优先使用 `c.FullPath()` 作为路由标签，为空时回退请求路径，避免高基数标签；`c.Next()` 之后读取状态码，与来源一致。
- **指标服务**：`RunServer` 在上下文取消时用 3 秒超时优雅关闭，`http.ErrServerClosed` 返回空错误，与来源一致。
- **消费语义不变**：RabbitMQ 三类消费的 Nack/Ack 分支、无效 JSON 丢弃、处理失败重新入队均保持原状，埋点只新增观测调用。
- **上传语义不变**：`Create` 的成功路径、校验失败、处理失败和删除文件行为保持目标仓库原有逻辑；来源在保存失败时新增删除文件，VideoFeed 未采纳，保持既有行为。

## 与来源的差异

- 命名空间 `gcfeed_` 改为 `videofeed_`，作业名、容器名、看板 uid 同步改名。
- API 指标端口由来源 `8080` 适配为 VideoFeed 的 `9000`；worker 指标端口沿用 `9091`。
- 来源有独立 `httpgin` 包负责创建 Gin 引擎，VideoFeed 在 `internal/router.Setup` 创建，因此中间件挂载点相应移动。
- 来源压测脚本的推荐场景路径 `POST /api/feed-queries` 在 VideoFeed 中是 `POST /api/feed/query`，已适配。
- 来源在两个 Grafana provisioning 目录放空 `.gitkeep` 占位，VideoFeed 未复制空文件，不影响功能。
- 来源 README 的监控文档内容按 VideoFeed 真实端口、服务名与看板链接重写，未直接复制旧仓库地址。

## 测试映射

| 来源 | 当前位置 | 处理 |
| --- | --- | --- |
| 无（来源提交未新增测试） | `internal/infrastructure/metrics/metrics_test.go` | 新增 7 个测试：指标端点暴露与命名空间、Feed 观测结果与条目、缓存命中与未命中拆分、非正写入按一次、中间件路由与状态码标签、上下文取消优雅关闭、标签清洗与结果映射 |

测试针对全局默认注册表，因此计数类断言使用调用前后的差值，避免测试间互相污染；带标签的仪表在首次使用前不输出，端点测试先触发全部采集器。

## 验证结果

- 重新枚举来源 22 个文件，逐一对应迁移表；两个 `.gitkeep` 为空占位文件，已说明不迁移的原因。
- `go build ./...`：通过。
- `go vet ./...`：通过，退出码 0。
- `go test ./...`：通过，无失败；`go test -count=1 ./internal/infrastructure/metrics/` 7 个测试全部通过。
- `gofmt -w` 后对改动文件执行 `gofmt -l` 无输出。
- OpenAPI：本次未改变业务接口，`api/openapi.yaml` 未修改；已核对既有 42 个 operationId 仍然唯一。
- YAML 与 JSON 解析：`docker-compose.yaml`、`monitoring/prometheus.yml`、两个 Grafana provisioning 文件解析通过；看板 JSON 解析通过，7 个面板与看板 uid 正常。
- `scripts/feed-load.js`：Node ESM 语法检查通过。
- 依赖：`go.mod` 新增的 Prometheus 版本与来源一致（`v1.23.2`），间接依赖为 `beorn7/perks`、`munnerz/goautoneg`、`prometheus/client_model`、`prometheus/common`、`prometheus/procfs`。
- 新增或修改的函数、结构体均已添加紧邻中文注释。

## 已知限制与未验证内容

- `docker compose config --quiet` 未执行成功，本机 Docker 守护进程未运行；本次以 YAML 解析与结构断言代替，未实际启动 Prometheus 与 Grafana 容器。
- 未执行真实抓取验证：没有启动 API/worker 并用 Prometheus 实际抓到指标，也没有打开 Grafana 看板确认面板出图；看板 PromQL 只做了静态替换与 JSON 解析。
- 未执行 k6 压测：环境中没有 k6，`scripts/feed-load.js` 只做了语法检查。
- `deploy.yaml` 未包含 Prometheus 与 Grafana，也没有为 worker `9091` 增加 Service 或探针，Kubernetes 环境无法抓取 worker 指标；该限制已写入部署说明。
- 来源未提供指标单元测试，新增的 7 个测试是对该提交行为的补充覆盖，不代表完成了端到端监控联调。

## 最终审计

来源提交 22 个文件重新枚举：依赖 2 个、新增指标模块 1 个、应用层埋点 4 个、基础设施埋点 2 个、HTTP 接口与路由 3 个、入口 1 个、部署编排 1 个、监控配置 3 个、看板 1 个、占位 2 个、压测脚本 1 个、README 1 个，全部有明确迁移结果或说明。12 个指标、6 个观测函数、中间件、指标服务与全部分支语义逐项核对等价；命名空间、端口、路径与目录按 VideoFeed 真实结构适配。

结论：来源提交的可观测能力已完整迁移并按 VideoFeed 命名与端口适配，未发现尚未迁移的行为；容器编排、真实抓取、看板出图与压测未执行，已在限制中列出。
