# Day 11：曝光上报、本地向量化与视频向量存储

## 今日目标

在 Day 10 视频发布异步分发的基础上，补齐观看行为采集和视频内容向量存储，为后续用户画像和推荐能力提供数据基础。迁移时适配当前 PostgreSQL、应用分层及消息拓扑，逐项保留原提交的行为，并通过真实服务验证并发、重复投递和故障回退。

本记录于 2026-09-20 整理，所引用的真实联调结果产生于 2026-09-19。

## 完成内容

- 迁移 `fa476283`：新增曝光与观看上报接口、行为流水、曝光聚合和画像事件发布，覆盖来源 17 个文件。
- 迁移 `98d04ee`：新增本地哈希向量化组件，保留模型标识、128 维输出、归一化和余弦相似度，覆盖来源 3 个文件。
- 迁移 `4272741`：接入视频发布后的独立向量消费队列和 PostgreSQL 存储，覆盖来源 14 个文件。
- 将旧仓库路径映射到当前 `domain`、`application`、`handler`、`infrastructure` 与 `router` 分层，补齐函数和结构体中文注释。
- 同步曝光接口 OpenAPI、配置及模块说明，记录逐文件与逐符号等价审计。
- 完成真实 PostgreSQL、RabbitMQ 和实际 API 进程联调，并通过 Linux 容器补齐此前受宿主 GCC 缺失影响的相关包竞态检测。

## 关键设计

### 曝光与观看上报

```text
POST /api/video-view-events
  -> JWT 认证，从上下文取得用户 ID
  -> 清洗并校验视频、场景、事件类型及观看时长
  -> PostgreSQL 事务
       检查视频处于已发布状态
       追加 video_view_events 流水
       exposed 事件原子更新 exposures 聚合
  -> 提交成功后尝试发布 ViewEventRecordedEvent
  -> 返回 201；消息发布失败不回滚已保存行为
```

请求示例：

```json
{
  "video_id": 1001,
  "scene": "timeline",
  "request_id": "req-1",
  "event_type": "exposed",
  "watch_ms": 0,
  "completed": false
}
```

| 字段 | 当前约束 |
| --- | --- |
| `video_id` | 正整数，且对应视频处于已发布状态 |
| `scene` | 去空白并转小写后非空，最多 32 个 UTF-8 字节；不限定为 Feed 查询支持的场景 |
| `request_id` | 可省略，去首尾空白后最多 64 个 UTF-8 字节；用于关联请求 |
| `event_type` | 去空白并转小写后为 `exposed`、`play`、`complete` 或 `skip` |
| `watch_ms` | 非负整数，缺省为 0 |
| `completed` | 缺省为 false；`complete` 事件强制保存为 true |

成功响应包含 `event`，只有 `exposed` 事件附带 `exposure` 聚合；空 `request_id` 不输出。参数或 JSON 错误返回 400，认证失败返回 401，视频缺失或未发布返回 404，存储失败返回 500，错误格式统一为 `{"error":"..."}`。

`request_id` 不是幂等键。相同请求重复上报仍追加流水，重复曝光仍累加次数；`play`、`complete`、`skip` 不更新曝光聚合。画像事件包含已保存流水标识、用户、视频、观看行为、UTC 时间和可选曝光次数，本轮提供发布与消费接口，尚未启动用户画像计算消费者。

### 曝光事务与数据库适配

`video_view_events` 保存行为流水，`exposures` 通过 `(user_id, video_id)` 唯一约束保存曝光事实。流水和聚合位于同一事务，聚合更新失败时先插入的流水也回滚。

迁移将 MySQL 的 `VALUES(...)` 改为 PostgreSQL `EXCLUDED`，使用 `exposures.exposure_count + 1` 原子累加，保留首次曝光时间，更新最近时间和场景。新增索引采用表专属名称，避免与现有 `feed_inbox` 索引在 PostgreSQL 中重名。

本轮没有修改既有 Redis WATCH、事务管道或缓存计数逻辑。

### 本地哈希向量化

`HashNgramVectorizer` 实现 `Vectorizer` 接口，模型标识为 `hash-ngram-v1`，默认输出 128 维向量。

1. 文本转小写并去首尾空白，保留 Unicode 字母和数字，将空白、标点与符号折叠为空格。
2. 对词项生成带 `tok:` 前缀的特征，对去空格后的字符序列生成二元、三元特征。
3. 使用 FNV-1a 64 位哈希决定维度位置与正负号，累加特征权重。
4. 对非零向量做 L2 归一化；空文本返回零向量。

每次调用分配独立切片，共享向量器可并发使用。`CosineSimilarity` 支持未归一化向量，维度不一致返回 `ErrDimensionMismatch`，零范数返回 0。

该组件生成确定性文本哈希特征，尚不具备训练语义模型的表达能力。本轮也没有引入向量数据库或向量检索接口。

