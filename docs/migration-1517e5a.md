# 关注流迁移审计

来源：`C:/Users/HP/Desktop/GCFeed`，提交 `1517e5a7b21fd5063632433efbd00dd92a7ddf69`（feat(feed): 实现关注流推拉混合模式）。
目标：`C:/Users/HP/Desktop/VideoFeed`，起始分支 `main`，工作区干净。迁移完成后按用户要求拆分为基础设施、业务逻辑、测试和文档四个主题提交。
已读取 git show --stat、--name-only、--format=fuller 及全部十个文件的完整 diff。

## 逐文件迁移表

原路径均相对于 `apps/api/`；目标路径相对于本项目根目录。

| 原文件 | 目标文件 | 新建 | 符号和行为对应 | 数据库、配置、依赖适配 | 测试与文档 |
| --- | --- | --- | --- | --- | --- |
| internal/application/feed/service.go | internal/application/feed/service.go、internal/application/feed/dto.go | 否 | FollowingStrategy、NewFollowingStrategy、Scene、List、listPageFromRepo；WithFeedCache 注入；New → NewService 注册 | 复用现有卡片/计数缓存，关注页不共享缓存 | internal/application/feed/following_test.go、docs/modules/feed.md |
| internal/domain/feed/entity.go | internal/domain/feed/feed.go | 否 | BigCreatorFollowerThreshold = 10000 | 无新配置 | 阈值边界测试 |
| internal/domain/feed/errors.go | internal/domain/feed/error.go | 否 | ErrViewerRequired | 无 | 401 测试 |
| internal/domain/feed/repository.go | internal/domain/feed/repository.go | 否 | Repository.ListFollowingPage | 扩充已有仓储及测试替身 | 分层仓储测试 |
| internal/infra/persistence/feed/gorm.go | internal/infrastructure/persistence/feed/repository.go | 否 | ListFollowingPage：UNION 去重、有效关注、发布状态、稳定游标 | video → videos；关系表名保持现状；使用 PostgreSQL 参数绑定 | internal/infrastructure/persistence/feed/following_test.go |
| internal/infra/persistence/feed/model.go | internal/infrastructure/persistence/feed/model.go | 是 | InboxModel 全部字段、唯一约束、索引、TableName | GORM AutoMigrate 创建 feed_inbox | 模型及数据库测试 |
| internal/infra/persistence/video/gorm.go | internal/infrastructure/persistence/video/repository.go | 否 | Save、fanoutPublishedVideoToFollowers、authorFollowerCount | 同一事务写视频、计数和收件箱；ON DUPLICATE KEY → ON CONFLICT (user_id, video_id) | internal/infrastructure/persistence/video/following_test.go |
| internal/interfaces/http/feed/handler.go | internal/handler/feed/handler.go | 否 | writeFeedError：ErrViewerRequired → 401 | 沿用统一 error 响应 | internal/handler/feed/following_test.go、api/openapi.yaml |
| internal/interfaces/http/router/router.go | internal/router/register.go | 否 | Register 注册 InboxModel 迁移 | 使用已注入的 GORM；POST 保留 /api/feed/query | internal/router/following_test.go、api/openapi.yaml |
| test/feed_api_test.go | internal/handler/feed/following_test.go | 是 | TestFeedSceneQuery 的匿名关注流断言、TestFollowingFeedScene；内存关注关系/粉丝数/inbox、FollowingCalls 等辅助能力；JWT 测试装配和种子数据 | JWT 使用 config.JWTConfig、SignAccessToken；New → NewHandler；旧 /api/feed-queries → /api/feed/query | 保留推拉合并、阈值、过滤、两页结果和两次仓储调用断言 |

## 架构与边界

- 当前按 handler/application/domain/infrastructure 分层，Go 模块为 github.com/inumoroha/video-feed。
- 数据库为 PostgreSQL，视频表 videos、作者表 users、关系表 user_follow/user_relation_stat。普通作者的发布扇出与视频保存共用事务，失败整体回滚；唯一约束和原子冲突更新保持重复投递语义。
- 关注流按发布时间、视频 ID 双降序；普通作者只读发布时已推送的收件箱，大作者通过当前粉丝数和当前关注关系拉取。取关后旧收件箱条目不返回；推拉重叠由 UNION 去重。
- 保留原提交边界：不回填关注前的普通作者视频，不补齐作者跨阈值后的历史 inbox；非空最后一页仍返回游标，以 has_more 判断是否继续。
- 原提交没有 Redis 命令、WATCH/事务/Lua、RabbitMQ 连接/拓扑/确认/拒绝/重试/关闭、配置、环境变量、依赖或 Compose 文件变更。现有相关实现保持不变；关注流使用现有卡片和计数缓存的失败回源行为。
- 当前 Compose 已包含 PostgreSQL、Redis、RabbitMQ 的数据卷和健康检查以及 API 的健康依赖；API 9000、Web 5173、宿主 PostgreSQL 5433。无需新增服务、卷、依赖顺序或配置项。

