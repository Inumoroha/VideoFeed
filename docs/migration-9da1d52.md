# 简历亮点文档迁移审计

来源：`C:/Users/HP/Desktop/GCFeed`，提交 `9da1d5234e082b21a6df5cdeb07bab47db29ce17`，标题为 `docs: 补充简历项目亮点`，作者与提交时间均为 2026-06-06 19:58:21 +0800。目标为当前 VideoFeed，开始时 `main` 分支工作区干净，基线 `f0d47ef`。

## 提交完整变更

已确认该提交只存在于来源仓库（VideoFeed 中不存在该对象），并执行 `git show --stat`、`git show --name-only`、`git show --format=fuller` 以及逐文件完整 diff。

| 项 | 结果 |
| --- | --- |
| 变更文件 | `docs/resume-highlights.md` |
| 变更规模 | `195 0`，即新增 195 行、删除 0 行 |
| 变更类型 | 纯新增 Markdown 文档，无函数、结构体、类型、接口、常量、测试、配置、依赖、数据库、缓存、消息队列或部署文件改动 |

## 逐文件迁移表

| 原文件 | 当前文件 | 新建 | 声明与适配 | 测试和文档 |
| --- | --- | --- | --- | --- |
| `docs/resume-highlights.md` | [`docs/resume-highlights.md`](resume-highlights.md) | 是 | 保留全部章节结构与能力条目，按 VideoFeed 真实架构逐条改写：项目名改为 VideoFeed、数据库改为 PostgreSQL、分层命名改为 Handler、内部令牌只描述实际覆盖范围，并删除当前未实现的监控、审核、后台运营与系统治理表述 | 本记录；[迁移与联调记录](migrations.md) 索引 |

来源提交没有可复用路径冲突：VideoFeed 的 `docs/` 目录此前没有同名文件，也没有其他简历类文档，因此该文件按原名同路径迁移，不需要重新映射文件名。

## 架构与命名适配

已先检查 VideoFeed 的 `AGENTS.md`、目录结构、当前分支与工作区状态，以及配置、依赖、数据库、缓存、消息队列和 HTTP 路由架构，再逐条核对文档声明，没有直接假设两个仓库目录一致。

| 维度 | 来源文档表述 | VideoFeed 实际情况 | 适配方式 |
| --- | --- | --- | --- |
| 项目与模块名 | GCFeed | VideoFeed，模块 `github.com/inumoroha/video-feed` | 正文项目名统一为 VideoFeed，不引入旧模块路径或包名 |
| 后端分层 | Domain、Application、Infrastructure、Interfaces | `internal/domain`、`internal/application`、`internal/infrastructure`、`internal/handler`，路由集中在 `internal/router` | 分层表述改为 Domain、Application、Infrastructure、Handler |
| 数据库 | MySQL | PostgreSQL 17，GORM 仅注册 postgres 驱动 | 全部 MySQL 表述删除，Compose 与部署段落改为 PostgreSQL |
| 缓存 | Redis ZSET、分片计数 | Redis ZSET 分钟桶、基数加 16 分片计数、WATCH 加事务管道 | 按 16 分片与「基数 + 分片增量」的真实读法改写 |
| 消息队列 | RabbitMQ 持久化、ACK/NACK、重入队 | topic 交换机与队列持久化、`DeliveryMode: amqp.Persistent`、显式 Ack 与 `Nack(false, true)` 重入队 | 保留等价描述；不声称存在重试上限、死信队列或退避 |
| 内部鉴权 | 推荐、曝光等内部接口用 X-Internal-Token | 仅 `POST /internal/messages` 与 `POST /internal/playback-qos-reports` 挂载令牌中间件，推荐与曝光三条内部路由无鉴权 | 改写为「站内消息创建和内部质量上报使用令牌」，并把推荐曝光无鉴权列入讲解注意事项 |
| 部署 | Docker Compose 含 MySQL、Kubernetes 清单 | Compose 为 postgres、redis、rabbitmq、api、worker、web 六服务；`deploy.yaml` 提供六类 Deployment 与 Service、4 个 PVC、依赖等待与健康检查 | 六服务与清单内容按真实资源配置改写 |

该提交不新增或修改任何 HTTP 接口，因此 `api/openapi.yaml` 无需变更；Compose、环境变量、数据卷、健康检查顺序、`go.mod` 与 `go.sum` 也没有对应增量。

## 逐条能力核对

