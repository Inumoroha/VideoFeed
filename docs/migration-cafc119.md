# cafc119 推荐流迁移审计

来源：`C:/Users/HP/Desktop/GCFeed`，提交 `cafc11932ee9e20e0908186b13a81b0a41b9b4fe`。目标：`C:/Users/HP/Desktop/VideoFeed`，起始分支 `main`，工作区干净。

## 逐文件迁移表

| 原文件 | 目标文件 | 新建 | 对应关系及适配 | 测试与文档 |
| --- | --- | --- | --- | --- | --- |
| `apps/api/internal/application/feed/service.go` | `internal/application/feed/service.go` | 否 | Service.repo、WithRecommender、RecommendStrategy、Recommender、clientContextValue；New 对应 NewService；推荐策略拆入 recommendation.go | 推荐单元、接口和仓储测试；api/openapi.yaml；docs/modules/recommendation.md |
| `apps/api/internal/application/recommendation/service.go` | `internal/application/recommendation/service.go` | 是 | 所有声明同名保留；包名改为 recommendation | 推荐单元、接口和仓储测试；api/openapi.yaml；docs/modules/recommendation.md |
| `apps/api/internal/domain/recommendation/entity.go` | `internal/domain/recommendation/recommendation.go` | 是 | 所有声明同名保留；包名改为 recommendation | 推荐单元、接口和仓储测试；api/openapi.yaml；docs/modules/recommendation.md |
| `apps/api/internal/domain/recommendation/errors.go` | `internal/domain/recommendation/error.go` | 是 | 所有声明同名保留；包名改为 recommendation | 推荐单元、接口和仓储测试；api/openapi.yaml；docs/modules/recommendation.md |
| `apps/api/internal/domain/recommendation/repository.go` | `internal/domain/recommendation/repository.go` | 是 | 所有声明同名保留；包名改为 recommendation | 推荐单元、接口和仓储测试；api/openapi.yaml；docs/modules/recommendation.md |
| `apps/api/internal/infra/persistence/recommendation/gorm.go` | `internal/infrastructure/persistence/recommendation/repository.go` | 是 | Repository 及全部辅助函数同名保留；video → videos；VALUES → EXCLUDED；计数限定 exposures；复用已有曝光模型和唯一索引 | 推荐单元、接口和仓储测试；api/openapi.yaml；docs/modules/recommendation.md |
| `apps/api/internal/interfaces/http/recommendation/dto.go` | `internal/handler/recommendation/dto.go` | 是 | 所有声明同名保留；包名改为 recommendation | 推荐单元、接口和仓储测试；api/openapi.yaml；docs/modules/recommendation.md |
| `apps/api/internal/interfaces/http/recommendation/handler.go` | `internal/handler/recommendation/handler.go` | 是 | 所有声明同名保留；包名改为 recommendation | 推荐单元、接口和仓储测试；api/openapi.yaml；docs/modules/recommendation.md |
| `apps/api/internal/interfaces/http/router/router.go` | `internal/router/register.go` | 否 | Register 对应现有 Register；内部路由装配提取 registerRecommendationRoutes；保留现有 GORM 注入和路由 | 推荐单元、接口和仓储测试；api/openapi.yaml；docs/modules/recommendation.md |
| `apps/api/test/recommendation_api_test.go` | `internal/handler/feed/recommendation_api_test.go` | 是 | 三个原测试及辅助声明保留；复用当前 feed 测试仓储；JWT 配置和 Handler 构造器适配 | 推荐单元、接口和仓储测试；api/openapi.yaml；docs/modules/recommendation.md |

## 架构与基础设施核对

- 当前分层为 handler → application → domain，infrastructure/persistence 实现仓储；配置通过 Viper 加载 YAML 和环境变量。
- PostgreSQL/GORM，videos.status 使用现有 StatusPublished；video_stat、video_embedding、video_view_events、exposures 已由 Register 迁移，曝光唯一键为 (user_id, video_id)。
- 推荐不新增 Redis 写操作、MQ 发布/消费、依赖、配置、环境变量或数据卷。复用 Feed 卡片/计数缓存及现有向量生成链路；现有 Redis 原子性和 RabbitMQ 连接、拓扑、确认、拒绝、重试与关闭代码不改动。
- Compose 已有 PostgreSQL、Redis、RabbitMQ 持久卷、健康检查和 api 的 service_healthy 依赖；API 9000，前端 5173，PostgreSQL 宿主机 5433。
- 原提交未修改配置、Compose 或文档；新增文档描述目标真实路径及行为，不复制旧项目部署配置。

## 验证状态

目标提交 10 个参考文件均已逐项映射；新增推荐服务、领域模型、仓储、HTTP 处理器、Feed 策略和测试覆盖。另补充了领域边界、应用失败重试、PostgreSQL 事务、并发曝光、Feed 缓存回源和真实路由测试。未迁移内容：原提交没有配置、Compose 或 RabbitMQ/Redis 文件。OpenAPI 已补充两个内部路径和推荐响应 schema。


- `gofmt`、`go test ./...`、`go vet ./...`、`git diff --check`、`docker compose config --quiet` 和 OpenAPI YAML 解析均通过。
- 已在用户目录安装 WinLibs GCC 16.1.0，并通过临时 CC 和 PATH 配置运行 `go test -race ./...`；全仓库通过，未报告竞态。
- `TestRecommendationPostgresLive` 仅在 `VIDEOFEED_TEST_POSTGRES_URL` 配置时执行；本次未配置专用 PostgreSQL 连接，因此真实并发数据库测试被跳过。
