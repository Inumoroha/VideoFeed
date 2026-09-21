# 消息中心迁移审计

来源：`C:/Users/HP/Desktop/GCFeed`，提交 `7a62b5c6418ea025a89e2d594810b56001310296`；目标：VideoFeed。按用户要求分为基础设施与依赖、业务逻辑、测试、文档四个主题提交。

## 逐文件迁移表

| 原文件 | 目标文件 | 方式 |
| --- | --- | --- |
| `apps/api/configs/config.docker.yaml` | `configs/config.docker.yaml` | 适配已有层 |
| `apps/api/configs/config.yaml` | `configs/config.yaml` | 适配已有层 |
| `apps/api/internal/application/interaction/service.go` | `internal/application/interaction/service.go` | 适配已有层 |
| `apps/api/internal/application/message/service.go` | `internal/application/message/service.go` | 新建 |
| `apps/api/internal/application/relation/service.go` | `internal/application/relation/service.go` | 适配已有层 |
| `apps/api/internal/domain/interaction/repository.go` | `internal/domain/interaction/repository.go` | 适配已有层 |
| `apps/api/internal/domain/message/entity.go` | `internal/domain/message/message.go` | 新建 |
| `apps/api/internal/domain/message/errors.go` | `internal/domain/message/error.go` | 新建 |
| `apps/api/internal/domain/message/repository.go` | `internal/domain/message/repository.go` | 新建 |
| `apps/api/internal/infra/config/entity.go` | `internal/config/config.go` | 适配已有层 |
| `apps/api/internal/infra/persistence/interaction/gorm.go` | `internal/infrastructure/persistence/interaction/repository.go` | 适配已有层 |
| `apps/api/internal/infra/persistence/message/gorm.go` | `internal/infrastructure/persistence/message/repository.go` | 新建 |
| `apps/api/internal/infra/persistence/message/model.go` | `internal/infrastructure/persistence/message/model.go` | 新建 |
| `apps/api/internal/infra/persistence/migration/migration.go` | `internal/infrastructure/persistence/migration/migration.go` | 适配已有层 |
| `apps/api/internal/interfaces/http/message/dto.go` | `internal/handler/message/dto.go` | 新建 |
| `apps/api/internal/interfaces/http/message/handler.go` | `internal/handler/message/handler.go` | 新建 |
| `apps/api/internal/interfaces/http/middleware/jwt_auth.go` | `internal/middleware/internal_token.go` | 新建，拆分中间件 |
| `apps/api/internal/interfaces/http/router/router.go` | `internal/router/register.go + internal/router/message.go` | 修改注册，新增消息路由适配器 |
| `apps/api/test/interaction_api_test.go` | `internal/application/interaction/message_test.go` | 新建 |
| `apps/api/test/message_api_test.go` | `internal/handler/message/api_flow_test.go` | 新建 |
| `apps/api/test/relation_api_test.go` | `internal/application/relation/message_test.go` | 新建 |

## 架构适配和行为审计

迁移前已检查根 `AGENTS.md`、目录树、`main` 分支、干净工作区、Go 模块及版本、配置加载与环境变量绑定、PostgreSQL/GORM、迁移咨询锁、Redis WATCH/事务、RabbitMQ 和 Gin 路由。执行了源提交 `git show --stat`、`git show --name-only`、`git show --format=fuller`，并分批读取全部 21 个文件的 diff，避免整体输出截断造成遗漏。