对文档中的 50 条能力声明逐条对照 VideoFeed 代码核实：**已实现 33 条、部分实现 16 条、未实现 1 条**。部分实现与未实现项已按真实行为改写，或在文档末尾「讲解注意事项」中显式标注边界。

### Feed 流核心链路

| 编号 | 声明 | 结论与证据 |
| --- | --- | --- |
| 1 | 各列表稳定游标分页 | 已实现：`internal/application/feed/service.go` 覆盖 timeline、following、hot、recommend 四种游标；评论与消息分别使用 `created_at` 加评论、消息标识 |
| 2 | Feed 页缓存只存轻量字段 | 部分实现：缓存体只有视频 ID、发布时间、热度分（`internal/application/feed/dto.go`），不含作者 ID；只有时间线写入该缓存 |
| 3 | ID-Detail 分离查询 | 已实现：`internal/infrastructure/persistence/feed/repository.go` 先取轻量页，`internal/application/feed/service.go` 再批量聚合 |
| 4 | 批量聚合并按原序恢复 | 已实现：`BatchGetFeedCards` 与 `BatchGetFeedStats` 批量读取，组装阶段按原分页顺序重建 |
| 5 | 五类字段独立缓存 | 部分实现：字段齐全但统一封装在 `video:card:v1:{id}` 单个 JSON 中，不是逐字段独立缓存 |
| 6 | 互动计数缓存 | 已实现：`video:stat:v1:{id}` 加基数与分片键 |
| 7 | 缓存降级回源 | 已实现：页、卡片、计数缓存读取失败即回源数据库 |
| 8 | singleflight 合并回源 | 部分实现：仅时间线 Feed 页使用 `singleflight`，热榜、关注流、推荐流未使用 |
| 9 | TTL 抖动 | 已实现：按缓存键哈希取 10% 到 20% 抖动 |

### 关注流与视频分发

| 编号 | 声明 | 结论与证据 |
| --- | --- | --- |
| 10 | 推拉结合关注流 | 部分实现：写入侧按 `BigCreatorFollowerThreshold = 10000` 分流（`internal/application/video/fanout_worker.go`）；读取侧合并的是全部有效关注作者的 Outbox，带阈值的 `ListFollowingPullAuthorIDs` 生产路径未调用 |
| 11 | 分批 fanout | 已实现：按粉丝 ID 游标每批 500 写入 Inbox |
| 12 | 关注关系回填 | 已实现：新关注后最多回填 100 条近期作品 |
| 13 | 发布事件触发缓存预热 | 已实现：卡片 TTL 15 分钟、计数 TTL 15 秒 |
| 14 | 发布事件联动三条链路 | 部分实现：一个发布事件绑定分发与向量两个队列；发布失败被忽略（`internal/application/video/service.go` 中 `_ = s.publisher.PublishVideoPublished`），无重试补偿 |

### 异步解耦与互动性能

| 编号 | 声明 | 结论与证据 |
| --- | --- | --- |
| 15 | 点赞收藏先写 Redis 再异步落库 | 已实现：先 `SetActionState`（Redis WATCH 加事务管道），成功后再发消息；发布失败回退同步落库 |
| 16 | 四类链路消息队列拆分 | 已实现：互动、发布、向量、曝光四条队列；曝光队列只有发布没有消费进程 |
| 17 | 持久化消息与 ACK/NACK 重入队 | 已实现：交换机与队列持久化、持久化消息、显式 Ack 与 `Nack(false, true)`；没有重试上限、死信交换机或退避 |
| 18 | 六类写操作幂等 | 部分实现：视频、互动、评论、关注、消息、播放 QoS 幂等键齐全，并由唯一约束兜底；曝光和观看事件没有幂等键，QoS 空幂等键不去重 |
| 19 | 重复消费保护 | 已实现：唯一约束冲突时回读既有结果 |
| 20 | 事务内计数更新 | 已实现：互动事实与统计在同一事务，评论同理 |
| 21 | 计数最终一致 | 部分实现：Redis 计数加 Worker 落库成立，但每条消息独立同步事务、无批量聚合；计数键 TTL 为 24 小时，不是短 TTL |
| 22 | 计数分片 | 已实现：16 分片按用户编号取模，写入单分片、读取汇总全部分片 |

### 热榜与推荐系统

