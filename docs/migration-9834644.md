# 9834644 异步关注流迁移审计

来源：`C:\Users\HP\Desktop\GCFeed` 的 `983464427014db244ac57e27913b3c1f48e18b43`，目标：`C:\Users\HP\Desktop\VideoFeed`。开始时 main 分支干净。已读取 `git show --stat`、`--name-only`、`--format=fuller` 和全部 17 个文件的完整 diff。

## 全部文件迁移表

源路径以 `apps/api/` 为前缀；测试简称以 `internal/` 为前缀。全部行为文档对应 `docs/modules/feed.md`，对外契约对应 `api/openapi.yaml`。

|源文件|目标文件|新建|类型、接口、函数、常量对应|数据库、配置和架构适配|测试位置|
|---|---|---|---|---|---|
|configs/config.docker.yaml|configs/config.docker.yaml|否|三个视频拓扑配置项|保持容器连接配置；videofeed 命名|config/video_test.go、Compose 解析|
|configs/config.yaml|configs/config.yaml|否|三个视频拓扑配置项|本地连接不变；videofeed 命名|config/video_test.go|
|internal/application/feed/service.go|internal/application/feed/{service,dto}.go|否|FollowingIndexCache、FollowingStrategy、WithFeedCache、listFollowingItems、listPageFromRepo|增加全部关注作者查询和可见性核验；缓存故障/不足回退|application/feed/index_test.go；handler/feed/index_test.go|
|internal/application/video/event.go|internal/application/video/event.go|是|PublishedEvent 全部 9 字段、NewPublishedEvent、newEventID|包路径改为当前模块；保留随机源失败时间兜底|application/video/fanout_retry_test.go|
|internal/application/video/fanout_worker.go|internal/application/video/fanout_worker.go|是|全部接口、选项、适配器、分发器及方法|批次500、阈值10000、inbox1000、outbox500、预热15分钟/15秒|application/video/fanout_worker_test.go、fanout_retry_test.go|
|internal/application/video/service.go|internal/application/video/service.go|否|Service、PublishedEventPublisher、Option、WithPublishedEventPublisher、publishCreatedVideo；New → NewService|保留目标 ready 校验和幂等竞态处理；发送失败不回滚|handler/video/api_flow_test.go|
|internal/domain/feed/entity.go|internal/domain/feed/feed.go|否|FeedPageItem.AuthorID|轻量索引增加作者；对外 JSON 字段不变|缓存、仓储和 HTTP 测试|
|internal/domain/feed/repository.go|internal/domain/feed/repository.go|否|Repository.ListFollowingPullAuthorIDs|保留大作者查询；扩展 ListFollowingAuthorIDs 供有效关注校验|persistence/feed/fanout_test.go|
|internal/infra/cache/feed_cache.go|internal/infrastructure/cache/feed/following.go|是|三个索引方法、七个键/编解码/排序辅助函数、followingIndexKeyTTL|事务管道替代普通管道；微秒 score、完整条目精确游标过滤；既有 WATCH 不变|cache/feed/following_test.go、shards_test.go|
|internal/infra/config/entity.go|internal/config/{config,env}.go|否|RabbitMQConfig.VideoExchange/VideoPublishedQueue/VideoPublishedRouting|yaml 标签映射 mapstructure；补环境变量显式绑定|config/video_test.go|
|internal/infra/mq/rabbitmq.go|internal/infrastructure/mq/rabbitmq.go|否|发布/消费视频方法、三默认常量、ensureTopology；normalizeRabbitMQConfig → normalizeConfig|适配 publish/consume/cfg 字段；持久化 topic 与队列；Ack/Nack 规则保留|mq/rabbitmq_test.go、video_live_test.go|
|internal/infra/persistence/feed/gorm.go|internal/infrastructure/persistence/feed/repository.go|否|ListFollowingPage、ListFollowingPullAuthorIDs、CountFollowers、ListFollowerIDs、basePageQuery、baseHotPageQuery|video → videos；PostgreSQL；users 卡片关联修复保留；新增可见性校验|persistence/feed/{following,fanout,repository,cards}_test.go|
|internal/infra/persistence/video/gorm.go|internal/infrastructure/persistence/video/repository.go|否|Save；移除 fanoutPublishedVideoToFollowers、authorFollowerCount|保留视频/统计事务和唯一约束；移除同步 SQL inbox 调用及仅用于该调用的类型|persistence/video/following_test.go|
|internal/interfaces/http/router/router.go|internal/router/register.go|否|Register|使用目标构造器、PostgreSQL 实例及路由；无 Redis 时仍可配置发布器；原互动链路保留|router 测试；真实账号联调|
|test/feed_api_test.go|internal/handler/feed/{following,index}_test.go|index 是|新增索引合并测试和全部辅助类型/方法；SQL 回退场景|路径 /api/feed-queries → /api/feed/query；作者查询计数映射全部关注查询|TestFollowingFeedUsesRedisIndex、TestFollowingFeedScene|
|test/video_api_test.go|internal/handler/video/api_flow_test.go|否|memoryVideoPublisher、PublishVideoPublished、EventCount、newVideoRouterWithPublisher、TestVideoPublishedEvent|保留真实 JWT 测试路由；201/200 与事件数断言|另补发送失败与16路并发幂等|
|test/video_fanout_worker_test.go|internal/application/video/fanout_worker_test.go|是|所有内存替身、方法和两个 Worker 测试|改为 video_test 包和当前模块引用；全部原断言保留|另补四阶段故障重试和真实 Redis 协议预热测试|

