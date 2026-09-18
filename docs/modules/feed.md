# Feed 模块设计（MVP）

## 1. 模块职责

负责刷视频主链路，输出游标分页结果。当前提供已发布视频的时间线、热榜和关注流读取能力。

## 2. 接口设计（最小实现）

| 方法 | 接口路径 | 作用 | 鉴权 | 幂等键 |
| --- | --- | --- | --- | --- |
| GET | `/api/feed-items` | 获取按 scene 排序的视频流 | timeline/hot 可匿名，following 必须登录 | - |

| POST | `/api/feed/query` | 通过 JSON 请求体查询 scene、cursor、limit、context | timeline/hot 可匿名，following 必须登录 | - |

### 2.1 Feed Items API

`GET /api/feed-items` 支持 `cursor`、`limit` 和 `scene` 查询参数。`limit` 默认 10，最大 100，非正数返回 400；`scene` 默认 `timeline`，可选 `timeline`、`hot`、`following`。

| 参数 | 位置 | 类型 | 必填 | 默认值 | 说明 |
| --- | --- | --- | --- | --- | --- |
| `cursor` | query | string | 否 | - | 上一页返回的游标 |
| `limit` | query | int | 否 | 10 | 返回数量，最大 100 |
| `scene` | query | string | 否 | `timeline` | Feed 场景：`timeline`、`hot`、`following` |

响应包含 `scene`、`items`、`next_cursor` 和 `has_more`。游标使用 URL 安全的 Base64 编码；时间线和热榜无下一页时 `next_cursor` 为空字符串；关注流非空页始终返回游标，使用 `has_more` 判断是否继续，非法游标返回 400。

### 2.2 排序规则

`scene=timeline` 按 `published_at DESC`、`id DESC` 排序。

`scene=hot` 按 `hot_score DESC`、`published_at DESC`、`id DESC` 排序，其中：

```text
hot_score = like_count * 3 + comment_count * 5 + favorite_count * 4
```

### 2.3 游标内容

时间线游标包含 `published_at` 和 `video_id`；热榜游标包含 `hot_score`、`published_at` 和 `video_id`，均取当前页最后一条视频的对应字段。

## 3. 数据表设计（最小实现）

Feed 依赖已有 `videos`、`users` 和 `video_stat` 表：`videos` 保存视频内容、作者 ID、发布状态和发布时间；`users` 提供作者昵称和头像；`video_stat` 提供互动计数。查询只返回已发布且 `published_at` 不为空的视频，关注流还使用 `user_follow` 和 `user_relation_stat`；历史 `feed_inbox` 表保留，但不再参与发布与读取。

## 4. Timeline 访问优化

### 4.1 数据库索引

时间线查询只读取已发布视频，并按发布时间和视频 ID 倒序排列。项目通过 GORM 在 `videos` 表上维护复合索引 `idx_video_timeline`，对应以下查询路径：

```sql
CREATE INDEX idx_video_timeline
ON videos (status, published_at DESC, id DESC);
```

其中 `status` 用于过滤发布状态，`published_at` 和 `id` 与排序及游标分页条件保持一致；`id` 作为同一发布时间下的稳定排序字段。

### 4.2 Redis 读缓存

路由装配时，配置了 Redis 地址便为 `scene=timeline` 启用读缓存。页缓存保存视频 ID 和排序字段，卡片与计数分别缓存和组装；未命中或读取异常时回源 PostgreSQL。查询成功后尝试写入缓存，写入失败仍返回已取得的结果，不会再次查询数据库；数据库查询失败仍返回错误。

| 缓存页 | 有效期 |
| --- | --- |
| 首页（无游标） | 5 秒，并增加按缓存键计算的 10% 至 20% 抖动 |
| 后续页（带游标） | 45 秒，并增加按缓存键计算的 10% 至 20% 抖动 |

缓存键包含页大小，后续页使用游标的 SHA-1 摘要，避免把完整游标直接放入键名：

```text
feed:timeline:v1:limit:{limit}:first
feed:timeline:v1:limit:{limit}:cursor:{sha1(cursor)}
```

同一进程内，同一缓存键的并发回源请求通过 `singleflight` 合并为一次查询。共享查询使用独立的 5 秒超时；单个 HTTP 请求取消时只结束自身等待，不会取消其他请求或共享回源查询。

## 5. 关注流推拉混合