| 项目 | 核对结论与对应位置 |
| --- | --- |
| 包和模块 | 原 `GCFeed/internal/infra` 映射至 `github.com/inumoroha/video-feed/internal/infrastructure`，HTTP 层映射至 `internal/handler`，JWT 复用 `internal/auth/jwt`；新消息包均命名 `message` |
| 互动接口 | 源 `Repository.GetVideoAuthorID` 映射至目标可选扩展 `NotificationRepository.GetVideoAuthorID`；补齐 `SetActionWithDelta`、`CreateCommentWithDelta`。原接口返回双计数，新接口另加事务变化量，不修改旧签名、幂等快照或既有状态转换行为 |
| 应用服务 | `WithMessageWriter`、`MessageWriter.CreateFromEvent` 保留；源 `setActionAsync/setActionSync` 通知分别合入现有 `setAction` 和 `setActionAndNotify`；生产仓储实现扩展能力，旧测试/自定义仓储保持兼容 |
| 数据库 | PostgreSQL 布尔已读字段、两个按用户限定的可空唯一键、事务失败回滚后读取原消息；新增 pgx 23505 识别，保留 GORM 与文本兼容识别。索引改名 `idx_message_user_*`、`uk_message_user_*`；补齐原列表索引缺少的 user_id 列 |
| 迁移 | 源 `AutoMigrate` 注册消息表映射到 `migration.migrateTables`，保留目标 `AutoMigrate` 的事务级咨询锁、失败回滚和 API/worker 并发启动保护 |
| Redis | 本提交不修改 Redis 文件。保留用户行为键 WATCH、事务更新状态与分片、冲突返回错误、幂等 delta=0。消息触发使用原子操作完成后返回的 delta，未用非原子读改写替代 |
| RabbitMQ | 本提交不改连接或拓扑。继续复用配置的持久化 topic 交换机、队列、路由键、互斥发布、手动 Ack、无效 JSON Nack 不重入、业务失败 Nack 重入、上下文取消与连接关闭。既有实现没有 publisher confirm、自动重连或退避，本次不声称添加这些能力 |
| 失败回退 | 缓存失败返回原互动错误；发布失败同步保存后按实际 delta 通知；保存失败不通知。消息或作者查询失败不传播到互动请求；关注回填失败仍返回原错误，重试成功后通知 |
| 配置 | 原 `InternalConfig.Token` YAML 解码映射到目标 `mapstructure`；两个配置文件增加 `internal.token`，`env.go` 绑定 `INTERNAL_TOKEN`。不使用源示例令牌作有效默认凭据，空值默认拒绝内部创建，业务通知不受影响 |
| 部署 | Compose 向 API 传入令牌；Kubernetes Secret 和 API 环境绑定同步增加。原提交无新增镜像、卷、健康检查或依赖顺序；复用现有 PostgreSQL/Redis/RabbitMQ，API 9000、宿主 PostgreSQL 5433 |
| 鉴权 | 新建 `middleware/internal_token.go` 保留空配置拒绝、去空白和常量时间比较；错误体从源 `message` 字段适配为新仓库统一 `error` 字段。既有内部推荐接口保持原行为 |
| 文档 | `api/openapi.yaml` 增加四个实际方法及请求、响应、状态码、令牌方案；同步 README、模块索引、消息模块、部署和迁移索引；前端与审核通知未被标为已实现 |

## 逐符号清单

下列清单覆盖源提交新增消息文件的全部类型、函数、常量和错误值。除表中明确列出的适配项，函数逻辑、字段、值和长度校验保留；不是仅以测试通过作为迁移依据。

### `internal/domain/message/entity.go` → `internal/domain/message/message.go`

- 类型/函数/错误：`Message`, `Cursor`, `New`, `Restore`, `NormalizeType`。
- 常量逐值：`TypeLike="LIKE"`, `TypeComment="COMMENT"`, `TypeFollow="FOLLOW"`, `TypeSystem="SYSTEM"`, `MaxTitleLength=128`, `MaxContentLength=1024`, `MaxEventIDLength=64`, `MaxIdempotencyKeyLength=128`, `MaxLimit=100`。
- 状态：已逐项映射；数据库及鉴权适配见上表。

### `internal/domain/message/errors.go` → `internal/domain/message/error.go`

- 类型/函数/错误：`ErrInvalidUserID`, `ErrInvalidMessageID`, `ErrInvalidLimit`, `ErrInvalidCursor`, `ErrInvalidMessageType`, `ErrEmptyTitle`, `ErrTitleTooLong`, `ErrEmptyContent`, `ErrContentTooLong`, `ErrEventIDTooLong`, `ErrIdempotencyKeyTooLong`, `ErrMessageNotFound`。
- 状态：已逐项映射；数据库及鉴权适配见上表。