| 编号 | 声明 | 结论与证据 |
| --- | --- | --- |
| 23 | 按点赞收藏评论权重计算热度 | 已实现：`like*3 + comment*5 + favorite*4`，领域函数与两处 SQL 表达式一致 |
| 24 | 分钟桶热榜 | 已实现：`ZIncrBy feed:hot:minute:v1:{yyyyMMddHHmm}`，桶 TTL 2 小时 |
| 25 | 滑动窗口热榜 | 已实现：合并最近 60 个分钟桶后 `ZRevRange` 分页 |
| 26 | 候选池过滤近期曝光 | 已实现：候选查询左连曝光表并排除近七天已曝光，公开状态过滤在查询内完成 |
| 27 | 相似度、热度、新鲜度融合 | 已实现：有画像 `0.70/0.20/0.10`，无画像退化为热度 `0.65` 加新鲜度 `0.35` |
| 28 | 作者打散 | 已实现：相邻同作者延后 |
| 29 | 曝光去重 | 已实现：近七天窗口返回不允许曝光及原因 |
| 30 | 曝光聚合索引 | 已实现：首次曝光、最近曝光、曝光次数、最近场景，用户与视频唯一约束，原子累加 |
| 31 | 观看行为采集 | 部分实现：记录曝光、播放、完播、跳过以及观看时长和完成标记，没有独立的进度事件类型 |
| 32 | 用户兴趣向量 | 部分实现：没有后台画像聚合消费者，向量在推荐读取时按最近三十天最多 200 条正向事件现算 |
| 33 | 视频向量生成 | 已实现：发布事件驱动独立队列异步生成 hash-ngram 向量 |

### 播放与上传优化

| 编号 | 声明 | 结论与证据 |
| --- | --- | --- |
| 34 | 播放配置回退 | 已实现：先按平台与网络类型查询，再回落该平台 DEFAULT，最后使用领域默认配置 |
| 35 | 预加载建议 | 已实现：按发布时间与标识读取当前视频之后的公开作品，最多 10 条 |
| 36 | QoS 幂等上报 | 已实现：读取幂等键头并校验首帧、卡顿次数与观看时长 |
| 37 | 静态资源 Range | 已实现：`gin.Static` 注册上传目录，测试断言 206 与 `Content-Range` |
| 38 | 上传多维校验 | 已实现：大小、扩展名、MIME、十分钟时长、分辨率上限和音视频编码白名单 |
| 39 | MP4 faststart | 已实现：仅 MP4 与 MOV，`-c copy -movflags +faststart` 无重编码，成功替换原文件并始终清理临时文件 |

### 账号、消息与安全

| 编号 | 声明 | 结论与证据 |
| --- | --- | --- |
| 40 | 可选登录态 Feed | 已实现：`/api/feed-items` 与 `/api/feed/query` 使用可选 JWT |
| 41 | JWT 登录态与资料读写 | 已实现：注册、登录、当前用户、公开资料与资料更新 |
| 42 | 内部接口令牌鉴权 | 未实现（原表述不成立）：令牌中间件存在且使用常量时间比较，但只挂在站内消息创建与内部播放质量上报两条路由；推荐与曝光三条内部路由无任何鉴权 |
| 43 | 消息中心 | 已实现：点赞、评论、关注三类通知，支持分页、未读数和已读标记 |
| 44 | 消息幂等写入 | 部分实现：事件 ID 与幂等键去重完整；点赞事件 ID 为固定值，取消点赞后再次点赞不会生成新通知 |

### 工程化与部署

| 编号 | 声明 | 结论与证据 |
| --- | --- | --- |
| 45 | 分层架构 | 已实现：domain、application、infrastructure、handler 加 router、middleware、auth |
| 46 | 依赖注入 | 已实现：Redis、RabbitMQ、JWT、Repository 均以接口注入 |
| 47 | 自动迁移与索引 | 部分实现：迁移在事务与咨询锁内执行并创建时间线索引；`feed_inbox` 表只被迁移，生产路径零读写，关注流实际只用 Redis ZSET |
| 48 | Compose 一键启动 | 已实现（数据库不同）：六个服务为 postgres、redis、rabbitmq、api、worker、web，数据库是 PostgreSQL 17 而不是 MySQL |
| 49 | Kubernetes 清单 | 部分实现：六类 Deployment 与 Service、4 个 PVC、依赖等待与健康检查齐全；worker 没有健康端点，JWT 密钥是占位值 |
| 50 | 测试覆盖 | 已实现：`internal` 与 `cmd` 下共 105 个 `_test.go`，覆盖 Feed、推荐、曝光、互动、播放、上传、消息、关系与发布 Fanout |