`GET /api/feed-items?scene=following` 和 `POST /api/feed/query` 的 `scene=following` 均使用当前访问令牌的用户身份。请求体不接受用于覆盖身份的用户编号。未提供令牌或令牌无效时返回 `401 {"error":"viewer is required"}`，非法 limit、cursor 或未知场景返回 400，数据库读取失败返回 500。

- 发布事务只保存 `videos` 和 `video_stat`，提交成功后发送 `PublishedEvent`；幂等重放不再发消息。事件携带事件编号、视频和作者编号、标题、描述、媒体地址、封面地址、发布时间与发生时间。
- 普通作者：粉丝数小于 `BigCreatorFollowerThreshold`（10,000），消费者按每批 500 名有效粉丝，将视频写入 Redis `feed:following:inbox:v1:{userId}`，最多保留 1000 条。
- 大作者：粉丝数达到阈值时只写 `feed:following:author:v1:{authorId}`，最多保留 500 条，查询时与用户 inbox 合并。
- 索引使用有序集合去重，事务管道连续执行添加、裁剪和 30 天有效期刷新。重复投递可安全重试，既有互动缓存的 WATCH 并发保护保持不变。
- 消费者先预热卡片（15 分钟）和统计（15 秒）。任一读取、写入或粉丝批次失败，消息重新入队；无效 JSON 拒绝且不重新入队，成功后确认。
- 读取时查询全部有效关注作者，过滤取关条目，并按完整发布时间和视频 ID 倒序去重分页；避免参考实现仅以大作者列表过滤导致普通作者丢失。索引不足一页、缺失、不可用或命中已删除条目时回源 PostgreSQL。
- 命中索引仍批量核验视频公开状态及关注关系，避免缓存卡片展示已删除视频；不使用跨用户共享的分页缓存。最后一页非空时仍有 `next_cursor`，以 `has_more=false` 停止翻页。

行为边界：消息发布与数据库提交不是同一事务，发送失败不回滚视频，未加入发布确认、持久化事件发件箱或自动重连。索引有容量和有效期限制；完整的一页索引可能暂时缺少消费中的新视频或新关注作者历史，当前实现没有全量完整性水位或历史回填。数据库回退会返回有效关注作者的全部公开视频。

### 部署与验证

开发配置在 `configs/config.yaml`，容器配置在 `configs/config.docker.yaml`。新增 `rabbitmq.video_exchange`、`video_published_queue`、`video_published_routing`，默认分别为 `videofeed.video`、`videofeed.video.published` 和 `video.published`。支持环境变量 `RABBITMQ_VIDEO_EXCHANGE`、`RABBITMQ_VIDEO_PUBLISHED_QUEUE`、`RABBITMQ_VIDEO_PUBLISHED_ROUTING`。在容器部署中覆盖环境变量时，需要将其传入 API 容器。

沿用根目录 `docker-compose.yaml` 的 PostgreSQL、Redis、RabbitMQ、数据卷、健康检查和依赖顺序，无新服务或 Go 依赖。RabbitMQ 不可用时启动记录错误并禁用发布器，Feed 仍能回源数据库；消费者运行后连接故障不自动重连，需要恢复服务连接。消息服务账号必须与部署配置一致。

在仓库根目录运行 `go test ./...`、`go vet ./...` 和 `docker compose config --quiet`。使用 `docker compose up -d --build` 启动后，API 端口为 9000，Web 端口为 5173，宿主 PostgreSQL 端口为 5433。完整映射、适配差异和验证结果见 [异步迁移审计](../migration-9834644.md)。

## 6. 前端关注流入口

打开 `/following` 或点击侧栏“关注流”进入。前端 `web/src/App.jsx` 在没有访问令牌时直接显示“登录后查看关注流”，不请求 Feed 接口；有令牌时调用 `GET /api/feed-items?scene=following&limit=10`。关注流请求返回 401 时同样显示登录提示，其他错误保留重新加载操作。登录按钮进入 `/auth`，登录成功仍按现有流程进入时间线。

桌面导航按“最新视频、关注流、热门榜单”排列；900px 及以下宽度使用三等分底栏，样式位于 `web/src/style.css`。现有 Vite 和 Nginx 的单页回退支持直接访问 `/following`，不新增服务端路由。页面切换或令牌变化时，旧 Feed 请求的返回结果不会再更新状态。

前端构建命令为 `cd web` 后运行 `npm run build`；开发服务默认端口为 5173。迁移映射和验证结果见 [前端入口迁移审计](../migration-7d58cd0.md)。

真实账号、数据库和浏览器联调结果及修复说明见 [联调报告](../following-live-integration.md)。
