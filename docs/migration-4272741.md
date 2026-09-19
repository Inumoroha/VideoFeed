# 视频向量存储迁移审计

来源：C:/Users/HP/Desktop/GCFeed，提交 `4272741acf71251cd8f102df4cf009a3468b1040`；来源 main/origin/main 包含该提交。目标 VideoFeed 在迁移前为 main/70f6f42，工作区干净。

## 逐文件迁移表

| 来源 | 目标 | 新建 | 适配、测试和文档 |
| --- | --- | --- | --- |
| `apps/api/configs/config.docker.yaml` | `configs/config.docker.yaml` | 否 | Viper mapstructure 与显式环境变量绑定；默认队列 videofeed.video.embedding；配置测试同目录 |
| `apps/api/configs/config.yaml` | `configs/config.yaml` | 否 | Viper mapstructure 与显式环境变量绑定；默认队列 videofeed.video.embedding；配置测试同目录 |
| `apps/api/internal/application/embedding/service.go` | `internal/application/embedding/service.go` | 是 | 包改为当前 embedding；保留全部函数、字段和错误；补齐中文注释 |
| `apps/api/internal/application/embedding/worker.go` | `internal/application/embedding/worker.go` | 是 | 包改为当前 embedding；保留全部函数、字段和错误；补齐中文注释 |
| `apps/api/internal/domain/embedding/entity.go` | `internal/domain/embedding/embedding.go` | 是 | 包改为当前 embedding；保留全部函数、字段和错误；补齐中文注释 |
| `apps/api/internal/domain/embedding/errors.go` | `internal/domain/embedding/error.go` | 否 | 包改为当前 embedding；保留全部函数、字段和错误；补齐中文注释 |
| `apps/api/internal/domain/embedding/hash_ngram_test.go` | `internal/domain/embedding/hash_ngram_test.go` | 否 | 迁移新增文本哈希测试、同向量比较函数和相似度附加断言；保留已有边界测试 |
| `apps/api/internal/domain/embedding/repository.go` | `internal/domain/embedding/repository.go` | 是 | 包改为当前 embedding；保留全部函数、字段和错误；补齐中文注释 |
| `apps/api/internal/infra/config/entity.go` | `internal/config/config.go` | 否 | Viper mapstructure 与显式环境变量绑定；默认队列 videofeed.video.embedding；配置测试同目录 |
| `apps/api/internal/infra/mq/rabbitmq.go` | `internal/infrastructure/mq/rabbitmq.go` | 否 | 当前通道/配置字段；独立持久化队列绑定原视频路由键；共享消费实现；同目录消息测试 |
| `apps/api/internal/infra/persistence/embedding/gorm.go` | `internal/infrastructure/persistence/embedding/repository.go` | 是 | GORM ON CONFLICT 使用 PostgreSQL 方言；原表名、json 字段、复合主键/唯一约束和更新时间保留；同目录仓储测试 |
| `apps/api/internal/infra/persistence/embedding/model.go` | `internal/infrastructure/persistence/embedding/model.go` | 是 | GORM ON CONFLICT 使用 PostgreSQL 方言；原表名、json 字段、复合主键/唯一约束和更新时间保留；同目录仓储测试 |
| `apps/api/internal/interfaces/http/router/router.go` | `internal/router/register.go` | 否 | 当前 GORM 迁移及依赖组装；仅 RabbitMQ 成功时启动 worker，不依赖 Redis |
| `apps/api/test/video_embedding_worker_test.go` | `internal/application/embedding/worker_test.go` | 是 | 原两项 worker 测试与全部内存仓储辅助函数迁移到应用层测试目录；New 映射 NewService |

## 关键语义