## 行为及资源审计

- 事件字段：EventID、VideoID、AuthorID、Title、Description、MediaURL、CoverURL、PublishedAt、OccurredAt 共 9 项；空视频或无发布时间不构造事件，文本去空白、时间 UTC，12 字节随机编号失败回退纳秒时间。
- 发布：仅 Save 成功且 Created=true 后发送；普通幂等命中和唯一约束竞态重放均不重复发。发送错误不回滚数据库，与参考提交一致。目标原有未初始化校验保留。
- Worker：PublishedEventConsumer、FanoutRepository、FollowingIndexCache、FeedPreheater、FeedCacheWriter、FanoutWorkerOption 均保留；NewFanoutWorker、NewFeedPreheater、PreheatFeedVideo、WithFanoutBatchSize、Start、HandleVideoPublished、preheat 均逐项核对。
- 阈值和常量：defaultFollowerBatchSize=500；BigCreatorFollowerThreshold=10000；inboxMaxLen=1000；outboxMaxLen=500；preheatTTL=15分钟；统计TTL=15秒；followingIndexKeyTTL=30天。三个 MQ 默认名称改为 videofeed.video / videofeed.video.published / video.published。
- 分发：卡片预热、统计预热、粉丝计数、粉丝分页、inbox/outbox 写入错误均返回；普通作者游标取最后一个粉丝编号，大作者不枚举粉丝。部分批次已成功时，重投从头开始，由 Redis ZADD 去重。
- Redis：新增写入使用 TxPipeline，使添加、裁剪、续期不被其他客户端插入；Redis 事务不提供命令错误回滚，失败交由消息重试。原有互动 WATCH、事务、计数分片及幂等逻辑未修改。
- MQ：复用目标连接与双通道；topic 交换机、持久化队列、指定 routing key、JSON、持久化消息、MessageId、Timestamp 均保留。成功 Ack(false)，无效 JSON Nack(false,false)，处理失败 Nack(false,true)。连接/通道创建或拓扑声明失败的清理，以及 Close 关闭通道和连接的逻辑保留。
- 数据库：Save 只在同一事务内写 videos/video_stat，作者幂等唯一键及错误映射保持；参考提交删除的两个同步 SQL 分发函数已删除。历史 feed_inbox 及唯一索引保留，不删除数据。关注查询使用当前关系状态、视频状态、非空发布时间和稳定双字段游标。CountFollowers 使用结构体扫描，缺失统计返回零。
- 路由：只修改依赖组装，不新增/删除 HTTP 路由或改变认证、状态码、请求/响应字段。OpenAPI 仅补异步可见性说明。原互动事件、热榜、users 表关联与 Web 路由均保留。
- 配置：两个 YAML 新字段、mapstructure 标签、三个环境变量全部对应；Go 依赖无需新增。Compose 服务、数据卷、健康检查、depends_on 不变。容器内 RabbitMQ 账号仍需与既有部署配置一致，本次未重建整套容器镜像。
- 注释：所有新增/修改的具名函数和结构体均已核对紧邻中文注释；测试替身包含在内。按迁移前基线比较了 116 项新增或修改的具名声明，未发现缺少中文注释的声明。

## 明确的适配差异

