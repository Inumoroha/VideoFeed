# 曝光提交迁移审计

来源：`C:/Users/HP/Desktop/GCFeed`，提交 `fa476283d6f6e2d6ec429c412ce12515fdffda40`。目标：当前 VideoFeed 仓库；开始时 main 分支工作区干净。

## 逐文件迁移表

| 原文件 | 对应文件 | 新建 | 适配与核对范围 |
| --- | --- | --- | --- |
| `README.md` | `README.md` | 否 | 保留变更内容；对应测试与文档见下文 |
| `apps/api/configs/config.docker.yaml` | `configs/config.docker.yaml` | 否 | mapstructure、环境变量绑定及 videofeed 曝光拓扑名称；配置测试 internal/config/exposure_test.go |
| `apps/api/configs/config.yaml` | `configs/config.yaml` | 否 | mapstructure、环境变量绑定及 videofeed 曝光拓扑名称；配置测试 internal/config/exposure_test.go |
| `apps/api/internal/application/exposure/event.go` | `internal/application/exposure/event.go` | 是 | 逐项保留类型、字段、函数及常量；包路径映射到当前模块 |
| `apps/api/internal/application/exposure/service.go` | `internal/application/exposure/service.go` | 是 | 逐项保留类型、字段、函数及常量；包路径映射到当前模块 |
| `apps/api/internal/domain/exposure/entity.go` | `internal/domain/exposure/exposure.go` | 是 | 逐项保留类型、字段、函数及常量；包路径映射到当前模块 |
| `apps/api/internal/domain/exposure/errors.go` | `internal/domain/exposure/error.go` | 是 | 逐项保留类型、字段、函数及常量；包路径映射到当前模块 |
| `apps/api/internal/domain/exposure/repository.go` | `internal/domain/exposure/repository.go` | 是 | 逐项保留类型、字段、函数及常量；包路径映射到当前模块 |
| `apps/api/internal/infra/config/entity.go` | `internal/config/config.go` | 否 | mapstructure、环境变量绑定及 videofeed 曝光拓扑名称；配置测试 internal/config/exposure_test.go |
| `apps/api/internal/infra/mq/rabbitmq.go` | `internal/infrastructure/mq/rabbitmq.go` | 否 | 新增发布/消费、三项默认常量、拓扑及配置归一化；使用现有连接及关闭逻辑；消息测试同目录 |
| `apps/api/internal/infra/persistence/exposure/gorm.go` | `internal/infrastructure/persistence/exposure/repository.go` | 是 | MySQL VALUES 改为 PostgreSQL EXCLUDED；保留事务、已发布检查、原子递增与回滚；仓储测试同目录 |
| `apps/api/internal/infra/persistence/exposure/model.go` | `internal/infrastructure/persistence/exposure/model.go` | 是 | 表名字段保持；索引加表名前缀以适配 PostgreSQL 全局命名空间；唯一用户视频约束保持 |
| `apps/api/internal/interfaces/http/exposure/dto.go` | `internal/handler/exposure/dto.go` | 是 | 逐项保留类型、字段、函数及常量；包路径映射到当前模块 |
| `apps/api/internal/interfaces/http/exposure/handler.go` | `internal/handler/exposure/handler.go` | 是 | 逐项保留类型、字段、函数及常量；包路径映射到当前模块 |
| `apps/api/internal/interfaces/http/router/router.go` | `internal/router/register.go` | 否 | Register 组装服务和表迁移；registerRoutes 可选处理器注册认证接口；路由测试同目录 |
| `apps/api/test/exposure_api_test.go` | `internal/handler/exposure/api_flow_test.go` | 是 | 保留三个测试和辅助类型/函数；JWT 改用 JWTConfig、SignAccessToken 与当前认证中间件 |
| `docs/modules/feed.md` | `docs/modules/feed.md` | 否 | 保留变更内容；对应测试与文档见下文 |

## 适配边界

- 新接口 POST /api/video-view-events 使用现有 JWT 身份，返回 201/400/401/404/500；其余接口保持。
- 流水不按 request_id 去重；重复 exposed 上报追加流水并累加曝光次数。只有 exposed 更新聚合，complete 强制 completed=true。
- 数据库事务提交后发布事件；发布失败不回滚、不自动重试。原提交无发件箱或发布确认；不声称可靠必达。
- 消费成功 Ack；无效 JSON Nack 不重入队；处理失败 Nack 重入队；上下文取消结束消费。原提交未启动画像 worker，本次仅迁移消费接口。
- 无新增 Redis 操作，现有 WATCH 和事务管道保持。沿用 PostgreSQL、Redis、RabbitMQ 的卷、健康检查、依赖顺序；无新依赖。
- 文档对应 README.md、docs/modules/feed.md，并补充 api/openapi.yaml；开发记录即本文件。

## 核对及验证

结果见下方逐项审计和验证记录。

## 逐项符号审计

