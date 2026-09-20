# Day 8：互动异步落库与项目阅读导览

## 今日目标

在已有点赞、收藏和 Feed 热度能力的基础上，引入 Redis 快速状态、RabbitMQ 异步落库和互动 Worker，并完善项目部署说明与代码阅读入口。

## 完成内容

- 增加 RabbitMQ 连接、交换机、队列、路由键、事件发布和消费实现。
- 为点赞、收藏增加 Redis 用户状态、幂等键和实时计数缓存。
- 增加 `ActionChangedEvent` 和 `ActionWorker`，将互动事实异步写入 PostgreSQL。
- 增加 Redis `WATCH` 事务，保护状态读取、增量计算和计数写回的一致性。
- 增加 RabbitMQ 发布失败时的同步落库回退逻辑。
- 增加互动统计快照读取和 Redis 计数下限保护，避免计数出现负数。
- 将 RabbitMQ 加入 Docker Compose，并补充健康检查、数据卷和容器配置。
- 更新 README 与互动模块文档，说明异步链路、Redis Key 和 RabbitMQ 拓扑。
- 新增 `docs/quickread.md`，提供项目分层、启动链路、业务链路和测试阅读顺序。

## 关键设计

点赞或收藏请求的处理流程如下：

```text
HTTP Handler
  -> Interaction Service
  -> Redis 写入用户状态和实时计数
  -> RabbitMQ 发布 ActionChangedEvent
  -> ActionWorker 消费事件
  -> PostgreSQL 写入互动事实和 video_stat
```

Redis 使用以下键保存互动数据：

```text
interaction:action:v1:{user_id}:{video_id}:{action}
video:stat:counter:v1:{video_id}
video:stat:v1:{video_id}
```

RabbitMQ 使用 `gcfeed.interaction` Exchange、`gcfeed.interaction.action_changed` Queue 和 `interaction.action_changed` Routing Key。Redis 状态写入使用 `WATCH` 事务，重复请求通过幂等键保持稳定结果；消息消费失败时重新入队，消息格式错误则拒绝重试。

## 提交记录

- `5444c7a feat(mq): 增加 RabbitMQ 互动事件基础设施`
- `39c1d29 feat(interaction): 支持点赞收藏异步落库`
- `81f8f81 docs(interaction): 补充异步互动部署说明`
- `ab9e34b docs(readme): 增加代码阅读导览`

## 验证情况

- `go test ./...` 全部通过。
- `git diff --check` 通过。
- `docker compose config --quiet` 通过。
- RabbitMQ 配置默认值和拓扑配置测试通过。

## 后续计划

- 增加 Redis 和 RabbitMQ 集成测试，覆盖并发重复点赞、取消点赞和消息重试。
- 增加异步落库延迟、队列积压和失败消息数量监控。
- 为 RabbitMQ 增加死信队列、消息重放和优雅关闭流程。
- 继续完善曝光、播放和评论计数等事件的批量异步落库能力。
