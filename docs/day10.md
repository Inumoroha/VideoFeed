# Day 10：关注流推拉混合、异步分发与真实账号联调

## 今日目标

在 Day 9 的 Feed 缓存和计数分片基础上，接入关注流及前端入口，再将发布时的同步粉丝分发迁移为 RabbitMQ 异步消费。迁移过程中对照当前 PostgreSQL、应用分层和 HTTP 路由，保留幂等、重试、缓存回退和用户隔离行为，并使用真实测试账号验证完整链路。

## 完成内容

- 迁移关注流基础能力：支持 `scene=following`，通过访问令牌识别当前用户，按发布时间与视频编号稳定分页。
- 增加前端 `/following` 入口、未登录提示及移动端导航，复用现有 Feed 请求和视频播放能力。
- 在真实账号联调中修复卡片查询关联旧 `account` 表的问题，改为当前项目的 `users` 表，并增加回归测试。
- 迁移参考提交 `983464427014db244ac57e27913b3c1f48e18b43` 的 17 个文件变更，逐项核对类型、函数、常量、配置和测试。
- 新增视频发布事件、分发 Worker 和缓存预热，移除视频事务内的同步 SQL inbox 写入。
- 为普通作者维护粉丝 Redis inbox，为大作者维护 author outbox；读取时合并索引，并在必要时回源 PostgreSQL。
- 修复参考实现只使用大作者名单过滤 inbox 的问题，避免普通作者视频被错误排除。
- 补充并发幂等、部分失败重试、索引裁剪、取关过滤、删除可见性和真实 RabbitMQ 测试，更新 README、模块文档及 OpenAPI 行为说明。

## 关键设计

### 发布与消费

```text
POST /api/videos
  -> 校验身份、参数和幂等键
  -> PostgreSQL 事务保存 videos 与 video_stat
  -> 仅首次创建成功后发布 PublishedEvent
  -> RabbitMQ 视频队列
  -> FanoutWorker 消费
       预热视频卡片与统计
       读取作者粉丝数
       少于 10000：每批读取 500 名有效粉丝，写入用户 inbox
       达到 10000：只写作者 outbox
  -> 成功确认消息；处理失败重新入队
```

`PublishedEvent` 保存事件编号、视频编号、作者编号、标题、描述、媒体地址、封面地址、发布时间和事件发生时间。文本去除首尾空白，时间统一为 UTC；事件编号优先使用 12 字节随机值，随机源失败时使用纳秒时间兜底。

普通幂等命中和并发唯一约束冲突后的重放均不重复发送事件。消费时先预热，再分发；卡片、统计、粉丝查询或索引写入失败都会返回错误，由消费者重新入队。无效 JSON 拒绝且不重新入队。

视频与初始统计仍在同一数据库事务内保存。历史 `feed_inbox` 表及索引保留，但当前发布与关注流读取不再依赖该表。

### 索引与预热参数

| 数据或配置 | 当前值 | 用途 |
|---|---|---|
| `feed:following:inbox:v1:{userId}` | 最多 1000 条，30 天 | 保存普通作者向粉丝分发的视频索引 |
| `feed:following:author:v1:{authorId}` | 最多 500 条，30 天 | 保存大作者视频索引，读取时合并 |
| 视频卡片预热 | 15 分钟 | 减少卡片组装时的数据库读取 |
| 视频统计预热 | 15 秒 | 提供短期互动统计缓存 |
| 粉丝分发批次 | 500 人 | 分页读取有效粉丝 |
| 大作者阈值 | 10000 名粉丝 | 决定写用户 inbox 或作者 outbox |

索引使用有序集合去重，通过 `TxPipeline` 连续执行添加、容量裁剪和有效期刷新。消息部分批次成功后重试，会从头分发，重复条目由 `ZADD` 去重。Redis 事务不提供命令执行错误的自动回滚，错误仍需交由消息重试处理。

既有互动缓存的 WATCH、用户行为幂等键和计数分片没有改动。

### 读取与回退

`GET /api/feed-items?scene=following` 和 `POST /api/feed/query` 保持现有认证、请求字段与响应结构。前端仍通过 `/following` 展示关注视频。

读取时查询当前用户的全部有效关注作者，合并 inbox 与对应 author outbox，排除已取关作者，按完整发布时间和视频编号倒序去重分页。索引以微秒时间作为存储分值，读取后的游标过滤使用完整时间和视频编号，避免参考实现按秒和编号取模导致的分页错误。

命中索引后，数据库还会批量核验视频公开状态与有效关注关系，避免已删除视频通过预热卡片再次展示。索引缺失、不足 `limit+1`、读取失败或可见性校验失效时，直接查询当前关注作者的公开视频。数据库自身读取失败仍返回错误。

关注流不使用跨用户共享的分页缓存。非空末页仍可带 `next_cursor`，客户端以 `has_more=false` 判断结束。

## 文件与配置对应

