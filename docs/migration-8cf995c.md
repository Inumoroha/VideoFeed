# 面试问答清单迁移审计

来源：`C:/Users/HP/Desktop/GCFeed`，提交 `8cf995cfa03985c335594c453ea050df5c0f1b6a`，标题为 `docs(interview): 补充项目面试问答清单`，作者与提交时间均为 2026-06-09 20:02:50 +0800。目标为当前 VideoFeed，迁移前工作区干净，基线为提交 `4832f74`。该提交是来源仓库 `main` 分支的当前顶端。

## 提交完整变更

已确认该提交只存在于来源仓库，并执行 `git show --stat`、`git show --name-only`、`git show --format=fuller` 以及逐文件完整 diff。

| 项 | 结果 |
| --- | --- |
| 变更文件 | 1 个：新增 `docs/interview-questions.md` |
| 变更规模 | `890 insertions(+)`，无删除 |
| 变更类型 | 纯新增文档，无函数、结构体、类型、接口、常量、测试、配置、依赖、数据库、缓存、消息队列、接口或部署变更 |

## 逐文件迁移表

| 原文件 | 当前文件 | 新建 | 声明与适配 | 测试和文档 |
| --- | --- | --- | --- | --- |
| `docs/interview-questions.md` | `docs/interview-questions.md` | 是 | 20 个章节与全部 196 个编号问题逐条保留；项目名、数据库、分层目录、表名、路由、常量数字、指标命名空间、队列名与命令路径按 VideoFeed 真实实现改写；新增「使用说明」与「讲解注意事项」两节 | 本记录；[README](../README.md) 文档地图 |
| 无对应来源文件 | `README.md` | 否 | 文档地图新增该清单条目，与仓库既有文档导航惯例一致 | 同上 |

来源提交没有改名或删除文件，因此不存在路径冲突；VideoFeed 的 `docs/` 下此前没有同名文件。

## 结构等价核对

| 项 | 来源 | 目标 |
| --- | --- | --- |
| 二级章节 | 20 个（1 到 20） | 20 个，编号与标题逐字一致 |
| 编号问题条目 | 196 | 196 |
| 反问结构 | 每章「基础问题」+「追问」 | 保持一致 |
| 结尾练习顺序 | 有 | 有 |

目标文档另有「使用说明」与「讲解注意事项」两节，属于本次主动补充，来源没有对应内容，原因见下节。

## 与来源的差异

### 机械适配（事实性更正）

| 来源表述 | VideoFeed 真实情况 | 依据 |
| --- | --- | --- |
| 项目名 GCFeed | VideoFeed | 仓库与模块名 |
| 数据库 MySQL | PostgreSQL | `go.mod`、`internal/database` |
| 分层 `Domain / Application / Infrastructure / Interfaces` | `domain / application / infrastructure / handler`，路由在 `internal/router`，中间件在 `internal/middleware` | `internal/` 目录结构 |
| 目录 `infra/cache`、`infra/persistence`、`interfaces/http/feed` | `internal/infrastructure/cache/feed`、`internal/infrastructure/persistence`、`internal/handler/feed` | 同上 |
| 表名 `account` | `users` | `internal/infrastructure/persistence/user/model.go` |
| `POST /api/feed-queries` | `POST /api/feed/query`（另有 `GET /api/feed-items?scene=recommend`） | `internal/router/register.go` |
| `cmd/feed/main.go` | `cmd/server/main.go`（另有 `cmd/worker/main.go`、`cmd/test/main.go`） | `cmd/` 目录 |
| 指标命名空间 `gcfeed` | `videofeed_` | `internal/infrastructure/metrics/metrics.go` |
| 交换机 `gcfeed.interaction`、`gcfeed.video`、`gcfeed.exposure` | Docker 配置为 `videofeed.*`；本地默认配置的互动交换机仍保留历史命名 `gcfeed.interaction` | `configs/config.yaml`、`configs/config.docker.yaml`、`internal/infrastructure/mq/rabbitmq.go` |
| 由 `infrafeed.EnsureTimelineIndex` 创建时间线索引 | 索引由视频模型的 GORM 索引标签在自动迁移时创建 | `internal/infrastructure/persistence/video/model.go`、`migration.go` |
| `npm --prefix apps/web run build` | `npm --prefix web run build` | 仓库根目录结构 |
| 数据卷 `mysql_data` | `postgres_data` 等 | `docker-compose.yaml` |
| `g.Static("/uploads", "./uploads")` | `g.Static("/uploads", uploadRoot)`，`uploadRoot` 由 `filepath.Abs("./uploads")` 得到 | `internal/router/register.go` |
| 测试集中在 `apps/api/test` | 分布在 `internal/` 各模块的 `_test.go` | 测试文件位置 |
| 仓库已有 6 个服务 | 8 个服务（含 Prometheus 与 Grafana） | `docker-compose.yaml` |

