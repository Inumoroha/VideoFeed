# 互动状态与通知用户展示迁移审计

来源：`C:/Users/HP/Desktop/GCFeed`，提交 `2a216857945cfcc8c265196267c4c00bcde31d2e`，标题为 `fix(feed): 修复互动状态和通知用户展示`，作者与提交时间均为 2026-06-06 20:26:27 +0800。目标为当前 VideoFeed，迁移前 `main` 分支工作区干净，基线为提交 `b45974b`。

## 提交完整变更

已确认该提交只存在于来源仓库，并执行 `git show --stat`、`git show --name-only`、`git show --format=fuller` 以及逐文件完整 diff。

| 项 | 结果 |
| --- | --- |
| 变更文件 | 30 个，覆盖领域、应用、缓存、持久化、迁移、HTTP 接口、测试和 Web 前端 |
| 变更规模 | `753 insertions(+), 116 deletions(-)` |
| 修复目标 | Feed 条目缺少当前用户点赞收藏状态；点赞、评论、关注通知只显示用户编号 |

来源仓库使用 `apps/api` 与 `apps/web` 目录、MySQL、`GCFeed` 模块路径和 `infra`、`interfaces` 分层；VideoFeed 使用仓库根目录的 `internal`、PostgreSQL 和 `handler` 分层，因此下表按真实分层重新映射，未复制旧目录或包名。

## 逐文件迁移表

