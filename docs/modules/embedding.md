# 视频文本向量模块

发布视频成功后，现有 `PublishedEvent` 同时路由到关注分发队列和独立向量队列。`internal/application/embedding` 的消费者调用本地哈希向量器，将标题、简介生成的向量保存到 PostgreSQL `video_embedding` 表。本模块没有新增 HTTP 接口。

## 1. 模块职责

- `internal/domain/embedding`：Vectorizer、hash-ngram-v1、视频文本拼接、SHA-256 文本哈希、VideoEmbedding 与 Repository。
- `internal/application/embedding`：NewService、GenerateForPublishedVideo、VideoEmbeddingWorker。
- `internal/infrastructure/persistence/embedding`：GORM 模型与 PostgreSQL 仓储。
- `internal/infrastructure/mq/rabbitmq.go`：独立队列声明、ConsumeVideoPublishedForEmbedding 和共享消费循环。
- `internal/infrastructure/persistence/migration`：API 和 worker 共用向量表迁移；`cmd/worker` 启动独立向量消费者。worker 要求配置 Redis 地址，但向量生成不调用 Redis。

## 2. 接口设计

没有独立 HTTP 接口；由视频发布消息触发。

## 3. 数据表设计

`video_embedding` 使用 `(video_id, model)` 复合主键，并保留同字段唯一索引。其余字段为 dimension、embedding_json（PostgreSQL json）、text_hash、created_at、updated_at；有 model/updated_at 索引。不同模型独立存储，同视频同模型原子覆盖向量、维度、哈希和更新时间，首次创建时间不变。

标题和简介分别去首尾空白，用换行连接；内容哈希为拼接文本去首尾空白后的 SHA-256。默认模型 hash-ngram-v1，128 维；token 与二元/三元字符特征经过 FNV-1a 哈希累加和 L2 归一化。空文本可保存零向量。

重复发布不会增加同模型记录，但仍重新计算和覆盖保存。TextHash 用于描述内容，不作为跳过写入或事件排序条件。旧事件晚到可能覆盖较新内容。查询返回模型、维度、JSON、哈希和时间，不自动解析 JSON 到领域对象的 Embedding 切片。

## 4. 业务规则

`rabbitmq.video_embedding_queue` 默认 `videofeed.video.embedding`；支持环境变量 `RABBITMQ_VIDEO_EMBEDDING_QUEUE`，包括旧 YAML 缺少该键的情况。两个配置文件 `configs/config.yaml`、`configs/config.docker.yaml` 均已补充。

向量队列为持久化队列，绑定已有 VideoExchange/VideoPublishedRouting，与关注分发队列名称不同。成功处理逐条确认，非法 JSON 拒绝且不重新入队，向量序列化或保存失败重新入队。上下文取消结束订阅；沿用现有连接和资源关闭行为。没有退避、死信队列或自动重连，持续故障会重复投递。

API 初始化 RabbitMQ 失败时记录日志并禁用发布器；独立 worker 初始化或订阅失败会返回错误并关闭已取得资源；视频发布接口仍遵守既有消息失败回退行为。没有持久化发件箱或历史回填，不能保证故障期间发布的视频最终一定生成向量。没有新增向量检索、语义模型或画像消费接口。

## 5. 测试建议

沿用根目录 Compose 中 PostgreSQL、Redis、RabbitMQ 的数据卷、健康检查和依赖顺序，向量功能没有新增 Go 依赖，消费者由已有独立 worker 运行。API 9000、Web 5173、宿主 PostgreSQL 5433；运行 `docker compose up -d --build`。自定义队列环境变量用于容器部署时需同时传入 API 和 worker 容器；RabbitMQ 账号仍需与实际服务一致。

常规验证：`go test ./...`、`go vet ./...`、`git diff --check`、`docker compose config --quiet`。

真实服务验证：

```powershell
python scripts/verify_exposure_live.py --embedding --output docs/embedding-live-results.json
```

此命令启动独立 PostgreSQL/RabbitMQ/Redis，执行两个向量真实测试和既有曝光真实测试，启动实际 API 验证视频发布、消息双投、128 维存储、重放覆盖、故障重试及 Redis 不可达场景，然后回归曝光链路。退出时清理测试资源。`--embedding` 未指定时保留原曝光验证入口。

完整来源映射、审计和结果见 [迁移报告](../migration-4272741.md)；机器可读结果见 [真实服务证据](../embedding-live-results.json)。

## 6. 前端接入点

前端通过发布视频间接触发向量生成；推荐场景消费向量排序，没有向量管理页面。