### 视频发布与独立向量消费

```text
POST /api/videos
  -> 保存视频，发布 PublishedEvent
  -> videofeed.video / video.published
       -> videofeed.video.published
            -> 原有 FanoutWorker：关注流分发与缓存预热
       -> videofeed.video.embedding
            -> VideoEmbeddingWorker
            -> 拼接标题与简介，计算文本哈希和向量
            -> 按 video_id + model 保存 video_embedding
```

两个持久化队列绑定同一视频交换机和路由键，各自取得一份消息，避免向量生成与关注分发争抢任务。向量消费者在 RabbitMQ 连接成功后启动，独立于 Redis；订阅失败记录日志。

标题和简介分别去首尾空白后用换行连接，`TextHash` 对拼接文本去首尾空白后计算 SHA-256。空事件或非正视频 ID 被忽略；向量序列化或保存失败返回错误，由消息层重新入队。成功逐条确认，非法 JSON 拒绝且不重入队。

| 表 | 关键字段与约束 |
| --- | --- |
| `video_view_events` | 主键 id；用户、视频、场景、请求编号、事件类型、观看时长、完播状态和创建时间 |
| `exposures` | 用户与视频唯一；首次/最近曝光时间、曝光次数、最近场景和创建/更新时间 |
| `video_embedding` | 视频与模型复合主键及唯一约束；维度、JSON 向量、文本哈希和创建/更新时间 |

同视频同模型通过 PostgreSQL 冲突更新覆盖向量、维度、哈希和 `updated_at`，保留 `created_at`；不同模型分别存储。重复事件不增加记录，但仍重新计算和保存，不以文本哈希或事件编号跳过处理。查询恢复时保留 JSON 字符串，不自动解析到领域对象的向量切片。

## 文件与配置对应

| 能力 | 当前项目位置 |
| --- | --- |
| 曝光领域、应用服务与 HTTP 处理 | `internal/domain/exposure/`、`internal/application/exposure/`、`internal/handler/exposure/` |
| 曝光事务与表模型 | `internal/infrastructure/persistence/exposure/` |
| 文本向量器与向量领域对象 | `internal/domain/embedding/` |
| 向量服务与消息消费者 | `internal/application/embedding/` |
| 向量仓储与表模型 | `internal/infrastructure/persistence/embedding/` |
| 消息拓扑及依赖装配 | `internal/infrastructure/mq/rabbitmq.go`、`internal/router/register.go` |
| 对外接口文档 | `api/openapi.yaml`，本轮新增曝光上报定义 |
| 隔离测试环境与实际应用联调 | `scripts/compose.exposure-test.yaml`、`scripts/verify_exposure_live.py` |

| `rabbitmq` 下的配置键 | 默认值 | 环境变量 |
| --- | --- | --- |
| `exposure_exchange` | `videofeed.exposure` | `RABBITMQ_EXPOSURE_EXCHANGE` |
| `view_event_recorded_queue` | `videofeed.exposure.view_event_recorded` | `RABBITMQ_VIEW_EVENT_RECORDED_QUEUE` |
| `view_event_recorded_routing` | `exposure.view_event_recorded` | `RABBITMQ_VIEW_EVENT_RECORDED_ROUTING` |
| `video_embedding_queue` | `videofeed.video.embedding` | `RABBITMQ_VIDEO_EMBEDDING_QUEUE` |

开发与容器配置分别位于 `configs/config.yaml`、`configs/config.docker.yaml`，新增字段显式绑定环境变量，旧 YAML 缺少字段时也可覆盖。部署沿用现有 PostgreSQL、Redis、RabbitMQ、数据卷、健康检查与依赖顺序，无新增运行服务或 Go 依赖。API、Web 和宿主 PostgreSQL 端口分别为 9000、5173、5433；容器环境变量需显式传入，消息账号必须与实际服务一致。

## 测试与真实联调

迁移保留来源测试，并补充领域边界、错误映射、上下文取消、并发重复上报、事务回滚、向量切片隔离、重复覆盖、双队列投递和消费失败重试。三次来源迁移均有逐文件、逐声明审计，而非仅以编译或测试通过判断完整性。

代码迁移阶段已执行并通过：

- `gofmt`、`go test ./...`、`go vet ./...`。
- `git diff --check`、Compose 配置检查、OpenAPI YAML 解析及 `operationId` 唯一性检查。
- 曝光请求、成功响应和错误响应的 OpenAPI 示例校验。
- Go 1.25.14 / GCC 14.2.0 Linux 容器内相关包 `go test -race`；宿主 Windows 未安装 GCC。

竞态检测与真实服务测试分开执行：竞态检测轮次没有配置真实服务连接；真实联调使用 Windows 原生 Go 1.26.0、Docker 29.6.2、PostgreSQL 17.11、RabbitMQ 3.13.7 和 Redis 7.4.11。