| 原文件 | 当前文件 | 新建 | 声明与适配 | 测试和文档 |
| --- | --- | --- | --- | --- |
| `apps/api/internal/domain/feed/entity.go` | `internal/domain/feed/feed.go` | 否 | `FeedItem` 增加 `Liked`、`Favorited`；新增 `ViewerActionState`，中文注释补齐 | `internal/handler/feed/following_test.go` |
| `apps/api/internal/domain/feed/repository.go` | `internal/domain/feed/repository.go` | 否 | `Repository` 增加 `BatchGetViewerActionStates` | 六个仓储替身补齐方法 |
| `apps/api/internal/domain/interaction/entity.go` | `internal/domain/interaction/interaction.go` | 否 | 新增 `UserProfile`（`ID`、`Nickname`、`AvatarURL`） | `internal/application/interaction/service_test.go` |
| `apps/api/internal/domain/interaction/repository.go` | `internal/domain/interaction/repository.go` | 否 | `Repository` 增加 `GetUserProfile` | 仓储替身补齐方法 |
| `apps/api/internal/domain/message/entity.go` | `internal/domain/message/message.go` | 否 | `Message` 增加 `ActorID`、`ActorNickname`、`ActorAvatarURL`；新增 `WithActor`、`RestoreWithActor`，`Restore` 保持不变 | `internal/handler/message/api_flow_test.go` |
| `apps/api/internal/domain/relation/entity.go` | `internal/domain/relation/relation.go` | 否 | 新增 `UserProfile` 与 `RestoreUserProfile` | `internal/application/relation/message_test.go` |
| `apps/api/internal/domain/relation/repository.go` | `internal/domain/relation/repository.go` | 否 | `Repository` 增加 `GetUserProfile` | 仓储替身补齐方法 |
| `apps/api/internal/application/feed/service.go` | `internal/application/feed/service.go` | 否 | `assembleFeedItems` 增加 `viewerID` 参数并在有效查看者时批量读取状态；四处策略调用点传入 `req.ViewerID` | `TestFeedIncludesViewerActionState`、`TestAnonymousFeedHidesViewerActionState` |
| `apps/api/internal/application/feed/service.go`（推荐策略） | `internal/application/feed/recommendation.go` | 否 | 推荐策略调用点传入 `req.ViewerID`；VideoFeed 把推荐策略拆到独立文件，故映射到该文件 | 推荐 Feed 流程测试 |
| `apps/api/internal/application/interaction/service.go` | `internal/application/interaction/service.go` | 否 | 新增 `StatCache` 与 `WithStatCache`、`ActorMessageWriter`、`syncCommentCount`、`createInteractionMessage`；点赞与评论通知改用统一入口 | `TestInteractionCommentSyncsStatCache`、`TestInteractionStatCacheWithoutProviderUsesZeroBase` |
| `apps/api/internal/application/message/service.go` | `internal/application/message/service.go` | 否 | 新增 `CreateFromActorEvent`，`CreateFromEvent` 委托给它并传空触发者 | `internal/handler/message/api_flow_test.go` |
| `apps/api/internal/application/relation/service.go` | `internal/application/relation/service.go` | 否 | 新增 `ActorMessageWriter` 接口；`notifyFollow` 读取触发者资料、结构化写入或回退正文拼接 | `TestRelationMessageWriter` |
| `apps/api/internal/infra/cache/feed_cache.go` | `internal/infrastructure/cache/feed/feed.go` | 否 | 新增 `SetVideoStat`、`videoStatToFeedStat` 与 `videoStatCacheable`；复用既有 `setActionStatJSON` 和 `feedStatKey` | `internal/infrastructure/cache/feed/feed_test.go` |
| `apps/api/internal/infra/cache/feed_cache_test.go` | `internal/infrastructure/cache/feed/feed_test.go` | 否 | 新增 JSON 计数写入读回测试，并补充非法输入边界测试 | 同文件 |
| `apps/api/internal/infra/persistence/feed/gorm.go` | `internal/infrastructure/persistence/feed/repository.go` | 否 | 新增 `viewerActionStateModel` 与 `BatchGetViewerActionStates`，查询 `interaction_action` 的有效 LIKE 和 FAVORITE | `internal/infrastructure/persistence/feed` 既有测试 |
| `apps/api/internal/infra/persistence/interaction/gorm.go` | `internal/infrastructure/persistence/interaction/repository.go` | 否 | 新增 `userProfileModel`、`GetUserProfile` 和 `mapUserError`；表名由来源 `account` 适配为当前 `users` | `internal/infrastructure/persistence/interaction` 既有测试 |
| `apps/api/internal/infra/persistence/message/gorm.go` | `internal/infrastructure/persistence/message/repository.go` | 否 | 写入触发者三字段，读取改用 `RestoreWithActor` | `internal/infrastructure/persistence/message` 既有测试 |
| `apps/api/internal/infra/persistence/message/model.go` | `internal/infrastructure/persistence/message/model.go` | 否 | `MessageModel` 增加 `actor_id`（非空默认 0）、`actor_nickname`（128）、`actor_avatar_url`（512） | 同上 |
| `apps/api/internal/infra/persistence/migration/migration.go` | `internal/infrastructure/persistence/migration/migration.go` | 否 | 迁移拆出 `migrateModels`，新增 `migrateTablesWithRetry` 与 `isConcurrentMigrationError`；并发 DDL 冲突由 MySQL 1060/1061 适配为 PostgreSQL 42701/42P07/23505 | `internal/infrastructure/persistence/migration/migration_test.go` |
| `apps/api/internal/infra/persistence/relation/gorm.go` | `internal/infrastructure/persistence/relation/repository.go` | 否 | 新增 `relationUserProfileModel` 与 `GetUserProfile`；表名适配为 `users`，未找到映射为 `ErrTargetUserNotFound` | `internal/infrastructure/persistence/relation` 既有测试 |
| `apps/api/internal/interfaces/http/feed/dto.go` | `internal/handler/feed/dto.go` | 否 | `feedItemResponse` 增加 `liked`、`favorited` JSON 字段 | `internal/handler/feed` 测试 |
| `apps/api/internal/interfaces/http/feed/handler.go` | `internal/handler/feed/handler.go` | 否 | 组装响应时透传两个布尔字段 | 同上 |
| `apps/api/internal/interfaces/http/message/dto.go` | `internal/handler/message/dto.go` | 否 | `createMessageRequest` 与 `messageResponse` 增加触发者字段，响应字段使用 `omitempty` | `TestMessageAPIFlow` |
| `apps/api/internal/interfaces/http/message/handler.go` | `internal/handler/message/handler.go` | 否 | 创建改调 `CreateFromActorEvent`；响应映射补充三个字段 | 同上 |
| `apps/api/internal/interfaces/http/router/router.go` | `internal/router/register.go`、`internal/router/message.go` | 否 | 启用 Redis 时追加 `WithStatCache`；`MessageWriter` 增加 `CreateFromActorEvent` 转发方法 | `internal/router` 既有测试 |
| `apps/api/test/feed_api_test.go` | `internal/handler/feed/following_test.go` | 否 | 响应结构增加两字段，仓储替身增加状态映射与设置辅助方法，迁移 `TestFeedIncludesViewerActionState` | 同文件；补充匿名用例 |
| `apps/api/test/interaction_api_test.go` | `internal/application/interaction/service_test.go`、`internal/application/interaction/message_test.go` | 否 | 新增计数缓存替身与两个评论计数同步测试；通知写入器增加触发者字段与断言 | 同文件 |
| `apps/api/test/message_api_test.go` | `internal/handler/message/api_flow_test.go` | 否 | 响应结构增加触发者字段，创建与重放用例断言触发者信息 | 同文件 |
| `apps/api/test/relation_api_test.go` | `internal/application/relation/message_test.go` | 否 | 通知写入器增加触发者字段与断言 | 同文件 |
| `apps/web/src/App.jsx` | `web/src/App.jsx` | 否 | `FeedPage` 重建与合并点赞收藏映射；评论首次加载回填评论数；消息列表展示触发者；新增四个辅助函数；`mapFeedItem` 透传状态 | `web/tests/*.test.mjs`、Web 构建 |
| `apps/web/src/styles.css` | `web/src/style.css` | 否 | 消息行距、触发者行与正文样式；目标样式表文件名为单数 `style.css` | 同上 |

