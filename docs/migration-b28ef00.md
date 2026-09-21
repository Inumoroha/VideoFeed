# 播放优化提交迁移审计

原仓库：`C:\Users\HP\Desktop\GCFeed`。目标提交：`b28ef00fddbcfc6b75e772d7767493dbe3f0245d`。目标仓库：VideoFeed。

已执行 `git show --stat`、`git show --name-only`、`git show --format=fuller`，逐文件阅读全部 11 个文件的完整 diff。

## 迁移表

| 原路径 | 新路径 | 新建 | 适配、测试和文档 |
| --- | --- | --- | --- |
| `apps/api/internal/application/playback/service.go` | `internal/application/playback/service.go` | 是 | API/类型/函数同名；依赖领域仓储；`internal/application/playback/service_test.go`。 文档：播放模块说明及 OpenAPI。 |
| `apps/api/internal/domain/playback/entity.go` | `internal/domain/playback/playback.go` | 是 | 类型、接口、错误及常量同名同值；`internal/domain/playback/playback_test.go`。 文档：播放模块说明及 OpenAPI。 |
| `apps/api/internal/domain/playback/errors.go` | `internal/domain/playback/error.go` | 是 | 类型、接口、错误及常量同名同值；`internal/domain/playback/playback_test.go`。 文档：播放模块说明及 OpenAPI。 |
| `apps/api/internal/domain/playback/repository.go` | `internal/domain/playback/repository.go` | 是 | 类型、接口、错误及常量同名同值；`internal/domain/playback/playback_test.go`。 文档：播放模块说明及 OpenAPI。 |
| `apps/api/internal/infra/persistence/playback/gorm.go` | `internal/infrastructure/persistence/playback/repository.go` | 是 | 表 videos、播放专用唯一索引、pgx 错误适配；`internal/infrastructure/persistence/playback/{repository,live}_test.go`。 文档：播放模块说明及 OpenAPI。 |
| `apps/api/internal/infra/persistence/playback/model.go` | `internal/infrastructure/persistence/playback/model.go` | 是 | 表 videos、播放专用唯一索引、pgx 错误适配；`internal/infrastructure/persistence/playback/{repository,live}_test.go`。 文档：播放模块说明及 OpenAPI。 |
| `apps/api/internal/interfaces/http/playback/dto.go` | `internal/handler/playback/dto.go` | 是 | DTO/方法同名；认证上下文改为 internal/middleware；`internal/router/playback_test.go`。 文档：播放模块说明及 OpenAPI。 |
| `apps/api/internal/interfaces/http/playback/handler.go` | `internal/handler/playback/handler.go` | 是 | DTO/方法同名；认证上下文改为 internal/middleware；`internal/router/playback_test.go`。 文档：播放模块说明及 OpenAPI。 |
| `apps/api/internal/infra/persistence/migration/migration.go` | `internal/infrastructure/persistence/migration/migration.go` | 否 | AutoMigrate 内模型注册映射到 migrateTables；沿用事务/咨询锁；迁移及播放数据库测试。 文档：播放模块说明及 OpenAPI。 |
| `apps/api/internal/interfaces/http/router/router.go` | `internal/router/register.go + internal/router/playback.go` | 部分（新增路由文件） | Register 接入独立 registerPlaybackRoutes；playback.go 新建；`internal/router/playback_test.go`。 文档：播放模块说明及 OpenAPI。 |
| `apps/web/src/App.jsx` | `web/src/App.jsx` | 否 | FeedPage、VideoStage 增量适配；全部新增函数/常量保留；`web/tests/playback.test.mjs`。 文档：播放模块说明及 OpenAPI。 |

## 逐项符号清单