### 事实核对（对来源断言逐项验证）

以下数字与机制在 VideoFeed 代码中逐项核实，改写时保留了断言的实际取值：

| 断言 | 核对结果 | 依据 |
| --- | --- | --- |
| 时间线首页 5 秒、后续页 45 秒 | 正确，且热榜另有 10 秒 / 30 秒 | `internal/application/feed/service.go` 的 `timelineFirstPageCacheTTL`、`timelinePageCacheTTL`、`hotFirstPageCacheTTL`、`hotPageCacheTTL` |
| 卡片 15 分钟、计数 15 秒 | 正确 | `feedCardCacheTTL`、`feedStatCacheTTL` |
| TTL 抖动由 key hash 决定 | 正确，实际为 10% 到 20% | `feedPageCacheTTL` |
| 热榜权重 3 / 4 / 5 | 正确 | `hotScoreLikeWeight`、`hotScoreFavoriteWeight`、`hotScoreCommentWeight` |
| 分钟桶 `feed:hot:minute:v1:{yyyyMMddHHmm}`，TTL 2 小时 | 正确 | `hotMinuteKey`、`hotMinuteBucketTTL` |
| 合并 60 个桶、窗口 key TTL 2 分钟、`ZRemRangeByScore` | 正确 | `hotWindowMinutes`、`hotWindowCacheTTL`、`ListHotWindowPage` |
| inbox 1000、outbox 500、每批 500 | 正确 | `internal/application/video/fanout_worker.go` |
| 大作者阈值 10000 | 正确 | `domainfeed.BigCreatorFollowerThreshold` |
| 计数分片 16 | 正确 | `actionStatCounterShardCount`，按用户编号取模 |
| 候选池 `limit*8`，最小 50 最大 500 | 正确 | `candidatePoolMultiplier`、`minCandidatePoolSize`、`maxCandidatePoolSize` |
| 个性化 70/20/10，冷启动 65/35，热度 `log1p` 平滑 | 正确 | `internal/application/recommendation/service.go` 的 `rankScore` |
| `RecentExposureWindow` | 存在，取值为 7 天 | `internal/domain/recommendation/decisions.go` |
| 曝光聚合表记录四类字段 | 正确，表名 `exposures`，日志表 `video_view_events` | `internal/infrastructure/persistence/exposure/model.go` |
| Redis `WATCH` 保护点赞状态更新 | 正确 | `internal/infrastructure/cache/feed/feed.go` 的 `SetActionState` |
| `interleaveByAuthor`、`sameScore` | 均存在 | `internal/application/recommendation/service.go` |
| option 名称 `WithFeedCache`、`WithRecommender`、`WithAsyncActionPipeline` | 均存在；另有 `WithStatCache`、`WithMessageWriter`、`WithStrategy`、`WithFollowFeedBackfiller` 等 | `internal/application/**` |
| 四个策略类型 | 均存在 | `internal/application/feed` |

### 主动修正（来源断言与真实实现不符）

来源把若干尚未实现或范围更大的能力写成了已完成事实，本次按真实实现改写，并在「讲解注意事项」中汇总：

- **内部令牌覆盖范围**：来源称推荐、曝光、消息和播放 QoS 的内部接口都用 `X-Internal-Token`。实际只有 `POST /internal/messages` 与 `POST /internal/playback-qos-reports` 挂载该中间件，推荐和曝光的三个 `/internal/` 路由没有服务鉴权。
- **曝光削峰**：来源称曝光事件先写 RabbitMQ 由 Worker 批量落库。实际曝光观看事件在接口内同步落库、落库成功后才投递消息，曝光队列只有发布端，没有消费进程；播放行为事件也没有独立消费者。只有推荐内部接口 `POST /internal/exposures` 是单事务批量写入曝光流水与聚合表。该条改为扩展方向。
- **用户兴趣向量**：来源暗示有后台向量更新与画像构建。实际兴趣向量在推荐读取时按最近 30 天最多 200 条播放与完播事件现算，没有后台画像聚合进程。
- **死信队列与重试上限**：来源在多个追问里把死信队列当成已有机制。实际消费失败只有立即重新入队，没有最大重试次数、死信交换机和退避。
- **健康检查语义**：来源把 `/health` 描述为可用性判断。实际它只返回进程存活状态，不检查数据库、Redis 和 RabbitMQ。
- **审核角色**：来源称角色可用于内容治理。实际角色只用于删除评论的权限判断，审核后台尚未实现。
- **自动迁移并发保护**：来源未提。实际迁移在事务与 PostgreSQL 咨询锁内串行执行，并对并发 DDL 冲突做了有限重试。
- **压测数据**：来源在「写进简历」的答案里鼓励引用压测数字。本次补充说明指南中的示例数字只是格式说明，要求只使用自己真实跑出的数据。