### `internal/domain/message/repository.go` → `internal/domain/message/repository.go`

- 类型/函数/错误：`Repository`。
- 状态：已逐项映射；数据库及鉴权适配见上表。

### `internal/application/message/service.go` → `internal/application/message/service.go`

- 类型/函数/错误：`Service`, `CreateResult`, `ListResult`, `UnreadStat`, `MarkReadResult`, `cursorPayload`, `New`, `CreateFromEvent`, `List`, `CountUnread`, `MarkRead`, `normalizeLimit`, `parseCursor`, `encodeCursor`, `ErrLoadMessageFailed`, `ErrSaveMessageFailed`, `ErrUpdateMessageFailed`。
- 常量逐值：`defaultMessageLimit=20`。
- 状态：已逐项映射；数据库及鉴权适配见上表。

### `internal/infra/persistence/message/gorm.go` → `internal/infrastructure/persistence/message/repository.go`

- 类型/函数/错误：`Repository`, `New`, `Create`, `ListByUser`, `CountUnread`, `MarkRead`, `findExisting`, `restore`, `optionalString`, `stringValue`, `isDuplicateKeyError`。
- 状态：已逐项映射；数据库及鉴权适配见上表。

### `internal/infra/persistence/message/model.go` → `internal/infrastructure/persistence/message/model.go`

- 类型/函数/错误：`MessageModel`, `TableName`。
- 状态：已逐项映射；数据库及鉴权适配见上表。

### `internal/interfaces/http/message/dto.go` → `internal/handler/message/dto.go`

- 类型/函数/错误：`createMessageRequest`, `markReadRequest`, `messageResponse`, `messageListResponse`, `unreadStatResponse`, `markReadResponse`。
- 状态：已逐项映射；数据库及鉴权适配见上表。

### `internal/interfaces/http/message/handler.go` → `internal/handler/message/handler.go`

- 类型/函数/错误：`Handler`, `New`, `List`, `CountUnread`, `MarkRead`, `Create`, `userIDFromContext`, `parseLimit`, `listResponseFromResult`, `responseFromDomain`, `writeMessageError`, `isBadRequestError`。
- 状态：已逐项映射；数据库及鉴权适配见上表。


### 已有文件中的新增或修改符号

| 源文件主题 | 符号及审计结果 |
| --- | --- |
| 互动应用 | `Service.messageWriter`、`MessageWriter`、`WithMessageWriter`、`CreateComment`、`setActionAsync`、`setActionSync`、`notifyLike`、`notifyComment` 全部接入目标；保留 delta>0、有效点赞、自赞/自评排除、作者查询失败跳过、事件键和通知文本 |
| 关系应用 | `Service.messageWriter`、`MessageWriter`、`WithMessageWriter`、`setFollow`、`notifyFollow` 全部接入；回填后才通知、取关不通知、确定性事件键和错误忽略不变 |
| 互动领域/仓储 | `GetVideoAuthorID` 在扩展接口与 GORM 中实现；使用目标 `domainvideo.StatusPublished`（值为 2），不能照抄旧库状态数字；查询错误复用 `mapVideoError` |
| 配置 | `Config.Internal`、`InternalConfig.Token` 完整映射，额外补齐环境绑定与部署配置 |
| 迁移 | `AutoMigrate` 的消息表注册已转至目标 `migrateTables` |
| 中间件 | `NewInternalTokenAuth` 完整迁移，错误 JSON 字段适配统一格式 |
| 路由 | `Register` 组装共享消息服务，`MessageWriter`、`NewMessageWriter`、`CreateFromEvent` 迁移至 `router/message.go`，`registerMessageRoutes` 注册四个接口 |

## 测试逐项映射

