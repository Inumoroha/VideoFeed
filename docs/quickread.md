# 如何快速读懂 VideoFeed

先看 [产品范围](product.md) 区分现有能力和规划，再沿一次请求理解代码；工程约定见 [工程规范](engineering.md)。所有命令在仓库根目录执行。

## 1. 项目主线

| 主线 | 实现模块 |
| --- | --- |
| 用户身份与关系 | user、relation、JWT |
| 内容供给 | video、upload、embedding |
| 分发 | feed 的 timeline、hot、following、recommend；recommendation |
| 互动与行为 | interaction、exposure、Redis、RabbitMQ |

## 2. 目录和启动

后端 Go 模块位于根目录，前端在 `web`，不是嵌套的 apps 工作区。

1. `cmd/server/main.go`：API 启动。
2. `internal/config/loader.go`、`internal/database/database.go`：配置与 PostgreSQL 连接。
3. `internal/router/register.go`：迁移、依赖装配及路由。
4. `internal/infrastructure/persistence/migration/migration.go`：API/worker 共用迁移和咨询锁。
5. `cmd/worker/main.go`：独立消费互动、关注分发和向量队列。

装配顺序为配置→数据库/Redis/RabbitMQ/JWT→仓储→服务→处理器→路由。读取单个模块时从 `internal/handler` 进入 `internal/application`、`internal/domain`，再到 `internal/infrastructure/persistence`；鉴权在 `internal/middleware` 和 `internal/auth/jwt`。

## 3. 注册与发布

`POST /api/users`、`POST /api/sessions` 对应用户 handler/service、`internal/domain/user/user.go` 和 `internal/infrastructure/persistence/user/repository.go`。

`POST /api/videos` 对应视频 handler/service、`internal/domain/video/video.go` 与视频仓储。事务保存 `videos` 与 `video_stat`，幂等重放返回已有视频。提交后发布事件，worker 分别生成关注索引和 `video_embedding` 向量；消息失败不回滚视频。

## 4. Feed 与推荐

入口为 `GET /api/feed-items` 和 `POST /api/feed/query`，从 `internal/handler/feed/handler.go` 追到 `internal/application/feed/service.go`。时间线采用发布时间和 ID 游标；页缓存只存排序信息，卡片与计数批量 MGET，缺失时回源 PostgreSQL。缓存实现为 `internal/infrastructure/cache/feed/feed.go`。

Hot 使用 60 个分钟桶；未配置缓存时用数据库累计热度，两种游标不同。Following 需要 JWT，合并用户 inbox 与大作者 outbox，核验关注关系和公开视频；实现见 `internal/infrastructure/cache/feed/following.go` 与 Feed 仓储。

Recommend 从 `internal/application/feed/recommendation.go` 进入推荐服务，使用观看画像、向量、热度和新鲜度排序，过滤七天内曝光。三个内部推荐接口当前不使用 JWT。详细边界见 [Feed](modules/feed.md) 和 [推荐](modules/recommendation.md)。

## 5. 点赞、评论、曝光与回填

点赞/收藏依次经过互动 handler、service、Redis WATCH 事务、RabbitMQ 和 `internal/application/interaction/worker.go`。Redis 状态键为 `interaction:action:v1:{user_id}:{video_id}:{action}`，计数采用 16 分片。Redis 写入失败返回错误；MQ 发布失败同步写仓储。评论与计数在数据库事务同步更新。

`POST /api/video-view-events` 同步写曝光流水与聚合，再尽力投递画像事件；重复 request_id 不去重。已有画像消费接口不代表已启动画像 worker。

关注后的同步回填入口为 `internal/router/follow_backfill.go`，业务在 `internal/application/relation/backfill.go`。中途失败返回 500，但关系已提交；原幂等键显式重试不会重复计数。

## 6. 用测试理解边界

| 位置 | 重点 |
| --- | --- |
| `internal/router`、`internal/handler` | 路由、鉴权、参数、错误响应 |
| `internal/application`、`internal/domain` | 用例、幂等、游标、失败回退 |
| `internal/infrastructure/persistence` | PostgreSQL 事务与唯一约束 |
| `internal/infrastructure/cache/feed` | WATCH、计数分片、索引并发 |
| `internal/infrastructure/mq`、`cmd/worker` | 消费确认、拒绝、重试、资源关闭 |
| `web/tests` | 推荐页分页、曝光、身份切换和旧请求隔离 |

```bash
go test ./...
go vet ./...
npm --prefix web test
npm --prefix web run build
```

真实服务测试需专用测试连接；默认跳过并不代表真实 PostgreSQL/RabbitMQ 已验证。竞态与其他验证命令见工程规范。

## 7. 启动后验证

```bash
docker compose up -d --build
curl http://127.0.0.1:9000/health
docker compose logs -f api worker web
```

Web 地址为 `http://127.0.0.1:5173`。按注册→登录→上传/发布→浏览→互动→关注→推荐和曝光验证；观察 worker 队列及 PostgreSQL 结果。配置、依赖顺序、数据卷和关闭限制见 [部署说明](deployment.md)。

## 8. 深入阅读

[系统架构](architecture.md)、[模块索引](modules/README.md)、[性能优化](optimization.md)、[UI/UX 规格](uiux.md)、[迁移与真实联调记录](migrations.md)。下一步关注可靠事件投递、治理和观测；已有曝光同步落库与画像消息，不要重复当作待实现功能。