1. 参考实现用“大作者列表”过滤全部 inbox，会丢失普通作者。保留 ListFollowingPullAuthorIDs 的接口和阈值测试，同时增加 ListFollowingAuthorIDs 查询全部有效关注作者；线上读取以全部作者集合过滤并读取对应 outbox，兼容作者跨阈值后残留的 outbox。原 HTTP 测试的一次作者查询断言映射为该查询，不增加分页数据库查询。
2. 参考实现索引错误直接失败。本项目既有缓存故障降级行为保留，索引缺失、故障、不足 limit+1 或可见性校验失败均回源 PostgreSQL；数据库自身错误仍返回失败。
3. 参考 score 将秒与视频编号取模拼接，会丢失纳秒及大编号排序信息。本次以微秒 score 做存储，读取有容量上限的各集合后按完整时间、编号过滤排序，避免先截断再过滤导致空页。集合裁剪遇到同一微秒成员时仍采用 Redis 的成员排序，不能声称索引是完整历史。
4. 索引命中后批量核验公开状态及当前关系，删除或取关不能因预热卡片而再次展示；验证失效时回源数据库补页。
5. 新增写入由普通 Pipeline 改为 TxPipeline，增强并发更新的一致性；不改变现有 WATCH 事务。

## 测试迁移审计

- TestFollowingFeedScene：数据库回退按有效关注直接读取全部公开视频，不再依赖 SQL inbox；原有认证、分页、空页、非法输入与用户隔离覆盖保留。
- TestFollowingFeedUsesRedisIndex：保留 inbox/outbox 合并后 [4,2]、has_more=true、SQL页调用0、作者查询1断言；相关缓存克隆、合并、去重和排序辅助方法全部迁移。
- TestVideoPublishedEvent：保留首次201且事件1，重放200且事件仍1；发布器、计数和构造辅助函数全部迁移。
- TestFanoutWorkerPushesSmallCreatorInbox：保留3粉丝/批次2、分发目标顺序、无outbox和预热视频编号断言。
- TestFanoutWorkerWritesBigCreatorOutbox：保留阈值10000、无inbox和作者/视频编号断言。
- 新增：32路并发重复写入与裁剪、TTL、跨用户隔离、跨索引去重、纳秒时间与大编号游标、取关过滤、无效输入、Redis断开错误、索引故障/缺失/不足回退。
- 新增：粉丝计数、第二批粉丝、卡片读取、统计读取四阶段故障后重试，首次部分成功不导致重复索引；真实 Redis 协议验证预热有效期。
- 新增：完整事件字段/随机编号/UTC、发送失败不回滚与不重复发送、16路并发幂等只发一次。
- 新增：PostgreSQL 查询字段、状态、阈值、缺失统计、游标及错误传播；发布事务统计失败回滚后可重试，并确认不再访问 SQL inbox。
- 新增：配置缺省时三个环境变量绑定、自定义/空白 MQ 拓扑；实际 RabbitMQ 路由、一次失败后重投、无效 JSON 拒绝和连接清理。

## 验证结果

- `gofmt`：全部修改/新增 Go 文件已格式化。
- `go test ./...`：通过（普通运行中真实 MQ 测试按环境变量跳过；已单独配置本地消息服务实际运行通过）。
- `go vet ./...`：通过。
- `git diff --check`：通过。
- `docker compose config --quiet`：通过。
- OpenAPI：Python YAML 解析通过，22 个 operationId 无重复；本次没有路由或结构变更。
- 真实 MQ：`VIDEOFEED_TEST_AMQP_URL` 配置到本地专用 RabbitMQ 后，`TestVideoQueueLive` 通过，测试创建的独立队列与交换机已清理。
- 真实账号联调：通过前端 5173 代理调用重新构建的 API 9000，使用隔离数据库 `videofeed_live_20260918`、Redis DB15 和独立视频消息队列，共 13 项断言通过：健康检查、9999 inbox、10000 outbox、无同步 SQL inbox、卡片预热、合并排序、游标回退无重复、8 路幂等、索引去重、取关、删除、并发用户隔离、30 天 TTL。
- `go test -race`：尝试执行，但 Windows 当前 PATH 缺少 gcc，cgo 编译失败，未获得竞态检测结果；普通并发测试已通过。

## 边界与未验证内容

参考提交没有数据库事件 outbox、publisher confirms、自动重连、失败退避、死信策略或历史索引回填，本次没有虚构这些能力。数据库提交后进程退出或发送失败可能丢事件；完整的一页缓存可能暂时漏掉消费中的视频、新关注历史或缺失的另一作者索引。已补故障/不足页回退，但这不是全量一致性水位机制。后台消费者仍沿用原应用装配生命周期，未新增停机 drain 协议。

已验证本地真实 PostgreSQL/Redis/RabbitMQ 及真实测试账号，未验证生产流量、万人实际粉丝分页负载、生产故障恢复、所有同微秒裁剪边界或完整 Docker 镜像部署。10000 阈值实测通过隔离数据库中的粉丝统计模拟，不等于创建一万个账号。所有凭据仅保留在临时联调目录。