## 与来源文档的差异处理

- 删除来源中「Docker Compose 支持 MySQL」与「Kubernetes 提供 API、Worker、Web、MySQL、Redis、RabbitMQ 清单」里的 MySQL 表述，改为 PostgreSQL。
- 删除来源中把内部令牌鉴权描述为覆盖推荐、曝光的说法，改为只描述实际挂载令牌的两条路由，并在讲解注意事项中说明推荐曝光当前无鉴权。
- 删除来源中「监控、审核后台、后台运营、系统治理」相关的完成时态暗示：来源文档本身没有单列这些能力，本次按 VideoFeed 现状明确它们仍处于规划阶段，不作为已完成能力。
- 补充分批 fanout 的批大小、关注回填条数、TTL 抖动区间、热度权重、排序权重、候选窗口、上传限制等真实数值，便于面试时给出可核对的细节。
- 新增「讲解注意事项」章节，把部分实现与未实现项的边界集中列出，避免简历或面试夸大实际能力。

## 验证结果

- 重新枚举来源提交全部文件：只有 `docs/resume-highlights.md` 一个文件，195 行新增，与迁移表逐项对应，没有遗漏文件。
- 逐章对照来源与目标文档：保留全部能力章节、简历压缩版三个版本与面试展开顺序；改写内容均可对应 VideoFeed 具体文件与行号。
- 50 条能力声明逐条核对的结论为已实现 33 条、部分实现 16 条、未实现 1 条，边界项均已在文档中显式标注。
- `gofmt -l .`：列出 9 个既有未格式化文件（`cmd/test/main.go`、`internal/application/video/dto.go`、`internal/application/video/error.go`、`internal/domain/user/error.go`、`internal/domain/user/repository.go`、`internal/domain/video/error.go`、`internal/domain/video/repository.go`、`internal/handler/video/handler.go`、`internal/infrastructure/cache/redis/redis.go`），均为本次迁移前已存在，本次没有 Go 变更，未修改这些文件。
- `go test ./...`：通过。
- `go test -count=1 ./...`：通过，未使用缓存结果。
- `go vet ./...`：通过，退出码 0。
- `git diff --check`：通过，退出码 0；`git status --porcelain` 在验证前为空，本次只有新增文档。
- `docker compose config --quiet`：未执行成功。本机 Docker 守护进程未运行（`docker version` 连接 `npipe:////./pipe/dockerDesktopLinuxEngine` 失败），因此 Compose 校验无法完成，本次未变更 Compose 文件。
- OpenAPI YAML：本次没有接口变更，`api/openapi.yaml` 未修改；已核对 34 条路径均与 Gin 路由注册一致，无需补文档。
- 新增或修改函数、结构体：本次为纯 Markdown 变更，没有 Go 函数或结构体改动，注释规范不涉及额外变更。

## 已知限制与未验证内容

- Docker 守护进程不可用，`docker compose config --quiet` 无法执行，Compose 文件本次未改动。
- 未重新执行真实服务联调（PostgreSQL、Redis、RabbitMQ 容器内集成验证），条件式真实服务测试可能跳过。
- 迁移过程中发现三处与本次文档无关的既有问题，未在本次修改：`configs/config.yaml` 与 `internal/infrastructure/mq/rabbitmq.go` 的默认交换机、队列名仍为 `gcfeed.*`，而 `configs/config.docker.yaml` 使用 `videofeed.*`；`docker-compose.yaml` 的 `container_name` 仍为 `gcfeed-*`；`internal/infrastructure/persistence/migration/migration.go`、部分持久化文件与路由文件的注释存在编码乱码。这些属于存量问题，需要单独提交修复。

## 最终审计

参考提交全部文件重新枚举为 `docs/resume-highlights.md`，已按同名路径迁移为 [同路径文档](resume-highlights.md)，并新建本审计记录、更新 [迁移索引](migrations.md)。逐项确认每个文件、能力条目、测试、配置和文档：该提交不包含函数、类型、常量、测试、配置或接口变更，因此不存在行为缺失；文档中每一条保留的能力声明都能在 VideoFeed 代码中定位，无法完全成立的声明已改写或列入边界说明。

结论：来源提交的文档内容已完整迁移并按 VideoFeed 真实架构完成适配，未发现尚未迁移的文件或无法解释的行为差异；未执行成功的 Compose 校验与真实服务联调已在限制中列出。按用户要求，本次不创建提交。