该提交不新增或修改 HTTP 路径，因此 `api/openapi.yaml` 的路径集合不变；但 Feed 条目响应和消息响应新增了字段，已同步更新 `FeedItemResponse`、`Message`、`CreateMessageRequest` 三个 schema 及消息示例，检查规范要求的 OpenAPI 同步。

## 行为等价性核对

- **互动状态读取**：来源在 `assembleFeedItems` 内按 `viewerID > 0` 才批量读取，读取错误直接返回错误；VideoFeed 保持相同判断和错误传播，且匿名请求不发起查询。
- **状态语义**：来源查询 `status = 1` 且动作类型为 LIKE 或 FAVORITE，按动作类型置位；VideoFeed 的 `interaction_action` 表、`ActionStatusActive = 1`、`ActionTypeLike`、`ActionTypeFavorite` 常量一致，逐项保持。
- **通知触发者**：来源为互动与关注两类通知都先读取用户资料，成功则写入结构化触发者，失败或写入器不支持时回退为“昵称或用户 ID 加正文”。VideoFeed 三处保持同一分支结构与回退文案，评论通知正文改为评论内容、点赞通知正文改为“点赞了你的视频”，与来源一致。
- **通知幂等**：事件键保持 `interaction:like:<videoID>:<userID>`、`interaction:comment:<commentID>`、`relation:follow:<targetUserID>:<userID>`，幂等键继续使用同一事件键；重放返回既有消息及其触发者字段。
- **评论计数缓存同步**：来源在新增与删除评论后调用 `syncCommentCount`，缓存对象为空或视频编号非正时跳过；读取计数失败按零计数补齐，缓存写入失败忽略。VideoFeed 保持一致；差异是 VideoFeed 的 `Repository` 接口没有 `GetVideoStat`，因此改用与 `setAction` 相同的可选接口断言方式读取，行为等价。
- **迁移重试**：来源对整组模型迁移最多重试 4 次、每次退避 `(attempt+1) * 150ms`，仅在并发 DDL 冲突时重试并保留最后一次错误。VideoFeed 沿用同一重试次数、退避公式和错误保留语义；差异是错误码从 MySQL 1060（列已存在）、1061（索引已存在）适配为 PostgreSQL 42701（`duplicate_column`）、42P07（`duplicate_table`）和 23505（并发的唯一约束冲突），错误类型判断使用 `pgconn.PgError`。VideoFeed 仍在事务内持有 `pg_advisory_xact_lock`，重试属于并发防护的补充。
- **消息持久化**：来源写入三个触发者字段并用 `RestoreWithActor` 读取；VideoFeed 的 `MessageModel`、写入结构和读取函数逐项对应，`actor_id` 与来源同为非空默认 0。
- **前端状态**：来源首屏用响应重建点赞收藏映射、续页合并；VideoFeed 的 `FeedPage` 状态初值为空对象，加入相同行为，因此首屏、场景切换和续页语义一致。
- **评论数回填**：来源在评论首页加载完成且无下一页、条数大于当前计数时更新条目计数；VideoFeed 一致，并把 `updateCurrentItem` 加入依赖数组，避免闭包捕获旧值。