- `apps/api/internal/application/playback/service.go` → `internal/application/playback/service.go`：`ErrLoadPlaybackFailed`, `ErrSaveQoSReportFailed`, `Service`, `ConfigResult`, `PreloadResult`, `QoSReportResult`, `New`, `Service.GetConfig`, `Service.ListPreloadVideos`, `Service.CreateQoSReport`, `normalizeConfig`, `normalizeLimit`。逐项一致（仅白名单架构适配）。
- `apps/api/internal/domain/playback/entity.go` → `internal/domain/playback/playback.go`：`Config`, `PreloadVideo`, `QoSReport`, `RestoreConfig`, `DefaultConfig`, `RestorePreloadVideo`, `NewQoSReport`, `RestoreQoSReport`, `NormalizePlatform`, `NormalizeNetworkType`, `PlatformWeb`, `NetworkDefault`, `DefaultPreloadCount`, `DefaultBufferMs`, `MaxPreloadLimit`, `MaxPlatformLength`, `MaxNetworkTypeLength`, `MaxMediaURLLength`, `MaxCoverURLLength`, `MaxIdempotencyKeyLength`。逐项一致（仅白名单架构适配）。
- `apps/api/internal/domain/playback/errors.go` → `internal/domain/playback/error.go`：`ErrInvalidUserID`, `ErrInvalidVideoID`, `ErrInvalidPlatform`, `ErrInvalidNetworkType`, `ErrInvalidLimit`, `ErrInvalidFirstFrameMs`, `ErrInvalidStutterCount`, `ErrInvalidWatchMs`, `ErrIdempotencyKeyTooLong`。逐项一致（仅白名单架构适配）。
- `apps/api/internal/domain/playback/repository.go` → `internal/domain/playback/repository.go`：`Repository`, `Repository.FindConfig`, `Repository.ListPreloadVideos`, `Repository.CreateQoSReport`。逐项一致（仅白名单架构适配）。
- `apps/api/internal/infra/persistence/playback/gorm.go` → `internal/infrastructure/persistence/playback/repository.go`：`Repository`, `New`, `Repository.FindConfig`, `Repository.ListPreloadVideos`, `Repository.CreateQoSReport`, `Repository.findCurrentVideo`, `Repository.findExistingQoS`, `currentVideoModel`, `restoreConfig`, `restoreQoSReport`, `optionalString`, `stringValue`, `isDuplicateKeyError`。逐项一致（仅白名单架构适配）。
- `apps/api/internal/infra/persistence/playback/model.go` → `internal/infrastructure/persistence/playback/model.go`：`ConfigModel`, `ConfigModel.TableName`, `QoSLogModel`, `QoSLogModel.TableName`, `PreloadVideoModel`。逐项一致（仅白名单架构适配）。
- `apps/api/internal/interfaces/http/playback/dto.go` → `internal/handler/playback/dto.go`：`playbackConfigResponse`, `preloadVideoResponse`, `preloadVideosResponse`, `createQoSReportRequest`, `qosReportResponse`。逐项一致（仅白名单架构适配）。
- `apps/api/internal/interfaces/http/playback/handler.go` → `internal/handler/playback/handler.go`：`Handler`, `New`, `Handler.GetConfig`, `Handler.ListPreloadVideos`, `Handler.CreateQoSReport`, `Handler.CreateInternalQoSReport`, `Handler.createQoSReport`, `Handler.createQoSReportWithRequest`, `userIDFromContext`, `parseOptionalInt64`, `parseLimit`, `configResponseFromResult`, `preloadResponseFromResult`, `qosResponseFromResult`, `writePlaybackError`, `isBadRequestError`。逐项一致（仅白名单架构适配）。

后端 8 个独立模块文件共 84 项声明/接口方法已核对：去除导入、包名、注释和排版后进行完整正文比较，仅允许上述视频表名、索引名、认证包别名和 SQLSTATE 适配。所有函数体、结构字段、错误字符串及常量值均一致。

前端 12 个命名函数（含三个播放器事件处理函数）正文逐项比较一致；另对默认常量、配置副作用、预加载副作用、上报回调、播放器生命周期完整代码块作规范化比较；三处 VideoStage 属性、媒体事件绑定与 active 值均已核对。路由的服务组装与四条路径、迁移的两个模型逐项人工复核。

## 架构与语义适配

- 导入统一使用 `github.com/inumoroha/video-feed`，新增包名统一为 `playback`；旧 `infra` 映射至 `infrastructure`，旧 HTTP 层映射至 `handler`，领域实体和错误按当前命名为 `playback.go`、`error.go`。
- 原 `video AS v` 改为真实 `videos AS v`，保留发布状态、发布时间及 ID 双字段降序和严格游标边界；未发布或不存在的当前视频回退到最新公开视频；负数请求仍返回参数错误。
- 保留 `playback_config`、`playback_qos_log` 全部字段、可空首帧和幂等键、索引顺序。原 `uk_user_idempotency` 与目标已有索引重名，改为 `uk_playback_user_idempotency`，防止 PostgreSQL 同一 schema 下索引名冲突导致唯一约束缺失。
- 保留插入事务失败回滚后按用户和幂等键查旧记录及 `FOR UPDATE`；补充识别 PostgreSQL SQLSTATE 23505，兼容目标数据库错误翻译及未翻译错误。空键存 NULL，重复报告返回首次内容，不覆盖指标。
- 配置依次查询指定网络、同平台 DEFAULT，最后使用领域默认值；数据库故障返回错误，不伪装成配置缺失。保留全部长度和非负校验、预加载上限 10、默认 3、缓冲默认 1200。
- 四条路由按目标已有 JWT 和内部令牌注册方式接入，用户报告忽略请求体 user_id；内部报告允许 user_id=0。新建返回 201，重放返回 200。
- 新表接入已有事务与咨询锁迁移，不替换现有迁移流程。
- 原提交不涉及 Redis 或 RabbitMQ 操作、队列、重试或配置；沿用当前缓存 WATCH/事务和消息确认、拒绝、失败回退及关闭逻辑。播放质量直接写 PostgreSQL，不经过缓存和消息队列。
- 原提交没有配置文件、环境变量、依赖、Compose、数据卷、健康检查、依赖顺序或测试变更；目标已有 GORM、pgx、Gin、React 足够承载，沿用现有六服务部署、9000/5173 端口和 INTERNAL_TOKEN。
- 前端保留 DEFAULT_PLAYBACK_CONFIG、FeedPage 的配置获取/失败默认/认证失效、去重预加载与质量上报、VideoStage 的 active 控制/首帧/等待/暂停与卸载上报，以及 fetchPlaybackConfig、fetchPreloadVideos、normalizePlaybackConfig、detectNetworkType、prewarmVideoAssets、prewarmVideoMetadata、createVideoQoSState、buildPlaybackQoSPayload、createPlaybackQoSKey 全部函数。保留现有分页代际保护和关注交互。
- 原提交没有自动重试发送 QoS、页面关闭可靠投递、实际缓冲阈值控制或监控聚合；不将这些宣称为已实现。