- 先读取 --stat、--name-only、--format=fuller 和全部 14 文件完整 diff；遵守当前 AGENTS.md。当前配置为 Viper，分层为 domain/application/infrastructure/router，数据库为 PostgreSQL，缓存为 Redis，消息服务为 RabbitMQ。
- 输入标题与简介分别去首尾空白并用换行拼接；TextHash 对拼接文本去首尾空白后做 SHA-256。模型与 128 维算法来自此前迁移。
- 无效事件或非正视频 ID 返回空结果且不落库；JSON 序列化失败/存储失败分别返回原有应用错误。向量化结果维度使用实际切片长度。
- 保存按 video_id + model 原子覆盖同模型维度、JSON、文本哈希与 updated_at，保留 created_at；不会先比较 hash，也不按消息 ID 去重。成功时 CreatedOrUpdated=true，包括相同内容重放。
- RestoreVideoEmbedding 保留数据库 JSON 字符串和维度，不解析为 Embedding 切片。NewVideoEmbedding 则复制切片，避免共享输入。
- 新视频向量队列与原关注分发队列绑定同一交换机和路由键，各获一份发布消息；成功 Ack(false)，非法 JSON Nack(false,false)，处理失败 Nack(false,true)。不添加发布确认、死信策略、退避或自动重连。
- worker 在 RabbitMQ 连接成功后启动，独立于 Redis；启动失败仅记录日志。原有连接初始化失败释放、Close、发布失败回退等保持。
- 不新增 HTTP 接口或改变现有请求/响应，OpenAPI 只核对无需修改；无 Redis 操作增量，不改变 WATCH 和事务原子性；沿用 Compose 依赖、卷和健康检查，无新增 Go 依赖。
- 当前事件没有版本排序保护；晚到旧事件可以覆盖新内容。此迁移不增加历史回填、视频可见性复核、向量检索或画像更新。

## 测试与审计

所有来源测试已保留，并补充：

| 覆盖 | 测试位置 |
| --- | --- |
| 原视频文本/哈希和相似度增量 | internal/domain/embedding/hash_ngram_test.go |
| 原两个生成/覆盖测试及辅助仓储 | internal/application/embedding/worker_test.go |
| 空事件、默认向量器、JSON 失败、存储错误、上下文传递、重放、订阅/处理重试 | internal/application/embedding/service_test.go |
| 文本空值、切片复制、字段清洗、恢复不解析 JSON | internal/domain/embedding/embedding_test.go |
| 环境变量覆盖旧配置 | internal/config/embedding_test.go |
| PostgreSQL 冲突目标/列、错误透传、回滚、字段恢复 | internal/infrastructure/persistence/embedding/repository_test.go |
| 真实并发覆盖、模型隔离、创建时间、JSON 约束、唯一约束、失败恢复及取消 | internal/infrastructure/persistence/embedding/live_test.go |
| 默认/自定义队列 | internal/infrastructure/mq/embedding_test.go |
| 真实双队列分发、独立重投、毒消息拒绝、取消/确认/清理 | internal/infrastructure/mq/embedding_live_test.go |
| 实际 API 视频发布到存储、覆盖更新、故障恢复、Redis 不可达及曝光回归 | scripts/verify_exposure_live.py --embedding |

旧 video_live_test.go、exposure_live_test.go 增加独立向量队列与清理，防止既有消息测试创建或遗留默认业务队列。验证脚本使用显式自定义队列，并等待 RabbitMQ 延迟生成的消费者统计，不以统计尚未就绪误判启动失败。

## 验证结果

- gofmt：新增和修改的 Go 文件均格式化。
- go test ./...、go vet ./...：通过。
- Go 1.25.14 / GCC Linux 容器内 go test -race：domain/embedding、application/embedding、persistence/embedding、mq、router 五个包通过。该轮未配置真实服务，真实服务验证为另外一次原生执行。
- git diff --check、新文件空白检查、函数/结构体中文注释检查、Python 语法与注释检查：通过。
- docker compose config --quiet：通过；现有部署未变更。
- OpenAPI、应用配置与 Compose YAML 解析通过；无 HTTP 路径、字段、状态码或认证变化，因此 OpenAPI 无新增定义。
- 真实 PostgreSQL / RabbitMQ：TestEmbeddingPostgresLive、TestEmbeddingQueueLive 与既有两个曝光真实测试均 PASS，没有 SKIP。
- 实际 cmd/server HTTP 链路：发布后保存 128 维单位向量，文本哈希正确；重复消息保持单行/创建时间，内容更新改变向量；数据库约束故障保留旧记录、解除后消费者重投恢复；Redis 不可达仍能保存向量。随后完整曝光回归通过。
- 真实联调最终 15 个检查点通过；测试 API、容器、网络和数据卷清理完成。来源引入的消息基础设施变更没有导致曝光回归。