重新执行 `git show --name-only` 得到同样的 17 个文件，全部在上表有落点。来源 main 与 origin/main 包含该提交；目标仓库对象库不含该提交，未 cherry-pick；验证完成后按基础设施、业务逻辑、测试、文档分组提交。读取了 --stat、--name-only、--format=fuller 及每个文件完整 diff。

| 原文件 | 已核对的声明与映射 |
| --- | --- |
| `apps/api/internal/application/exposure/event.go` | `ViewEventRecordedEvent`, `NewViewEventRecordedEvent`, `newEventID` |
| `apps/api/internal/application/exposure/service.go` | `ErrSaveExposureFailed`, `Service`, `RecordViewEventResult`, `ViewEventPublisher`, `Option`, `New` → `NewService`, `WithViewEventPublisher`, `RecordViewEvent`, `publishViewEventRecorded` |
| `apps/api/internal/domain/exposure/entity.go` | `ViewEvent`, `Exposure`, `NewViewEvent`, `RestoreViewEvent`, `RestoreExposure`, `CountsAsExposure`, `isSupportedEventType`, `EventTypeExposed`, `EventTypePlay`, `EventTypeComplete`, `EventTypeSkip`, `MaxSceneLength`, `MaxRequestIDLength` |
| `apps/api/internal/domain/exposure/errors.go` | `ErrInvalidUserID`, `ErrInvalidVideoID`, `ErrEmptyScene`, `ErrSceneTooLong`, `ErrInvalidEventType`, `ErrRequestIDTooLong`, `ErrWatchMsNegative`, `ErrVideoNotFound` |
| `apps/api/internal/domain/exposure/repository.go` | `Repository` |
| `apps/api/internal/infra/persistence/exposure/gorm.go` | `Repository`, `New`, `SaveViewEvent`, `ensurePublishedVideo`, `restoreViewEvent`, `restoreExposure`, `stringPtr`, `stringValue`, `mapExposureError`, `isDuplicateKeyError` |
| `apps/api/internal/infra/persistence/exposure/model.go` | `ViewEventModel`, `TableName`, `ExposureModel`, `TableName` |
| `apps/api/internal/interfaces/http/exposure/dto.go` | `createViewEventRequest`, `viewEventResponse`, `exposureResponse`, `createViewEventResponse` |
| `apps/api/internal/interfaces/http/exposure/handler.go` | `Handler`, `New` → `NewHandler`, `CreateViewEvent`, `responseFromResult`, `userIDFromContext`, `writeExposureError`, `isBadRequestError` |
| `apps/api/test/exposure_api_test.go` | `exposureAPIResponse`, `exposureEventAPIResponse`, `exposureIndexAPIResponse`, `memoryExposureRepo`, `memoryViewEventPublisher`, `newMemoryExposureRepo`, `SaveViewEvent`, `EventCount`, `PublishViewEventRecorded`, `Events`, `TestExposureAPIFlow`, `TestExposurePublishesViewEvent`, `TestExposureAPIValidation`, `newExposureRouter`, `newExposureRouterWithPublisher`, `cloneExposureViewEvent`, `cloneExposure`, `memoryExposureKey` |

以上共核对 77 项声明（含同名方法、测试辅助声明和领域常量）。接口方法 `Repository.SaveViewEvent`、`ViewEventPublisher.PublishViewEventRecorded` 的输入输出保持；HTTP JSON、消息 JSON 及实体/结果结构字段逐项保留。

### 修改文件中的新增与扩展声明

- `RabbitMQConfig` 增加 ExposureExchange、ViewEventRecordedQueue、ViewEventRecordedRouting，三项 yaml 标签改用当前 Viper mapstructure 标签；另在 env.go 显式绑定，旧配置无新增键时仍支持环境覆盖。
- `RabbitMQ.PublishViewEventRecorded`、`ConsumeViewEventRecorded` 完整保留；消费逐消息分支提取到 handleViewEventDelivery 便于独立断言 Ack/Nack 参数。
- defaultExposureExchange、defaultViewEventRecordedQueue 的前缀按当前视频拓扑映射为 videofeed；defaultViewEventRecordedRouting 保持 exposure.view_event_recorded。
- ensureTopology 保留既有拓扑，再声明曝光持久化 topic、持久化队列及绑定；normalizeRabbitMQConfig 映射 normalizeConfig，并保留新增字段去空白与默认值。
- 原 Register 中新增两表迁移、仓储/服务/处理器和可选发布器均落在当前 Register；路由由 registerRoutes 的可选曝光处理器注册，原有接口路径保持。认证 NewJWTAuth 映射 NewJWTAuthMiddleware，JWT 构造采用 JWTConfig，测试签名采用 SignAccessToken。

### 语义与数据核对