| 能力 | 当前项目位置 |
|---|---|
| 发布事件与异步分发 | `internal/application/video/event.go`、`fanout_worker.go`、`service.go` |
| 关注索引读取策略 | `internal/application/feed/service.go`、`dto.go` |
| Redis 索引 | `internal/infrastructure/cache/feed/following.go` |
| RabbitMQ 发布、消费和拓扑 | `internal/infrastructure/mq/rabbitmq.go` |
| PostgreSQL 关注查询与视频事务 | `internal/infrastructure/persistence/feed/repository.go`、`internal/infrastructure/persistence/video/repository.go` |
| 配置与依赖组装 | `internal/config/`、`configs/`、`internal/router/register.go` |
| 对外契约及模块说明 | `api/openapi.yaml`、`docs/modules/feed.md` |

新增 `rabbitmq.video_exchange`、`video_published_queue`、`video_published_routing`，默认值分别为 `videofeed.video`、`videofeed.video.published`、`video.published`。对应环境变量为 `RABBITMQ_VIDEO_EXCHANGE`、`RABBITMQ_VIDEO_PUBLISHED_QUEUE`、`RABBITMQ_VIDEO_PUBLISHED_ROUTING`。

沿用 `docker-compose.yaml` 中的 PostgreSQL、Redis、RabbitMQ、数据卷、健康检查和依赖顺序，没有新增服务或 Go 依赖。容器部署时需将配置覆盖变量传入 API 容器，并保证消息服务账号与配置一致。

完整文件映射、逐声明复核及适配差异见[异步迁移审计](migration-9834644.md)。此前的基础关注流和前端迁移分别记录在[关注流迁移审计](migration-1517e5a.md)与[前端迁移审计](migration-7d58cd0.md)。

## 测试与真实联调

保留参考提交的关注索引合并、首次发布与幂等重放、普通作者分批推送和大作者 outbox 测试。新增覆盖包括：32 路并发重复索引写入、容量裁剪与有效期、精确游标、取关和用户隔离、缓存失败回退、四阶段故障重试、完整事件字段，以及 16 路同键发布只产生一个事件。

本轮代码迁移完成时已执行：

- `gofmt`：新增和修改的 Go 文件已格式化。
- `go test ./...`、`go vet ./...`：通过。
- `git diff --check`、`docker compose config --quiet`：通过。
- OpenAPI YAML 解析与 22 个 `operationId` 唯一性检查：通过。
- 真实 RabbitMQ 隔离拓扑测试：路由、失败后重投、无效 JSON 拒绝通过，测试队列与交换机已清理。

真实账号联调通过前端 5173 代理调用重新构建的 API 9000，使用专用 PostgreSQL 数据库、Redis DB15 和独立视频队列。13 项断言覆盖健康检查、9999/10000 分发分界、无同步 SQL inbox、卡片预热、索引合并、游标回退、8 路幂等发布、索引去重、取关、删除、并发用户隔离及 30 天有效期。

10000 粉丝分界通过隔离数据库中的统计值模拟，不代表创建了一万个真实账号或完成万人扇出压测。此前同步阶段的真实账号及浏览器验证见[联调报告](following-live-integration.md)，其中同步 inbox 断言属于历史实现，不作为当前异步分发的验收标准。

`go test -race` 因本机缺少 gcc 未能完成，不能将普通并发测试视为竞态检测通过。本次 Day 10 记录仅新增 Markdown，检查文档路径、链接及差异，不重复运行全量代码测试。

## 提交记录

- `69dec58 feat(infra): 增加视频事件拓扑与关注流缓存索引`
- `fcc6d55 feat(feed): 接入异步关注分发与数据库回退`
- `dab2834 test(feed): 覆盖异步分发幂等重试与真实消息链路`
- `91aa987 docs(feed): 记录异步分发迁移审计与联调结果`

业务接口与测试替身按主题拆分，应用迁移时应使用完整提交序列。

## 当前边界与后续计划

数据库提交与消息发送不是同一事务，发送失败不回滚视频；当前未实现持久化事件 outbox、发布确认或自动重连，因此不能保证消息必达。失败消息直接重新入队，尚无退避、死信策略和停机排空协议。

索引有容量和有效期限制，也没有历史回填或完整性水位。即使索引足够返回一页，也可能暂时缺少消费中的视频、新关注作者历史或另一作者缺失的索引；不足页回退不能替代完整性校验。同一微秒内的容量裁剪仍使用 Redis 成员排序，读取时的精确排序不能恢复已裁剪条目。

- 补齐编译工具链，运行竞态检测。
- 设计数据库事件 outbox、发布确认、重连、退避和死信重放流程。
- 补充新关注、作者跨阈值和索引过期后的历史回填与完整性校验。
- 增加消费延迟、队列积压、回退比例和分发失败监控。
- 开展真实大规模粉丝分发、生产故障恢复及完整容器部署验证。