## 验证与最终审计

审计日期：2026-09-18。重新从固定提交列出全部变更文件，确认上表十项均已落实；未发现本提交范围内尚未迁移的代码、测试、配置或文档项。

### 符号与行为核对

| 原提交符号 | 核对结果与证据 |
| --- | --- |
| FollowingStrategy（repo、cache） | 已移入 internal/application/feed/dto.go；仅共享卡片和统计缓存，不共享用户分页结果 |
| WithFeedCache、New | 已为关注流注入缓存；NewService 注册默认关注流策略，兼容入口 New 调用 NewService |
| NewFollowingStrategy、FollowingStrategy.Scene | 构造仓储策略并返回既有 SceneFollowing |
| FollowingStrategy.List | 保留登录校验、游标解析、分页规范化、仓储读取、卡片统计组装及 ErrLoadFeedFailed 映射；增加符合本项目惯例的空仓储和负 limit 防御校验 |
| FollowingStrategy.listPageFromRepo | 保留 limit+1 探测、截断、最后条目的时间与 ID 游标，以及非空尾页仍返回游标的语义 |
| BigCreatorFollowerThreshold、ErrViewerRequired | 值分别为 10000、viewer is required；没有新增可配置阈值 |
| Repository.ListFollowingPage、持久化实现 | 保留用户参数、时间游标、有效关注过滤、已发布/非空时间过滤、阈值比较、UNION 去重及双字段倒序；SQL 只将 video 适配为 videos |
| InboxModel、TableName | 保留 ID/UserID/VideoID/AuthorID/PublishedAt/CreatedAt；主键自增、必填约束、uk_user_video、idx_user_published_video、idx_video、idx_author 及索引顺序全部保留；表名 feed_inbox |
| 视频 Repository.Save | 视频、初始统计、收件箱共用调用方事务；错误回滚；仅事务成功后将 ID 和时间写回领域对象；原有幂等键错误映射保留 |
| fanoutPublishedVideoToFollowers | 空发布时间跳过；粉丝数达到阈值跳过；有效粉丝 INSERT SELECT；冲突时只更新 published_at，原子性由 PostgreSQL 唯一约束与 ON CONFLICT 保证 |
| authorFollowerCount | 缺失统计返回零，其他错误传播；原匿名结构改为带中文注释的 authorFollowerStat，字段 FollowerCount 保留 |
| writeFeedError | ErrViewerRequired 返回 401 和统一 error 字段；原有 400、500 映射保留 |
| Register | 在依赖装配前执行 InboxModel 自动迁移，错误返回并终止启动；使用本项目 GORM 实例，不引入旧仓库 sql.DB 装配 |
| memoryFeedRepo、newMemoryFeedRepo | 原新增 following/followerCounts/inbox/followingCalls 字段全部保留并初始化；互斥锁保护保留 |
| FollowingCalls、FollowForTest、SetFollowerCountForTest、PushToInboxForTest、测试 ListFollowingPage | 完整保留原实现的锁、计数、推拉合并、当前关注检查、去重、排序和分页 |
| TestFeedSceneQuery、TestFollowingFeedScene | 保留原修改的匿名 401 断言；原新增混合流用例断言第一页 [4,2]、第二页 [1]、has_more、scene、游标、两次仓储调用全部保留 |
| newFeedRouterWithServiceAndJWT、seedFollowingFeedItems | 四条原种子数据保留；JWT 构造适配配置结构和密钥长度要求；路由改用本项目路径和可选认证中间件 |

测试辅助函数 feedCardFromFeedItem 去除原仓库 FeedCard.PublishedAt 赋值：本项目发布时间来自 FeedPageItem，领域卡片没有该字段，HTTP published_at 仍由页项组装且分页测试通过。

### 测试与检查结果

共新增 14 个测试函数（含移植的 2 个源测试），位于五个 following_test.go 文件；既有 service_test.go 和 handler_test.go 的测试仓储增加 ListFollowingPage 接口实现。

