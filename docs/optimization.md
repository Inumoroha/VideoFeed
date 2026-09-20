# VideoFeed 性能与稳定性优化

本文沉淀 Feed 核心链路的性能问题、优化策略和验收指标。架构总览见 [architecture.md](architecture.md)，代码规范见 [engineering.md](engineering.md)。

## 1. 优先级边界

| 优先级 | 目标 | 典型问题 |
| --- | --- | --- |
| P0 | 保证首发可用 | Timeline 高访问、Feed 组装慢、游标重复、互动并发、缓存一致性 |
| P1 | 提升体验 | Hot 热 key、发布放大、预加载、播放 QoS |
| P2 | 支持扩展 | 多机部署、服务拆分、推荐特征、监控告警闭环 |

## 2. 核心架构

```mermaid
flowchart LR
    Client["Web Client"] --> API["Gin API"]
    API --> PostgreSQL["PostgreSQL"]
    API --> Redis["Redis"]
    API --> RabbitMQ["RabbitMQ"]
    RabbitMQ --> Worker["Worker"]
    Worker --> PostgreSQL
    Worker --> Redis

    API --> FeedService["Feed Service"]
    FeedService --> PageCache["Feed Page Cache"]
    FeedService --> CardCache["Video Card Cache"]
    FeedService --> StatCache["Video Stat Cache"]
```

Redis 用于读性能和短期状态，RabbitMQ 用于削峰和异步落库，PostgreSQL 保存最终事实。

## 3. P0 优化清单

| 编号 | 问题 | 策略 | 验收指标 |
| --- | --- | --- | --- |
| P0-01 | Timeline 首页大量访问 | 页缓存、短 TTL、singleflight 回源 | 首页 P95 响应稳定在可接受范围 |
| P0-02 | Feed 卡片组装慢 | 页缓存只存 ID，卡片和计数批量 MGET | 单页查询减少 N+1 回源 |
| P0-03 | 分页重复和漏数 | 游标携带排序字段，稳定排序 | 翻页结果无重复且顺序稳定 |
| P0-04 | 数据库缓存一致性偏差 | 写事实表，缓存短 TTL，异步更新 | 计数最终一致，缓存异常可回源 |
| P0-05 | 并发点赞收藏评论 | Redis 快速状态、RabbitMQ 异步落库、幂等键 | 重复请求计数稳定 |
| P0-06 | 大 V 发布放大 | 粉丝数阈值、异步 fanout、懒加载补偿 | 发布接口不被粉丝量线性拖慢 |
| P0-07 | 热门视频热 key | 分钟桶 ZSET、窗口合并、短期窗口缓存 | Hot 查询避免集中打 PostgreSQL |

## 4. Timeline 访问

问题：Timeline 首页访问频率高，直接查询 PostgreSQL 会把压力集中到 `videos` 和 `video_stat`。

优化：

- `feed:page:v1:timeline:limit:{limit}:first` 保存首页排序结果。
- `feed:page:v1:timeline:limit:{limit}:cursor:{cursorHash}` 保存后续页。
- 页缓存只保存 `video_id` 和排序字段。
- Feed Service 通过批量 MGET 读取 `video:card` 和 `video:stat`。
- 回源时使用 singleflight 合并同 key 请求。

当前 TTL（页缓存附加按键计算的 10%–20% 抖动）：

| 缓存 | TTL |
| --- | --- |
| 首页页缓存 | 5 秒 + 抖动 |
| 后续页缓存 | 45 秒 + 抖动 |
| 视频卡片 | 15 分钟 |
| 视频计数 | 15 秒 |

## 5. Feed 卡片组装

问题：Feed 返回项需要视频主体、作者信息和计数，逐条查询会形成 N+1。

优化：

1. 页查询得到视频 ID 列表。
2. Redis 批量读取视频卡片和计数。
3. 缺失项批量回源 PostgreSQL。
4. 回源结果写回 Redis。
5. 按原始排序组装响应。

验收：

- 单页 Feed 查询使用批量读取。
- Redis 缺失时最多一次批量视频查询和一次批量计数查询。
- 返回顺序与页缓存或数据库排序一致。

## 6. 游标分页

问题：只用 offset 翻页时，新视频插入会导致重复或漏数。

优化：

| 场景 | 排序 | 游标字段 |
| --- | --- | --- |
| Timeline | `published_at DESC, id DESC` | `published_at`、`video_id` |
| Hot（Redis） | 分数倒序，同分按 ZSET 成员字典序倒序 | `window_end`、`offset` |
| Hot（未配置缓存） | `hot_score DESC, published_at DESC, id DESC` | `hot_score`、`published_at`、`video_id` |
| Following | `published_at DESC, id DESC` | `published_at`、`video_id` |
| Recommend | 评分、发布时间、ID，之后进行作者打散 | `score`、`published_at`、`video_id` |
| 评论 | `created_at DESC, id DESC` | `created_at`、`comment_id` |

验收：

- 同一排序字段下使用 ID 作为稳定次级排序。
- Timeline/Hot/Following 的非法游标返回 400；Recommend 通过 Feed 的错误映射目前返回 500，内部候选接口返回 400。
- 多页查询结果无重复。