## 验证记录

- `gofmt`：全部新增/修改 Go 文件已格式化。
- `go test ./...`：通过；需要外部连接的真实服务测试按仓库约定跳过。
- `go vet ./...`：通过。
- `npm test`（web 目录）：18 项通过。原推荐分页测试增加播放接口专用响应，原有分页、重试、认证断言不变。
- `npm run build`（web 目录）：通过。
- `git diff --check`：通过。
- `docker compose config --quiet`：通过。
- OpenAPI：YAML 可解析，42 个 operationId 无重复；四条播放路径、认证、状态码核对通过；20 个请求/响应示例通过 JSON Schema 类型、必填及格式验证。
- 中文注释：全部新增 Go 函数、方法、具名结构体及新增/修改的前端命名函数已核对。
- 真实 PostgreSQL 并发测试：已实现 24 个并发请求去重、跨用户隔离、空键不去重、首次内容不覆盖和已有同名索引隔离测试；本机未配置 VIDEOFEED_TEST_POSTGRES_URL，测试明确 SKIP。Docker 引擎管道不存在，无法启动容器验证。
- 未执行真实浏览器媒体网络与性能测量；资源释放和播放器生命周期使用 React 测试渲染器与模拟媒体对象验证。

## 测试对应位置

原提交没有测试文件，以下为迁移补充，未删除原测试覆盖：

| 代码范围 | 测试路径 | 核验内容 |
| --- | --- | --- |
| 领域对象、常量、错误 | `internal/domain/playback/playback_test.go` | 全部校验错误、网络归一化、默认与恢复字段 |
| 应用服务 | `internal/application/playback/service_test.go` | 网络回退、缺省和上限、复制隔离、错误映射与调用方重试 |
| 数据库仓储和模型 | `internal/infrastructure/persistence/playback/repository_test.go` | SQL 方言/表名、双字段游标、回滚重放、失败后再次写入、可空键 |
| 数据库并发与唯一约束 | `internal/infrastructure/persistence/playback/live_test.go` | 独立 schema 下真实并发测试（本次跳过） |
| HTTP DTO、处理器和路由 | `internal/router/playback_test.go` | 四条实际路由、401/400/200/201/500、JWT 身份覆盖、内部匿名用户 |
| 前端函数与组件 | `web/tests/playback.test.mjs` | 网络请求、配置回退、预加载去重/释放、质量指标和幂等键、活动播放与暂停上报 |
| 既有 Feed 回归 | `web/tests/recommendation.test.mjs` | 保留分页串行、失败不循环重试、曝光去重和旧响应隔离 |

## 最终文件复核

重新执行 `git show --name-only` 确认参考提交仍为以下 11 个文件，均已有对应实现，无遗漏文件；没有参考测试、配置或部署变更待迁移：

- [x] `apps/api/internal/application/playback/service.go`
- [x] `apps/api/internal/domain/playback/entity.go`
- [x] `apps/api/internal/domain/playback/errors.go`
- [x] `apps/api/internal/domain/playback/repository.go`
- [x] `apps/api/internal/infra/persistence/migration/migration.go`
- [x] `apps/api/internal/infra/persistence/playback/gorm.go`
- [x] `apps/api/internal/infra/persistence/playback/model.go`
- [x] `apps/api/internal/interfaces/http/playback/dto.go`
- [x] `apps/api/internal/interfaces/http/playback/handler.go`
- [x] `apps/api/internal/interfaces/http/router/router.go`
- [x] `apps/web/src/App.jsx`

迁移范围内代码、常量、类型、接口、前端功能与文档已逐项核对；真实 PostgreSQL 并发及浏览器媒体端到端验证仍有上述环境限制，不将模拟测试通过等同于生产运行验证。


文档同步范围：`README.md`、`docs/modules/playback.md`、`docs/modules/README.md`、`docs/product.md`、`docs/architecture.md`、`docs/deployment.md`、`docs/migrations.md` 及本审计记录。按基础设施与依赖、业务逻辑、测试、文档四个主题分别提交；领域类型及仓储契约随持久化基础设施提交，确保后续业务提交具有完整依赖。