| 检查 | 实际结果 |
| --- | --- |
| gofmt -w，随后 gofmt -l 检查所有变更 Go 文件 | 通过，无待格式化文件 |
| go test ./... | 通过，所有包成功 |
| go vet ./... | 通过 |
| git diff --check | 通过 |
| docker compose config --quiet | 通过，退出码 0；该检查不要求 Docker 引擎运行 |
| OpenAPI YAML 解析 | 通过，同时拒绝重复键并解析全部本地引用 |
| OpenAPI 路由、operationId、示例 | 22 个 operationId 全局唯一；所有已记录 HTTP 路径和方法均存在于路由；Feed 两个入口的 200/400/401/500 及 9 个请求/响应示例通过 schema 检查 |
| 中文注释审计 | 比对 HEAD 声明内容，71 个新增/修改函数及结构体声明均有紧邻、以名称开头的中文注释 |
| 原源测试行为 | 普通作者 inbox、大作者阈值拉取、未关注作者过滤、两页排序和调用次数通过 |
| 新增边界与失败测试 | 未认证/无效令牌、无效参数、空页、同时间排序、去重、取关、未推送过滤、分页上限、取消、缓存读写失败回源、仓储各阶段错误与请求重试、32 个并发用户隔离均通过 |
| PostgreSQL SQL mock | 查询参数、真实表名、UNION、状态过滤、游标、唯一索引 SQL、原子冲突更新 SQL、阈值上下界、缺失统计、发布事务回滚和重试均通过 |
| go test -race（Feed 应用/处理器及 Feed/视频仓储） | 无法运行：runtime/cgo 找不到 gcc；不是竞态检测通过 |

### 验证限制及兼容边界

- Docker 引擎未运行，未启动容器；没有真实 PostgreSQL 上的 AutoMigrate、并发冲突、回滚或端到端联调结果。SQL mock 验证了 SQL 形状、参数和事务调用顺序，不代表真实数据库执行验证。
- 已执行普通并发用户隔离测试，但由于缺少 C 编译器，无法提供 race 检测结论。
- following 从此前未支持（400）变为已支持；匿名或无效令牌返回 401，有效用户返回 200。这是本次功能变化；timeline/hot 的既有行为不变。
- 原提交的历史补发与跨阈值回填缺失属于保留的功能边界，未擅自扩展为新能力。
- Redis、RabbitMQ、配置、环境变量、依赖及 Compose 不存在本提交新增变更；现有文件未修改，全仓相关回归测试通过。
- 按用户后续要求在 main 分支分主题提交：基础设施与依赖（本次无依赖变更）、业务逻辑、测试、文档；每次提交后检查工作区状态。

### 结束前重新列出的原提交文件


- `apps/api/internal/application/feed/service.go`：已迁移，目标见上表。
- `apps/api/internal/domain/feed/entity.go`：已迁移，目标见上表。
- `apps/api/internal/domain/feed/errors.go`：已迁移，目标见上表。
- `apps/api/internal/domain/feed/repository.go`：已迁移，目标见上表。
- `apps/api/internal/infra/persistence/feed/gorm.go`：已迁移，目标见上表。
- `apps/api/internal/infra/persistence/feed/model.go`：已迁移，目标见上表。
- `apps/api/internal/infra/persistence/video/gorm.go`：已迁移，目标见上表。
- `apps/api/internal/interfaces/http/feed/handler.go`：已迁移，目标见上表。
- `apps/api/internal/interfaces/http/router/router.go`：已迁移，目标见上表。
- `apps/api/test/feed_api_test.go`：已迁移，目标见上表。

### 本项目实际变更文件

- `README.md`
- `api/openapi.yaml`
- `docs/migration-1517e5a.md`
- `docs/modules/feed.md`
- `internal/application/feed/dto.go`
- `internal/application/feed/following_test.go`
- `internal/application/feed/service.go`
- `internal/application/feed/service_test.go`
- `internal/domain/feed/error.go`
- `internal/domain/feed/feed.go`
- `internal/domain/feed/repository.go`
- `internal/handler/feed/following_test.go`
- `internal/handler/feed/handler.go`
- `internal/handler/feed/handler_test.go`
- `internal/infrastructure/persistence/feed/following_test.go`
- `internal/infrastructure/persistence/feed/model.go`
- `internal/infrastructure/persistence/feed/repository.go`
- `internal/infrastructure/persistence/video/following_test.go`
- `internal/infrastructure/persistence/video/repository.go`
- `internal/router/following_test.go`
- `internal/router/register.go`

## 真实联调复核修正

后续真实 PostgreSQL 联调发现：卡片回源曾关联不存在的 account 表，导致有数据的关注流返回 500。此前静态审计误记了用户表名，也未捕获该问题。实际表名为 users；现已修复 BatchGetFeedCards 并添加先失败后通过的回归测试。本文件上方的表名已更正。原“未验证真实数据库”的记录反映迁移当时状态，后续真实数据库、账号和浏览器验证结果见 [联调报告](following-live-integration.md)。