## 7. 缓存一致性

原则：

- PostgreSQL 保存最终事实。
- Redis 保存短期状态、热榜和读缓存。
- 写路径优先保证事实安全。
- 读路径允许短 TTL 偏差。

启用异步管线时的点赞/收藏写入（评论主记录与计数同步写数据库事务）：

```text
HTTP Handler
  -> Interaction Service
  -> Redis 行为状态和实时计数
  -> RabbitMQ ActionChangedEvent
  -> Worker
  -> PostgreSQL interaction_action / video_stat
```

异常处理：

| 异常 | 处理 |
| --- | --- |
| Redis 不可用 | Timeline/卡片/统计、Following 可回源；Hot 已配置缓存时返回 500；互动快速状态失败返回 500 |
| RabbitMQ 投递失败 | 点赞/收藏同步写仓储；发布视频和曝光保留已提交数据库结果，无持久化失败任务 |
| Worker 重复消费 | 使用唯一键和幂等键保证安全 |
| 缓存计数偏差 | 存在 16 分片及基础值，JSON TTL 不等价于全量数据库对账；没有定时校准器 |

## 8. 发布放大

问题：作者发布视频后，如果同步写入所有粉丝 inbox，发布耗时会随粉丝数线性增长。

优化：

- 所有视频发布在事务提交后投递消息，由独立 worker 异步处理。
- 粉丝不足 10000 的作者按每批 500 个粉丝推送 inbox；达到阈值只写作者 outbox，读取关注流时合并。
- inbox 最多 1000 条、outbox 最多 500 条，两者 TTL 均为 30 天；事务管道执行添加、裁剪、续期。
- 新关注小作者同步回填最近 100 条作品；失败返回 500，不回滚已提交关系，可用原幂等键重试。
- 索引缺失、不足或异常时回源 PostgreSQL，命中仍核验公开状态和关注关系。

验收：

- 发布接口响应时间与粉丝数解耦。
- Worker 可重复消费同一发布事件。
- inbox 长度受控。

## 9. Hot Feed

Hot Feed 使用 Redis ZSET 维护一小时滑动窗口。

| 行为 | 分数 |
| --- | --- |
| 点赞 | +3 |
| 收藏 | +4 |
| 评论 | +5 |
| 取消点赞 | -3 |
| 取消收藏 | -4 |
| 删除评论 | -5 |

Key：

```text
feed:hot:minute:v1:{yyyyMMddHHmm}
feed:hot:window:v1:{windowEndUnix}
```

读取时合并最近 60 个分钟桶，移除分数小于等于 0 的条目，再按分数倒序分页。

分钟桶保留 2 小时，窗口汇总缓存 2 分钟。无缓存配置时使用数据库累计热度；已配置 Redis 后窗口读取错误不会自动切到数据库。窗口期间热度仍可能变化，不承诺跨页快照完全不重复。

## 10. 监控指标

以下是建议采集的指标，当前没有通用采集接口、告警或看板，也不代表已经达到性能 SLO：

| 指标 | 说明 |
| --- | --- |
| `feed_request_p95_ms` | Feed 请求 P95 |
| `feed_cache_hit_ratio` | Feed 页缓存命中率 |
| `video_card_cache_hit_ratio` | 视频卡片缓存命中率 |
| `interaction_queue_lag` | 互动队列积压 |
| `interaction_worker_error_count` | Worker 错误数 |
| `postgres_query_p95_ms` | PostgreSQL 查询 P95 |
| `redis_error_count` | Redis 错误数 |
| `rabbitmq_publish_error_count` | MQ 投递错误数 |

## 11. 落地顺序

1. 先确保游标分页和批量组装稳定。
2. 再接入 Feed 页缓存、卡片缓存和计数缓存。
3. 再完成互动异步落库与 Worker 幂等。
4. 再补发布 fanout 和 Hot 窗口缓存。
5. 最后补监控指标和降级开关。

## 12. 原优化专题的规划归档

旧长文中的设计方向保留在此，以下内容尚不是已交付能力：事务发件箱与全量缓存失效事件、上一版页缓存兜底、作者资料缓存、布隆过滤器和空值缓存、进程内热点卡片、入口限流与背压、MQ 死信重放和退避、读写分离及连接池隔离、批量异步曝光和分区、CDN 首段预热、播放 QoS、链路追踪及自动告警。已有 singleflight、TTL 抖动和互动 16 分片与这些规划分开理解。

旧文提出的 Feed P95 <100ms/P99 <300ms、缓存命中率 >90%、下架传播 <3s、互动计数延迟 <3s、MQ 延迟 <5s、首帧 <500ms 均为待压测目标，不是测得结果。落地顺序仍为分页→缓存与批量组装→一致性与幂等→异步分发→热点/高峰保护→指标告警。

审查实现时保持 WATCH 用户行为键、事务更新分片和原子累加。当前 MQ 成功逐条确认，非法 JSON 拒绝不重入队，处理失败重新入队；没有发布确认、自动重连或可靠发件箱。外部资源取消与关闭边界见 [部署说明](deployment.md)。
