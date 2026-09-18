# Day 9：Feed 缓存优化与热点计数分片

## 今日目标

在 Day 8 互动异步链路的基础上，迁移 Feed 缓存优化与计数分片改动，减少批量写入的网络往返和不同用户更新同一视频时的事务冲突，并补充并发、幂等与失败路径测试。

## 完成内容

- 将 Feed 主调用链统一为 `FeedResult`、`feedItemsResponse`、`ListFeedItems` 和 `RefreshFeed`；现有 `TimelineFeedResult`、`Timeline`、`RefreshTimelineFeed`、`WithTimelineCache` 兼容入口仍保留。
- 简化视频仓储 `EnsureStats`：仅为缺少统计记录的视频补零，不再读取旧视频表的计数字段。
- 将 `SetCards`、`SetStats` 改为 Pipeline 批量写入，保留无有效数据时直接返回的行为。
- 热榜窗口仅在键不存在时重建，合并最近 60 个分钟桶，通过 Pipeline 清理非正分数并设置过期时间。
- 将点赞、收藏计数拆为基数与 16 个增量分片；保留本项目用户状态中的 `active` 字段。
- 移除旧单哈希计数键的读取兼容，基数初始化直接使用传入的统计数据，未提供时使用零计数。
- 引入 miniredis 测试依赖，迁移原仓库测试并补充事务、回退和幂等验证。

## 关键设计

### 分片写入与聚合

```text
点赞或收藏请求
  -> WATCH 用户行为键
  -> 读取状态并计算增量；重复幂等键保留原状态，增量为零
  -> MULTI/EXEC
       保存 active 和幂等键
       HSETNX 初始化基数字段
       HINCRBY 累加当前用户对应的分片
       设置行为状态、基数和分片有效期
  -> 提交后读取基数与全部分片，聚合点赞和收藏计数
  -> 尽力回填短期 JSON 缓存并返回结果
```

正数用户编号通过 `userID % 16` 选择分片。事务仅监视用户行为键，不再监视同一视频共享的计数键和 JSON 键；基数通过 `HSETNX` 避免覆盖，增量通过 `HINCRBY` 原子累加。WATCH 冲突直接返回错误，缓存层没有新增自动重试。

| Redis 键 | 用途 | 有效期 |
|---|---|---|
| `interaction:action:v1:{user_id}:{video_id}:{action}` | 用户互动状态及幂等键 | 30 天 |
| `video:stat:counter:v1:{video_id}:base` | 计数基数 | 24 小时 |
| `video:stat:counter:v1:{video_id}:shard:{00..15}` | 点赞、收藏增量 | 24 小时 |
| `video:stat:v1:{video_id}` | 聚合统计 JSON；互动回填时使用短期有效期 | 15 秒 |

`video:stat:counter:v1:{video_id}` 旧键不再作为统计来源，也不会被本次改动主动删除。升级后不能依赖其中尚未持久化的计数；初始化来源是调用方传入的统计数据。

### 读取与失败处理

`GetStats` 优先通过 MGET 读取 JSON，未命中时读取基数并聚合全部分片。基数缺失时，聚合路径先尝试 JSON，再在 JSON 不存在时使用传入的初始统计；`GetStats` 本身不提供初始统计，完全未命中时由上层处理回源。点赞和收藏的最终聚合值限制为非负。

分片读取优先使用 Pipeline，不支持 Pipeline 的读取替身逐个查询。读取错误向上传递，JSON 回填失败不丢弃已聚合结果。聚合发生在写事务之后，不是跨分片快照；若提交后的聚合读取失败，接口仍会收到错误。这些边界沿用参考提交。

## 文件与迁移对应

| 原仓库文件或变更 | 当前项目位置 |
|---|---|
| `apps/api/internal/infra/cache/feed_cache.go` | `internal/infrastructure/cache/feed/feed.go` |
| `apps/api/internal/infra/cache/feed_cache_test.go` | `internal/infrastructure/cache/feed/feed_test.go` |
| 补充的事务、并发及回退测试 | `internal/infrastructure/cache/feed/shards_test.go` |
| 测试依赖 | `go.mod`、`go.sum` |

缓存部分依次参考 `cb176004`（批量读写优化）、`7bd66f8`（计数分片）和 `c5f21d3`（移除旧键兼容）。实现沿用当前模块路径和分层，没有引入原仓库包名。PostgreSQL 仓储、RabbitMQ 拓扑、HTTP 路由和响应结构没有因缓存改动而调整，无需修改 OpenAPI 接口定义。

## 测试与验证

缓存包目前包含 11 项测试：原提交对应的 4 项测试，以及 7 项补充测试。覆盖基数与正负分片聚合、初始统计回退、JSON 未命中回填、同分片并发更新、重复请求及取消、WATCH 冲突后的显式重试、旧状态保留、读取和回填失败、收藏幂等重放，以及旧计数键被忽略。

本轮代码迁移完成时已执行：

- `gofmt`：修改的 Go 文件已格式化。
- `go test ./...`、`go vet ./...`：通过。
- `git diff --check`：通过。
- `docker compose config --quiet`：通过。
- `api/openapi.yaml`：YAML 解析及 `operationId` 唯一性检查通过。

并发事务测试使用隔离的 miniredis 服务。`-race` 检查因本机缺少 GCC 未完成；真实 Redis、PostgreSQL 和 RabbitMQ 联调尚未完成，不能将模拟测试视为生产联调结果。本次开发记录提交只涉及 Markdown，不重复运行全量代码测试。

## 提交记录

- `3810b88 refactor(infra): 清理视频统计兼容逻辑`
- `9e1d312 refactor(feed): 统一信息流条目接口命名`
- `6aa7932 docs(feed): 更新信息流架构文档`
- `d01202a perf(feed): 优化 Redis Feed 缓存读写`
- `be841ff chore(deps): 增加缓存事务测试依赖`
- `1f8e228 feat(cache): 迁移热点计数分片`
- `2fe9100 test(cache): 覆盖分片计数与事务失败场景`
- `e71ee0a refactor(cache): 移除计数分片旧键兼容`
- `4553445 test(cache): 验证移除旧计数键后的回退行为`

## 后续计划

- 补齐编译工具链并运行竞争检测，在真实 Redis 中验证事务冲突、过期与故障恢复。
- 评估基数和分片独立过期、非快照聚合及并发 JSON 回填的一致性边界。
- 联调 Redis 更新、RabbitMQ 发布和 PostgreSQL 落库，验证提交后读取失败时的业务恢复路径。
- 通过请求延迟、Redis 命令数和事务冲突率衡量优化收益，避免仅凭实现结构判断性能提升。