- 数据库：新仓储查询当前 videos 表与 StatusPublished=2；保留流水和聚合同一事务、错误回滚、首次时间、最近时间/场景、计数和读取更新后聚合。MySQL VALUES 改 EXCLUDED；递增限定 exposures.exposure_count 避免 PostgreSQL 冲突列歧义。
- 字段/索引：两张表全部字段、空 request_id→NULL、主键、时间和默认值保留；所有新增索引添加表前缀，用户视频复合唯一键保留。原 idx_user_scene_created 实际只含 scene/created_at，本次不擅自新增 user_id。
- 错误：所有八个领域错误与 ErrSaveExposureFailed 保留；mapExposureError 原样透传；isDuplicateKeyError 从 MySQL 1062 改为 PostgreSQL 23505/GORM ErrDuplicatedKey（原映射函数对重复错误同样透传）。
- 消息：字段、omitempty、UTC 时间、随机 ID 和失败时纳秒回退、持久化属性、MessageId、Timestamp 保留。连接/通道初始化失败释放资源及 Close 沿用现有实现；没有新增连接、后台业务 worker 或发布保证。
- 重试/幂等：原提交无请求幂等、数据库重试或发布重试；不可把 request_id 变为唯一键。消费处理失败 Nack(false,true)，格式错误 Nack(false,false)，成功 Ack(false)；确认错误沿原实现忽略。
- Redis：参考提交没有 Redis 变更，本次未修改任何 Redis/cache 文件，现有 WATCH 原子性和事务管道未变。
- 部署：参考提交没有 Compose/卷/健康检查/依赖变更；检查当前根目录 Compose 后复用已有服务。没有新增 Go 依赖或 MySQL 驱动。容器配置的 RabbitMQ 账号需在服务中存在，当前 Compose 未声明该自定义账号初始化；这是既有部署前提，并非本次验证已覆盖的内容。

## 测试落点和覆盖

| 来源/补充范围 | 当前测试 |
| --- | --- |
| 原 TestExposureAPIFlow | internal/handler/exposure/api_flow_test.go；首次/重复曝光、首曝时间不变、完播强制 true、流水数量 |
| 原 TestExposurePublishesViewEvent | 同上；画像字段、保存失败不发布 |
| 原 TestExposureAPIValidation | 同上；认证、场景、类型、负观看时长、视频不存在 |
| 参数及事件字段边界 | internal/domain/exposure/exposure_test.go、internal/application/exposure/event_test.go |
| 消息失败回退、相同 request_id 并发计数、HTTP 额外校验 | internal/handler/exposure/regression_test.go；并发为内存仓储测试，不冒充数据库并发验证 |
| PostgreSQL SQL、各阶段回滚、唯一约束和错误识别 | internal/infrastructure/persistence/exposure/repository_test.go |
| 真实 PostgreSQL 并发、索引共存、约束回滚 | internal/infrastructure/persistence/exposure/live_test.go；未配置连接时跳过 |
| 真实注册路由、身份不可覆盖、500 不泄漏存储错误 | internal/router/exposure_test.go |
| 消息默认/定制配置、重入队/确认/拒绝参数 | internal/infrastructure/mq/exposure_test.go |
| 真实画像消息属性、路由、重投与毒消息 | internal/infrastructure/mq/exposure_live_test.go；未配置连接时跳过 |
| 原视频真实消息测试 | internal/infrastructure/mq/video_live_test.go；新增隔离曝光拓扑和清理，避免初始化产生业务默认队列 |
| 环境变量覆盖旧配置 | internal/config/exposure_test.go |

## 验证结果与限制

- gofmt：已格式化本次新增/修改 Go 文件。
- go test ./...：通过，包括原仓库全部三个曝光测试和新增回归测试。
- go vet ./...：通过。
- git diff --check：通过；额外检查新增文件尾空白。
- docker compose config --quiet：通过，仅说明 Compose 配置有效，不代表服务启动成功。
- OpenAPI、两个应用 YAML 与 Compose YAML：PyYAML 解析通过；operationId 唯一；新接口请求、201 两种响应以及 400/401/404/500 错误示例通过 JSON Schema 校验。路由与状态码由实际 handler/registerRoutes 核对。
- go test -race（曝光相关包、消息、路由）：2026-09-19 已通过 Go 1.25.14 / GCC 14.2.0 Linux 容器完成，六个包全部通过；宿主 Windows 缺少 gcc 的问题通过隔离验证环境绕开。真实服务测试另在原生 Go 环境执行通过。
- 真实 PostgreSQL / RabbitMQ 测试：2026-09-19 使用独立 Docker 服务执行通过，无 SKIP；实际 API 的并发上报、错误响应、事务回滚、消息故障回退、持久化恢复与清理也全部通过。详见 [真实服务验证](exposure-live-verification.md) 和 [机器可读结果](exposure-live-results.json)。
- 代码、测试、配置和文档的来源项均已有对应落点，未发现遗漏；本次约定的真实服务验证场景已全部通过。竞态检测单独记录。验证完成后按主题拆分 Git 提交。

补充逐函数比对：映射构造函数和 import 别名、去除注释/排版后，38 个来源函数完全一致。仅 SaveViewEvent（PostgreSQL SQL）、isDuplicateKeyError（PostgreSQL 错误码）、newExposureRouterWithPublisher（当前 JWT API）存在已解释的函数体差异。RabbitMQ 与路由的增量另按上文逐项核对。
