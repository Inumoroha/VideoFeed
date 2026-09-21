# 消息模块

状态：后端已实现。前端消息中心和审核事件通知不在此次迁移范围。

## 接口和实现位置

| 方法 | 路径 | 鉴权 | 行为 |
| --- | --- | --- | --- |
| GET | `/api/messages` | Bearer JWT | 按创建时间、ID 倒序分页，默认 20，最多 100 |
| GET | `/api/message-stats/unread` | Bearer JWT | 当前用户未读数量 |
| PATCH | `/api/messages` | Bearer JWT | `message_ids` 去重；省略、null 或空数组表示全部已读 |
| POST | `/internal/messages` | `X-Internal-Token` | 首次创建 201；事件或请求键重放返回原消息和 200 |

实现位于 `internal/domain/message`、`internal/application/message`、`internal/infrastructure/persistence/message`、`internal/handler/message`。路由和通知适配器见 `internal/router/message.go`。字段、状态码和示例以 `api/openapi.yaml` 为准。

## 数据库和幂等

PostgreSQL 表 `user_message` 保存 `id`、`user_id`、`type`、`title`、`content`、可空 `event_id`、可空 `idempotency_key`、布尔 `is_read`、`created_at` 和可空 `read_at`。类型为 LIKE、COMMENT、FOLLOW、SYSTEM；接收用户 ID 仅校验正数，不查询用户存在性。

`(user_id,event_id)` 与 `(user_id,idempotency_key)` 分别具有唯一约束。空键保存为 NULL，允许无键请求重复新增；冲突事务先回滚，再按同一用户查询既有消息。两个键分别命中不同消息时，沿用原实现返回 ID 较大的消息。索引名称使用 `message` 前缀，避免 PostgreSQL schema 内与其他表索引重名；补齐 `(user_id,created_at)` 列表索引。

创建时去除首尾空白，标题/内容分别非空且最多 128/1024 个 UTF-8 字节；事件标识最多 64 字节，幂等键最多 128 字节。类型忽略大小写。已读更新限定当前用户且只更新未读记录，忽略他人或不存在的 ID，重复执行不会改写已读时间。

## 通知触发与失败处理

- 点赞：有效点赞变化量大于零时通知视频作者，事件键 `interaction:like:<videoID>:<userID>`。
- 评论：真正新增评论时通知作者，事件键 `interaction:comment:<commentID>`。
- 关注：有效关注且关注流回填成功后通知被关注用户，事件键 `relation:follow:<targetUserID>:<userID>`。
- 自赞、自评不通知；收藏、取消和删除不通知。取消后重新点赞或关注仍复用事件键，同一收件用户最终只有一条消息。
- 查询作者失败、通知写入失败不会改变主业务成功结果。原提交没有消息重试队列、补偿任务或可靠投递保证，本迁移保留该限制。关注回填失败仍返回既有错误；请求重试并成功回填后可以写入通知。

互动扩展接口 `NotificationRepository` 返回事务内变化量，旧 `SetAction` 和 `CreateComment` 签名及结果保持兼容。生产 GORM 仓储实现扩展接口；仅实现旧接口的自定义仓储可继续工作，但不提供通知所需的作者和变化量。

Redis WATCH/事务、RabbitMQ 拓扑、手动确认/拒绝/失败重新入队及关闭流程沿用现有实现。消息通知在 API 请求中写数据库，不新增消息专用消费者。异步点赞发布失败时仍同步回退，通知仅在回退保存成功后触发。

## 配置和运行

本地 API 地址 `http://127.0.0.1:9000`。`configs/config.yaml`、`configs/config.docker.yaml` 中 `internal.token` 默认为空，内部创建接口默认拒绝访问；用户查询及业务触发通知不依赖此令牌。通过 `INTERNAL_TOKEN` 环境变量配置，Compose 自动传给 API；Kubernetes 示例使用 `videofeed-secret` 的 `internal-token` 字段。其余内部推荐接口保留原认证行为。

## 验证

原提交的四个测试入口全部保留：`TestMessageAPIFlow`、`TestMessageAPIValidation`、`TestInteractionMessageWriter`、`TestRelationMessageWriter`。另有错误映射、异步失败回退、回填失败重试、令牌配置、SQL 回滚与查询隔离、长度边界测试。

`internal/infrastructure/persistence/message/live_test.go` 在独立 schema 验证真实 PostgreSQL 并发唯一约束、用户隔离、空键、分页与已读幂等，需设置专用测试连接后执行：

```powershell
$env:VIDEOFEED_TEST_POSTGRES_URL = '<专用 PostgreSQL 测试连接>'
go test ./internal/infrastructure/persistence/message -run TestMessagePostgresLive -v
```

未设置时明确跳过，不代表已经验证真实数据库。完整迁移映射和验证记录见 [迁移审计](../migration-7a62b5c.md)。