| 真实验证范围 | 结果 |
| --- | --- |
| 曝光数据库并发 | 32 个并发事务与后续重复请求正确累加；唯一约束和失败回滚生效 |
| 实际曝光 HTTP 链路 | 注册、登录、发布视频后完成四类事件上报；JWT 身份不被请求字段覆盖；400/401/404/500 符合实现 |
| 曝光数据与消息 | 初始 36 条流水对应 36 条消息并逐字段核对；最终 40 条流水、曝光计数 33 |
| 消息故障与恢复 | 停机期间两条流水仍保存但未产生消息；重启后停机前持久化消息仍在，重启 API 后发布恢复 |
| 向量数据库并发 | 同视频同模型保持单行，模型间隔离；保留创建时间，JSON 约束、唯一约束和失败恢复通过 |
| 双队列消费 | 同一发布同时到达关注分发与向量队列；向量处理失败独立重投，非法消息丢弃，取消后无遗留消费者 |
| 实际向量链路 | 视频发布自动保存 128 维向量和正确哈希；重复投递与内容更新正确覆盖 |
| 向量故障恢复 | 表约束拒绝更新时保留旧记录，解除后重新投递并成功保存；Redis 不可达时仍能生成向量 |
| 清理 | 测试 API 进程、连接、容器、网络和数据卷清理完成 |

曝光独立联调包含 10 个检查点；向量扩展联调包含 15 个检查点，并回归曝光链路。两份结果均为 `passed`，对应真实数据库与消息测试没有跳过。验证中修正了测试环境的 RabbitMQ 端口就绪检查、重启后的动态端口发现，以及管理接口统计延迟处理。

仓库根目录可复现：

```powershell
# 曝光链路
python scripts/verify_exposure_live.py

# 向量链路及曝光回归
python scripts/verify_exposure_live.py --embedding --output docs/embedding-live-results.json
```

脚本启动隔离测试服务并编译、运行实际 `cmd/server`，通过真实 TCP HTTP 请求验证，不以模拟仓储替代真实服务。详情见[曝光验证报告](exposure-live-verification.md)、[曝光结果](exposure-live-results.json)和[向量扩展结果](embedding-live-results.json)。这些检查不等同于生产容量或长期稳定性测试。

本次 Day 11 撰写仅新增 Markdown，复核事实、路径、链接及差异，不重复运行已通过的代码和真实服务测试。

## 提交记录

| 主题 | 当前仓库提交 |
| --- | --- |
| 曝光基础设施 | `686db1e feat(infra): 增加曝光事件存储与消息基础设施` |
| 曝光应用接口 | `6ce8918 feat(exposure): 接入曝光上报与画像事件发布` |
| 曝光测试 | `891b025 test(exposure): 覆盖并发回滚与真实服务故障恢复` |
| 曝光文档 | `faf18ee docs(exposure): 记录迁移审计与真实服务验证结果` |
| 本地向量化 | `70f6f42 feat(embedding): 迁移本地哈希向量化组件` |
| 向量存储基础设施 | `df0720d feat(infra): 增加视频向量存储与独立消息队列` |
| 向量应用链路 | `ce693d8 feat(embedding): 接入视频发布后的向量生成链路` |
| 向量存储测试 | `2b525b7 test(embedding): 验证向量覆盖存储与真实消息重试` |
| 向量存储文档 | `85d6aca docs(embedding): 记录存储链路迁移审计与联调证据` |

详细来源映射见[曝光迁移审计](migration-fa476283.md)、[本地向量化审计](migration-98d04ee.md)、[向量存储审计](migration-4272741.md)。模块行为见[Feed 模块](modules/feed.md)与[视频向量模块](modules/embedding.md)。

## 当前边界与后续计划

曝光聚合、画像事件和视频向量存储已具备基础能力，但尚未构成完整推荐闭环：当前没有画像计算消费者、曝光去重接入、向量召回、排序或推荐质量评估。

数据库提交和消息发送仍不是同一事务。发布失败不回滚已保存数据，也没有发件箱、发布确认或自动重连；消息故障期间可能缺失对应事件。消费失败直接重新入队，尚无退避、死信与限次策略。

视频向量未按事件版本排序，晚到旧内容可能覆盖新内容；没有历史视频回填、删除联动清理或模型升级重建流程。本地哈希特征也不能代表语义推荐质量。

- 设计事件发件箱、发布确认与重连机制，完善失败消息退避和死信重放。
- 增加向量更新版本保护、历史回填与删除清理，明确模型升级策略。
- 基于观看事件实现可幂等更新的用户画像，再接入候选召回和排序评估。
- 增加向量生成延迟、消息积压、存储失败和回退行为的监控，开展容量与持续故障测试。