| 源新增测试/辅助结构 | 目标与覆盖 |
| --- | --- |
| `TestMessageAPIFlow` | `internal/handler/message/api_flow_test.go`：四类消息、201/200、原结果重放、倒序和游标、用户隔离、未读数、去重指定已读、全部已读 |
| `TestMessageAPIValidation` | 同文件：三个用户接口未认证、limit=0、坏游标、非法消息 ID、非法消息类型/用户/标题、缺失或错误内部令牌 |
| 消息夹具 | `messageAPIResponse`、`messageListAPIResponse`、`unreadStatAPIResponse`、`markReadAPIResponse`、`memoryMessageRepo`、`memoryMessageUniqueKey`、`newMemoryMessageRepo`、`Create`、`ListByUser`、`CountUnread`、`MarkRead`、`cloneMessage`、`newMessageRouter`、`performInternalMessageRequest` 均保留；JWT 工厂和共享请求辅助函数适配目标配置/签名 |
| `TestInteractionMessageWriter` | `internal/application/interaction/message_test.go`：原请求序列与通知数量、作者、类型断言保留；`memoryInteractionMessageWriter`、`memoryInteractionMessage`、构造函数、`CreateFromEvent`、`Messages` 保留；原 `memoryInteractionRepo.GetVideoAuthorID` 映射至 `notificationRepo.GetVideoAuthorID`，并补充变化量模拟 |
| `TestRelationMessageWriter` | `internal/application/relation/message_test.go`：原关注、重放、取关及消息断言保留；`memoryRelationMessageWriter`、`memoryRelationMessage`、构造函数、`CreateFromEvent`、`Messages` 保留 |
| 新增失败测试 | 异步成功/重放、缓存失败、MQ 失败同步回退、回退保存失败、自赞/自评、作者查询失败、通知写入失败、关注回填失败后重试；消息仓储错误映射、唯一冲突回滚查询、普通故障后重试、用户范围已读 SQL |
| 新增配置/边界测试 | 令牌空配置与去空白匹配、旧配置环境绑定；全部消息类型和长度常量、UTF-8 字节边界；互动幂等快照 delta=0、首次提交正 delta、回滚零 delta、评论唯一冲突零 delta、已发布视频作者查询 |
| 新增真实并发测试 | `internal/infrastructure/persistence/message/live_test.go`：独立 schema，24 并发事件去重及请求键去重、空键、跨用户、相同时间游标、重复已读；本次未运行到数据库，原因见验证限制 |

消息领域、应用服务、HTTP 处理器与仓储共 31 个函数的正文与源提交进行了去除注释和空白的逐函数比对，全部一致；唯一键错误识别函数单独核对 PostgreSQL 扩展。已有互动/关系函数按上述映射人工核对。

## 验证记录（2026-09-21）

- `gofmt`：所有新增/修改 Go 文件已格式化，中文函数和结构体注释已逐项补齐。
- `go test ./...`：通过，包含所有原提交新增测试和额外回归测试；依赖真实服务的条件测试可能跳过。
- `go vet ./...`：通过。
- `git diff --check`：通过。
- `docker compose config --quiet`：通过；这是静态配置检查，不需要 Docker 引擎运行。
- OpenAPI YAML：解析通过，38 个 operationId 全局无重复，4 个新增方法与实际路由一致，18 个新增请求/响应示例通过 schema 验证；两个应用配置、Compose、Kubernetes YAML 也均解析通过。
- `go test ./internal/infrastructure/persistence/message -run TestMessagePostgresLive -v`：明确 SKIP，未配置 `VIDEOFEED_TEST_POSTGRES_URL`。
- Docker 引擎不可用，未进行 PostgreSQL/Redis/RabbitMQ 容器联调或真实 HTTP 部署验证。未将 SQL mock 或测试夹具结果描述为真实服务验证。

## 最终审计结论和限制

重新从参考提交枚举全部 21 个文件，与上表逐项对应；源新增类型、函数、常量、错误值、四个新增测试和两个配置项没有发现遗漏。迁移适配和原有行为差异已明确记录，包括目标互动状态转换保持原样、内部令牌默认关闭、统一错误字段及 PostgreSQL 索引。

消息仍为尽力通知，不新增原提交不存在的重试/补偿保证。代码、测试、配置和文档迁移已完成；真实数据库并发和容器联调尚未验证，因此不宣称“全环境完整验证”。按用户要求分主题提交，未推送远程。