## 新增章节说明

- **使用说明**：提醒读者文中数字来自代码常量，并区分「现有实现」与「扩展方向」，避免把设计讨论当成已完成能力。
- **讲解注意事项**：12 条实现边界清单，覆盖数据库与分层命名、内部令牌覆盖范围、曝光与行为事件消费者缺失、兴趣向量现算、RabbitMQ 无死信与重试上限、singleflight 仅时间线、关注流读取侧使用全部关注作者 Outbox、发布事件失败不补偿、曝光与观看事件无幂等键、治理与审核未实现、监控无告警、worker 指标无 Kubernetes Service、`/health` 不检查依赖。

这两节是新增内容，目的是让该清单在 VideoFeed 仓库里既可用于面试，也不会因为沿用来源措辞而误导。

## 测试与文档映射

来源提交没有新增测试，也不涉及代码行为，因此没有测试需要迁移。文档侧：

- 新增 `docs/interview-questions.md`。
- `README.md` 文档地图新增该清单条目，保持与既有文档导航一致。
- 本审计记录，并登记到 `docs/migrations.md`。

## 验证结果

- 重新枚举来源提交：只有 `docs/interview-questions.md` 一个文件、890 行新增，与迁移表对应。
- 结构对比：来源 20 个二级章节与 196 个编号条目，目标同样为 20 个章节与 196 个编号条目，逐条对应，无遗漏。
- 来源残留检查：目标文档中 `GCFeed`、`gcfeed`、`MySQL`、`/api/feed-queries`、`apps/api`、`apps/web`、`cmd/feed`、`infra/cache`、`interfaces/http`、`` `account` `` 只剩两处命中，均为有意保留并已说明：解释本地默认互动交换机保留历史命名 `gcfeed.interaction`，以及在注意事项里说明数据库是 PostgreSQL 而不是 MySQL。
- 常量核对：缓存 TTL、抖动区间、热榜权重与窗口参数、inbox/outbox 与批量大小、大作者阈值、分片数量、候选池上下限、排序权重、曝光窗口、曝光表字段均在代码中逐项确认。
- Markdown 相对链接检查：README 与新文档的站内链接全部可解析。
- 独立复核：另起一个只读审计员，对来源文档中的 30 条具体技术断言（缓存 TTL、热榜参数、inbox/outbox 与批量大小、大作者阈值、分片数量、候选池上下限、排序权重、曝光窗口与表字段、令牌覆盖范围、入口路径、表名、索引创建机制、QoS 表字段等）逐条对照代码核实，结论与本文档的改写一致；其中与来源不符的 7 条（交换机命名、入口目录、用户表名、索引创建机制、内部令牌覆盖范围、Feed 查询路径、曝光落库方式）正是本次更正的内容。
- 本次为纯 Markdown 变更，没有 Go 代码、测试、配置、依赖、数据库、缓存、消息队列或接口变更，`gofmt`、`go test`、`go vet`、OpenAPI 与 Compose 校验不受影响；未改动相关文件。

## 已知限制与未验证内容

- 未执行真实服务联调：本文档的事实核对来自静态代码与配置检查，没有启动 PostgreSQL、Redis、RabbitMQ 验证运行期行为。
- 未执行 k6 压测，文档中不包含 VideoFeed 的实测性能数据。
- 文档中关于「扩展方向」的建议（多级缓存、分库分表、对象存储、死信拓扑等）属于设计讨论，未在仓库中实现，也未验证。
- 来源是仓库顶端提交，本次迁移后 VideoFeed 与来源 `main` 的文档集合不再逐一对齐，仅完成本轮指定提交的迁移。

## 最终审计

来源提交的唯一文件已完整迁移：20 个章节、196 个编号问题、练习顺序全部保留；数据库、分层目录、表名、路由、命令路径、指标命名空间、队列名与全部可核对常量按 VideoFeed 真实实现更正；来源中把未实现能力写成已完成事实的七处内容已按真实实现改写并汇总到「讲解注意事项」。

结论：该提交的文档内容已完整迁移并按 VideoFeed 真实实现完成适配，未发现尚未迁移的章节或问题条目；主动新增了两节说明以标注实现边界，实际服务联调与压测未执行，已在限制中列出。
