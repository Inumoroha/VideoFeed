# c160f400 新关注回填迁移审计

来源：C:/Users/HP/Desktop/GCFeed，提交 c160f400aa82c1bd92c9c5460027fb28313d77ad（fix(feed): 回填新关注小作者作品到 inbox）。目标：C:/Users/HP/Desktop/VideoFeed，开始时 main 分支、HEAD e5f0479，工作区干净。

已执行 git show --stat、--name-only、--format=fuller，逐文件读取全部 diff；核对目标 AGENTS.md、目录、Go 依赖、关系服务、Feed 仓储和缓存、Gin 路由、配置与 Compose。后端为 handler/application/domain/infrastructure 分层，PostgreSQL 使用 videos 表，Redis 和 RabbitMQ 已由 Register 装配。

## 全部源文件与映射

| 源文件 | 目标文件 | 是否新建 | 符号与适配 | 测试和文档 |
| --- | --- | --- | --- | --- |
| apps/api/internal/application/relation/service.go | internal/application/relation/service.go、backfill.go、error.go | 后两者中 backfill.go 新建 | New → NewService，新增可选参数不破坏旧调用；Service.backfiller、Option、FollowFeedBackfiller、WithFollowFeedBackfiller、backfillFollowFeed、两个常量和错误保留 | backfill_test.go、backfill_failure_test.go；关系/Feed 模块文档 |
| apps/api/internal/infra/cache/feed_cache.go | internal/infrastructure/cache/feed/following.go 与 internal/application/feed/service.go | 否；已有等价实现不修改 | ListFollowingIndexPage 的输入是全部有效关注作者，故小作者不被大作者列表误过滤；继续过滤取关条目并使用 ValidateFollowingIndex 校验视频状态 | 既有 following_test.go 与新增 router/follow_backfill_test.go |
| apps/api/internal/infra/persistence/feed/gorm.go | internal/infrastructure/persistence/feed/backfill.go | 是 | ListAuthorRecentVideos 原名迁入；video → videos；复用 feedPageItemsFromModels、StatusPublished | backfill_test.go 查询及错误断言 |
| apps/api/internal/interfaces/http/router/router.go | internal/router/register.go、follow_backfill.go | 适配器文件新建 | FollowFeedBackfiller、NewFollowFeedBackfiller、CountFollowers、ListAuthorRecentVideos、AddInboxItems 原名保留；Register 仅有 feedCache 时注入 | follow_backfill_test.go；OpenAPI 两个现有关注 PUT 路由 |
| apps/api/test/relation_api_test.go | internal/application/relation/backfill_test.go | 是 | 两个新增测试及 memoryFollowFeedBackfiller、backfillWrite、四个方法保留；原 newMemoryRelationRepo 对应现有 relationServiceRepository 的 backfillRelationRepo 初始化 | 小作者两条作品/作者/当前用户/1000 容量及大作者跳过断言全部保留 |

## 函数、类型和常量核对

- Service 新增 backfiller；NewService 遍历 Option；旧调用无需传选项。FollowFeedBackfiller 三个方法签名与源一致。
- defaultBackfillVideoLimit=100、defaultBackfillInboxMaxLen=1000、ErrBackfillFollowFeedFailed 错误文本逐项相同；BigCreatorFollowerThreshold 复用现有 10000。
- setFollow 保留目标已有初始化、领域错误及空结果保护；仅 active 且 follow.Active 时调用回填；失败转换为 ErrBackfillFollowFeedFailed。
- backfillFollowFeed：空依赖跳过；粉丝数读取失败即返回；达到阈值跳过；读取近期作品失败即返回；空条目跳过；对当前用户逐条 AddInboxItems，首个失败立即返回。
- ListAuthorRecentVideos：非法作者/数量返回空列表；仅已发布且有发布时间；发布时间、视频 ID 双重降序；按传入 limit 截断；错误透传；返回独立条目指针。
- 路由适配器三个委托方法不创建额外连接、事务、任务或资源；配置使用现有 cfg.Redis.Addr，无 Redis 时不注册回填能力。
- 原测试辅助方法 CountFollowers、ListAuthorRecentVideos、AddInboxItems、Writes 均保留互斥锁及复制语义。原两个 TestRelationFollowBackfillsSmallCreatorInbox、TestRelationFollowSkipsBigCreatorInboxBackfill 均迁入并通过。

## 并发、幂等与失败语义

数据库 SetFollow 先提交关注、统计和幂等快照；回填失败不会撤销这些提交。重复有效关注和同一幂等键重放均再次回填，没有自动重试。逐条写入可能部分成功；显式重试通过相同 Redis 成员去重。AddInboxItems 沿用 TxPipeline 中 ZADD、裁剪和 EXPIRE；容量 1000，有效期 30 天，没有弱化为普通管道。原提交未涉及 WATCH、Lua、RabbitMQ 或数据库唯一约束修改；目标现有相关保护均未更改。

源缓存修改不逐字套用：源传入大作者名单，取消 inbox 的名单过滤来保留小作者；目标传入全部关注名单，原本已能保留小作者，同时过滤取关。目标还会检查索引视频可见性，索引不足/故障回源数据库。沿用此等价行为可避免破坏已修复的取关和删除视频场景。

## 配置与接口

没有新增依赖、环境变量、数据表、Compose 服务、数据卷、健康检查或依赖顺序。继续使用 PostgreSQL、Redis、RabbitMQ 现有资源和关闭/重试机制，不增加 MQ 发布或消费逻辑。API 9000，前端 5173，数据库宿主机 5433。

补齐 OpenAPI 原先遗漏但已经实现的 PUT /api/relations/{targetUserId}、PUT /api/users/me/following/{targetUserId}。路由、JWT、路径参数、Idempotency-Key 和成功响应字段来自实际代码；记录 200/400/401/404/500，明确回填失败时关系已保存。未新增路由或改变 HTTP 错误格式。README、关系和 Feed 模块说明同步更新。

## 验证与限制

- gofmt：所有新增和修改 Go 文件已格式化；新增/修改命名函数和结构体紧邻中文注释已核对。
- go test ./...、go vet ./...、go test -race ./...：全部通过。
- 原两个测试均通过；补充各阶段错误、部分写入重试、取关跳过、关系仓储失败短路、SQL 表名/状态/排序/限制、适配器错误透传、24 个并发重复写入去重、TTL、小作者保留、取关过滤、Redis 不可用、HTTP 500 测试。
- git diff --check、docker compose config --quiet：通过。
- OpenAPI YAML 解析、operationId 唯一性、两个关注路由的参数/响应/认证与代码对照：通过。
- 重新列出源提交五个文件并逐项核对；没有发现遗漏的源提交新增行为。缓存调整已有目标等价实现，明确记录而非删除目标保护。
- 单元测试使用 sqlmock/miniredis；后续真实 PostgreSQL、Redis 和当前代码构建的 API 联调已完成，8 组检查全部通过。详情及边界见 [真实联调报告](backfill-live-verification.md)，脱敏结果见 [结果文件](backfill-live-results.json)。

迁移与联调完成时未创建 Git 提交；后续按用户要求将业务逻辑、测试与联调脚本、文档分主题提交。