## 测试映射

| 来源测试 | 当前位置 | 处理 |
| --- | --- | --- |
| `TestFeedIncludesViewerActionState` | `internal/handler/feed/following_test.go` | 保留并补充视频编号断言，确认返回顺序与个人状态一一对应 |
| `TestSetVideoStatWritesJSONCache` | `internal/infrastructure/cache/feed/feed_test.go` | 保留 JSON 写入后读回断言 |
| `TestInteractionCommentSyncsStatCache` | `internal/application/interaction/service_test.go` | 保留新增与删除评论同步计数断言 |
| `TestInteractionMessageWriter` 触发者断言 | `internal/application/interaction/message_test.go` | 保留点赞与评论触发者字段断言 |
| `TestMessageAPIFlow` 触发者断言 | `internal/handler/message/api_flow_test.go` | 保留创建与幂等重放的触发者字段断言 |
| `TestRelationMessageWriter` 触发者断言 | `internal/application/relation/message_test.go` | 保留关注通知触发者字段断言 |

补充的边界测试：`TestAnonymousFeedHidesViewerActionState` 验证匿名请求不带个人状态；`TestVideoStatCacheable` 验证空统计和非正视频编号不写入缓存；`TestInteractionStatCacheWithoutProviderUsesZeroBase` 验证仓储不支持计数读取时仍以零计数基数同步评论数。

## 验证结果

- 重新枚举来源全部 30 个文件，均与迁移表逐项对应；没有遗漏文件。
- `gofmt -w` 格式化全部改动 Go 文件后 `gofmt -l` 对改动文件无输出。
- `go vet ./...`：通过，退出码 0。
- `go test ./...`：通过。
- `go test -count=1 ./...`：通过，未使用缓存结果；全部包为 `ok`，无失败和跳过包。
- Web：`npm --prefix web test` 通过 18 个用例；`npm --prefix web run build` 构建成功。
- OpenAPI：YAML 解析通过，`openapi: 3.0.3`，42 个 operationId 全部唯一；`Message`、`CreateMessageRequest`、`FeedItemResponse` 已包含新增字段，`FeedItemResponse.required` 已加入 `liked`、`favorited`。
- `git diff --check`：通过。
- `docker compose config --quiet`：未执行成功，本机 Docker 守护进程未运行，本次未变更 Compose 文件。
- 新增或修改的函数、结构体均已添加紧邻中文注释。

## 已知限制与未验证内容

- 未执行容器内 `go test -race` 和真实 PostgreSQL、Redis、RabbitMQ 联调；条件式真实服务测试可能跳过。
- `AutoMigrate` 的并发重试在 VideoFeed 中属于补充防护，因为当前实现仍在事务与 `pg_advisory_xact_lock` 内串行迁移；PostgreSQL 事务内 DDL 失败后无法在同一事务继续执行，该重试只在事务未整体中止的场景生效，未通过真实并发启动验证。
- 来源提交没有采集触发者资料失败的重试或补偿逻辑，VideoFeed 保持一致，读取失败时触发者昵称为空。

## 最终审计

来源提交 30 个文件逐一重新枚举并对照：领域层 7 个文件、应用层 4 个文件、缓存与持久化 6 个文件、迁移 1 个文件、HTTP 接口 5 个文件、测试 4 个文件、Web 2 个文件，全部有对应迁移结果，其中推荐策略与样式表按 VideoFeed 真实文件名重新映射。新增的导出与未导出类型、接口方法、函数和常量均已核对；错误处理、幂等、失败回退与并发保护按上述差异逐项说明；`api/openapi.yaml` 与模块文档同步完成。

结论：来源提交的功能已完整迁移并按 VideoFeed 分层与 PostgreSQL 适配，未发现尚未迁移的行为；并发迁移重试和真实服务联调的限制已在上面列出。