真实结果文件：docs/embedding-live-results.json；复现命令：`python scripts/verify_exposure_live.py --embedding --output docs/embedding-live-results.json`。模块说明：docs/modules/embedding.md。

来源 14 文件及其函数、类型、错误、常量、配置、测试均有对应落点；未发现缺失迁移项。本次迁移约定的真实服务验证全部通过。验证完成后按用户要求拆分为基础设施、业务逻辑、测试和文档四个提交。

## 逐符号等价审计

以下文件在统一模块路径、包名、New→NewService 和导入排序，并忽略注释排版后，与来源完整代码一致：

- `internal/application/embedding/service.go`：`ErrSaveVideoEmbeddingFailed`, `ErrMarshalEmbeddingFailed`, `Service`, `GenerateVideoEmbeddingResult`, `NewService`, `GenerateForPublishedVideo`。
- `internal/application/embedding/worker.go`：`PublishedEventConsumer`, `VideoEmbeddingWorker`, `NewVideoEmbeddingWorker`, `Start`, `HandleVideoPublished`。
- `internal/domain/embedding/embedding.go`：`VideoEmbedding`, `BuildVideoText`, `TextHash`, `NewVideoEmbedding`, `RestoreVideoEmbedding`, `cloneVector`。
- `internal/domain/embedding/error.go`：`ErrDimensionMismatch`, `ErrVideoEmbeddingNotFound`。
- `internal/domain/embedding/hash_ngram_test.go`：`TestHashNgramVectorizerDeterministic`, `TestHashNgramVectorizerSimilarity`, `TestHashNgramVectorizerEmptyText`, `TestBuildVideoTextAndHash`, `TestCosineSimilarityDimensionMismatch`, `norm`, `sameVector`。
- `internal/domain/embedding/repository.go`：`Repository`。
- `internal/infrastructure/persistence/embedding/repository.go`：`Repository`, `New`, `SaveVideoEmbedding`, `FindVideoEmbedding`。
- `internal/infrastructure/persistence/embedding/model.go`：`VideoEmbeddingModel`, `TableName`。
- `internal/application/embedding/worker_test.go`：`memoryVideoEmbeddingRepo`, `newMemoryVideoEmbeddingRepo`, `SaveVideoEmbedding`, `FindVideoEmbedding`, `TestVideoEmbeddingWorkerWritesEmbedding`, `TestVideoEmbeddingWorkerUpdatesByVideoAndModel`, `cloneVideoEmbedding`, `videoEmbeddingKey`。

配置/消息/路由增量核对：

- VideoEmbeddingQueue 使用当前 mapstructure 标签；两个 YAML 与默认常量均使用 videofeed.video.embedding，并补充 RABBITMQ_VIDEO_EMBEDDING_QUEUE 绑定。
- ConsumeVideoPublished 保留原公开入口，委托 consumeVideoPublishedQueue；新增 ConsumeVideoPublishedForEmbedding 使用独立队列。共享函数的反序列化、确认、拒绝及重投与来源一致。
- ensureTopology 保留旧声明顺序及错误返回，增加持久化向量队列与视频交换机绑定；normalizeConfig 对新字段 TrimSpace 并补默认值。
- Register 增加 VideoEmbeddingModel 迁移、NewService 装配和 RabbitMQ 成功后的 worker.Start；放在 Redis 条件之外，未改任何 HTTP 路由。
- 原 TestHashNgramVectorizerSimilarity 新增同向量断言，TestBuildVideoTextAndHash、sameVector 与两个 worker 测试均完整迁移；此前 edge_test.go 保留。

实际验证环境与时间：2026-09-19T21:29:52+0800 至 2026-09-19T21:30:51+0800；go=go version go1.26.0 windows/amd64, docker=29.6.2, postgres=17.11, rabbitmq=3.13.7, redis=Redis server v=7.4.11 sha=00000000:0 malloc=jemalloc-5.3.0 bits=64 build=40ff01a501d8e4b6。