## 逐声明复核清单

以下清单通过比较参考提交与父提交的顶层声明生成，并核查对应目标声明存在；具体行为差异见上文。删除项核查目标旧实现已移除。

|参考文件（apps/api/ 下）|新增或修改的声明|删除的声明|
|---|---|---|
|internal/application/feed/service.go|`FollowingIndexCache`, `FollowingStrategy`, `WithFeedCache`, `listPageFromRepo`, `listFollowingItems`|无|
|internal/application/video/event.go|`PublishedEvent`, `NewPublishedEvent`, `newEventID`|无|
|internal/application/video/fanout_worker.go|`defaultFollowerBatchSize`, `PublishedEventConsumer`, `FanoutRepository`, `FollowingIndexCache`, `FeedPreheater`, `FeedCacheWriter`, `FanoutWorker`, `FanoutWorkerOption`, `NewFanoutWorker`, `FeedPreheaterAdapter`, `NewFeedPreheater`, `PreheatFeedVideo`, `WithFanoutBatchSize`, `Start`, `HandleVideoPublished`, `preheat`|无|
|internal/application/video/service.go|`Service`, `PublishedEventPublisher`, `Option`, `New`, `WithPublishedEventPublisher`, `CreatePublished`, `publishCreatedVideo`|无|
|internal/domain/feed/entity.go|`FeedPageItem`|无|
|internal/domain/feed/repository.go|`Repository`|无|
|internal/infra/cache/feed_cache.go|`followingIndexKeyTTL`, `AddInboxItems`, `AddAuthorOutboxItem`, `ListFollowingIndexPage`, `followingInboxKey`, `followingAuthorOutboxKey`, `followingIndexScore`, `followingIndexMember`, `feedPageItemFromFollowingMember`, `int64Set`, `sortFeedPageItemsByTimeline`|无|
|internal/infra/config/entity.go|`RabbitMQConfig`|无|
|internal/infra/mq/rabbitmq.go|`defaultVideoExchange`, `defaultVideoPublishedQueue`, `defaultVideoPublishedRouting`, `PublishVideoPublished`, `ConsumeVideoPublished`, `ensureTopology`, `normalizeRabbitMQConfig`|无|
|internal/infra/persistence/feed/gorm.go|`ListFollowingPage`, `ListFollowingPullAuthorIDs`, `CountFollowers`, `ListFollowerIDs`, `basePageQuery`, `baseHotPageQuery`|无|
|internal/infra/persistence/video/gorm.go|`Save`|`fanoutPublishedVideoToFollowers`, `authorFollowerCount`|
|internal/interfaces/http/router/router.go|`Register`|无|
|test/feed_api_test.go|`memoryFeedRepo`, `memoryFeedCache`, `newMemoryFeedCache`, `FollowingPullAuthorCalls`, `ListFollowingIndexPage`, `AddInboxItemsForTest`, `AddAuthorOutboxItemForTest`, `ListFollowingPage`, `ListFollowingPullAuthorIDs`, `TestFollowingFeedScene`, `TestFollowingFeedUsesRedisIndex`, `feedPageItemFromFeedItem`, `filterAndSortTimelineItems`|无|
|test/video_api_test.go|`memoryVideoPublisher`, `PublishVideoPublished`, `EventCount`, `TestVideoPublishedEvent`, `newVideoRouter`, `newVideoRouterWithPublisher`|无|
|test/video_fanout_worker_test.go|`memoryFanoutRepo`, `CountFollowers`, `ListFollowerIDs`, `BatchGetFeedCards`, `BatchGetFeedStats`, `memoryFollowingIndex`, `AddInboxItems`, `AddAuthorOutboxItem`, `memoryFeedPreheater`, `PreheatFeedVideo`, `TestFanoutWorkerPushesSmallCreatorInbox`, `TestFanoutWorkerWritesBigCreatorOutbox`|无|

## 提交与结论

- `69dec58`：基础设施、事件契约、消息拓扑、环境配置与 Redis 索引。
- `fcc6d55`：视频发布事件、分发 Worker、关注查询与 PostgreSQL 适配。
- `dab2834`：原测试迁移、回归测试、并发幂等及真实消息链路测试。
- 文档主题提交：本报告、Feed 模块说明、README 和 OpenAPI 描述。

17 个参考文件均已映射，新增/修改/删除声明均已核对，没有发现尚未处理的参考变更。与原提交不完全相同的行为已逐项列出，未将通过 go test 等同于具备可靠必达或生产验证。提交按上述顺序应用；业务接口与测试替身分属不同主题，应应用完整提交序列。
