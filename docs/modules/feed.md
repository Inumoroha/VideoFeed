# Feed 模块设计

## 1. 模块职责

负责刷视频主链路，输出游标分页结果。当前提供已发布视频的时间线、热榜、关注流和推荐流读取能力。

## 2. 接口设计

| 方法 | 接口路径 | 作用 | 鉴权 | 幂等键 |
| --- | --- | --- | --- | --- |
| GET | `/api/feed-items` | 获取按 scene 排序的视频流 | timeline/hot 可匿名，following/recommend 必须登录 | - |
| POST | `/api/feed/query` | 通过 JSON 请求体查询 scene、cursor、limit、context | timeline/hot 可匿名，following/recommend 必须登录 | - |
| POST | `/api/video-view-events` | 上报曝光和观看事件 | 必须登录 | 无；request_id 不去重 |

### Feed Items API

`GET /api/feed-items` 支持 `cursor`、`limit` 和 `scene` 查询参数。`limit` 默认 10，最大 100，非正数返回 400；`scene` 默认 `timeline`，可选 `timeline`、`hot`、`following`、`recommend`。

| 参数 | 位置 | 类型 | 必填 | 默认值 | 说明 |
| --- | --- | --- | --- | --- | --- |
| `cursor` | query | string | 否 | - | 上一页返回的游标 |
| `limit` | query | int | 否 | 10 | 返回数量，最大 100 |
| `scene` | query | string | 否 | `timeline` | Feed 场景：`timeline`、`hot`、`following`、`recommend` |

响应包含 `scene`、`items`、`next_cursor` 和 `has_more`。游标使用 URL 安全的 Base64 编码；时间线无下一页时 `next_cursor` 为空字符串；Redis 热榜非空页仍有游标，应以 `has_more` 停止；关注流非空页始终返回游标，使用 `has_more` 判断是否继续，timeline/hot/following 非法游标返回 400；推荐场景见业务规则。

### 排序规则

`scene=timeline` 按 `published_at DESC`、`id DESC` 排序。

Redis 热榜按一小时窗口分数和成员排序；未配置缓存时 `scene=hot` 按 `hot_score DESC`、`published_at DESC`、`id DESC` 排序，其中：

```text
hot_score = like_count * 3 + comment_count * 5 + favorite_count * 4
```

### 游标内容

时间线游标包含 `published_at` 和 `video_id`；Redis 热榜游标为 `window_end`、`offset`；数据库热榜游标包含 `hot_score`、`published_at` 和 `video_id`，均取当前页最后一条视频的对应字段。

## 3. 数据表设计

Feed 依赖已有 `videos`、`users` 和 `video_stat` 表：`videos` 保存视频内容、作者 ID、发布状态和发布时间；`users` 提供作者昵称和头像；`video_stat` 提供互动计数。查询只返回已发布且 `published_at` 不为空的视频，关注流还使用 `user_follow` 和 `user_relation_stat`；历史 `feed_inbox` 表保留，但不再参与发布与读取。

曝光流水 `video_view_events` 和聚合 `exposures` 的字段、唯一约束及事务语义见下文“曝光与观看上报”。

## 4. 业务规则

### Timeline 访问优化

#### 数据库索引

时间线查询只读取已发布视频，并按发布时间和视频 ID 倒序排列。项目通过 GORM 在 `videos` 表上维护复合索引 `idx_video_timeline`，对应以下查询路径：

```sql
CREATE INDEX idx_video_timeline
ON videos (status, published_at DESC, id DESC);
```

其中 `status` 用于过滤发布状态，`published_at` 和 `id` 与排序及游标分页条件保持一致；`id` 作为同一发布时间下的稳定排序字段。

#### Redis 读缓存

路由装配时，配置了 Redis 地址便为 `scene=timeline` 启用读缓存。页缓存保存视频 ID 和排序字段，卡片与计数分别缓存和组装；未命中或读取异常时回源 PostgreSQL。查询成功后尝试写入缓存，写入失败仍返回已取得的结果，不会再次查询数据库；数据库查询失败仍返回错误。

| 缓存页 | 有效期 |
| --- | --- |
| 首页（无游标） | 5 秒，并增加按缓存键计算的 10% 至 20% 抖动 |
| 后续页（带游标） | 45 秒，并增加按缓存键计算的 10% 至 20% 抖动 |

缓存键包含页大小，后续页使用游标的 SHA-1 摘要，避免把完整游标直接放入键名：

```text
feed:page:v1:timeline:limit:{limit}:first
feed:page:v1:timeline:limit:{limit}:cursor:{sha1(cursor)}
```

同一进程内，同一缓存键的并发回源请求通过 `singleflight` 合并为一次查询。共享查询使用独立的 5 秒超时；单个 HTTP 请求取消时只结束自身等待，不会取消其他请求或共享回源查询。

### 关注流推拉混合

`GET /api/feed-items?scene=following` 和 `POST /api/feed/query` 的 `scene=following` 均使用当前访问令牌的用户身份。请求体不接受用于覆盖身份的用户编号。未提供令牌或令牌无效时返回 `401 {"error":"viewer is required"}`，非法 limit、cursor 或未知场景返回 400，数据库读取失败返回 500。

- 发布事务只保存 `videos` 和 `video_stat`，提交成功后发送 `PublishedEvent`；幂等重放不再发消息。事件携带事件编号、视频和作者编号、标题、描述、媒体地址、封面地址、发布时间与发生时间。
- 普通作者：粉丝数小于 `BigCreatorFollowerThreshold`（10,000），消费者按每批 500 名有效粉丝，将视频写入 Redis `feed:following:inbox:v1:{userId}`，最多保留 1000 条。
- 大作者：粉丝数达到阈值时只写 `feed:following:author:v1:{authorId}`，最多保留 500 条，查询时与用户 inbox 合并。
- 索引使用有序集合去重，事务管道连续执行添加、裁剪和 30 天有效期刷新。重复投递可安全重试，既有互动缓存的 WATCH 并发保护保持不变。
- 消费者先预热卡片（15 分钟）和统计（15 秒）。任一读取、写入或粉丝批次失败，消息重新入队；无效 JSON 拒绝且不重新入队，成功后确认。
- 读取时查询全部有效关注作者，过滤取关条目，并按完整发布时间和视频 ID 倒序去重分页；避免参考实现仅以大作者列表过滤导致普通作者丢失。索引不足一页、缺失、不可用或命中已删除条目时回源 PostgreSQL。
- 命中索引仍批量核验视频公开状态及关注关系，避免缓存卡片展示已删除视频；不使用跨用户共享的分页缓存。最后一页非空时仍有 `next_cursor`，以 `has_more=false` 停止翻页。

行为边界：消息发布与数据库提交不是同一事务，发送失败不回滚视频，未加入发布确认、持久化事件发件箱或自动重连。索引有容量和有效期限制；完整的一页索引可能暂时缺少消费中的新视频或新关注作者历史，当前实现没有全量完整性水位；新关注小作者已有同步近期回填，见下文。数据库回退会返回有效关注作者的全部公开视频。

#### 部署与验证

开发配置在 `configs/config.yaml`，容器配置在 `configs/config.docker.yaml`。新增 `rabbitmq.video_exchange`、`video_published_queue`、`video_published_routing`，默认分别为 `videofeed.video`、`videofeed.video.published` 和 `video.published`。支持环境变量 `RABBITMQ_VIDEO_EXCHANGE`、`RABBITMQ_VIDEO_PUBLISHED_QUEUE`、`RABBITMQ_VIDEO_PUBLISHED_ROUTING`。在容器部署中覆盖环境变量时，需要将其同时传入 API 和 worker 容器。

沿用根目录 `docker-compose.yaml` 的 PostgreSQL、Redis、RabbitMQ、数据卷、健康检查和依赖顺序，相关功能未新增 Go 依赖；消费者已由独立 worker 执行。RabbitMQ 不可用时启动记录错误并禁用发布器，Feed 仍能回源数据库；消费者运行后连接故障不自动重连，需要恢复服务连接。消息服务账号必须与部署配置一致。

在仓库根目录运行 `go test ./...`、`go vet ./...` 和 `docker compose config --quiet`。使用 `docker compose up -d --build` 启动后，API 端口为 9000，Web 端口为 5173，宿主 PostgreSQL 端口为 5433。完整映射、适配差异和验证结果见 [异步迁移审计](../migration-9834644.md)。

### 曝光与观看上报

`POST /api/video-view-events` 使用现有 Bearer JWT，从认证上下文取得用户 ID。请求示例：

```json
{"video_id":1001,"scene":"timeline","request_id":"req-1","event_type":"exposed","watch_ms":0,"completed":false}
```

`video_id` 必须为正数且视频状态为已发布；`scene` 去空白、转小写后非空且最多 32 个 UTF-8 字节（不限定 Feed 查询场景）；`request_id` 去空白后最多 64 字节，可省略。`event_type` 去空白、转小写后支持 exposed、play、complete、skip；`watch_ms` 非负，缺省为 0；`completed` 缺省为 false，complete 强制为 true。

成功返回 201，响应包含 `event`（id、user_id、video_id、scene、event_type、watch_ms、completed、created_at 及非空 request_id）；exposed 还返回 `exposure`（user_id、video_id、first_exposed_at、last_exposed_at、exposure_count、last_scene）。参数或 JSON 错误返回 400，未认证返回 401，视频不存在或未发布返回 404，存储失败返回 500，错误结构统一为 `{"error":"..."}`。完整结构及示例见 `api/openapi.yaml`。

#### 数据表与并发

| 表 | 字段与约束 |
| --- | --- |
| `video_view_events` | id 主键；user_id、video_id、scene、可空 request_id、event_type、watch_ms、completed、created_at；保留用户时间、视频时间、场景时间及请求事件索引 |
| `exposures` | id 主键；user_id + video_id 唯一；first_exposed_at、last_exposed_at、exposure_count、last_scene、created_at、updated_at；保留用户/视频最近曝光索引 |

路由初始化通过 GORM 迁移两张表。仓储在同一 PostgreSQL 事务中校验视频、追加流水、更新聚合并读取结果；任一步失败回滚。聚合使用 `ON CONFLICT (user_id, video_id) DO UPDATE` 和数据库原子加一，首次时间不变。索引名称带表名前缀，避免与 `feed_inbox` 的历史索引重名。`request_id` 只做关联，不提供幂等性：重试相同请求仍追加流水，重复 exposed 仍累加。play、complete、skip 不创建或更新曝光聚合。

#### 画像消息与部署

落库成功后构造 `ViewEventRecordedEvent`，包含 event_id、view_event_id、user_id、video_id、scene、可选 request_id、event_type、watch_ms、completed、UTC recorded_at 和 occurred_at，以及可选 exposure_count。随机事件 ID 生成失败时沿用纳秒时间戳回退。

RabbitMQ 使用持久化 topic 交换机、持久化队列和持久化 JSON 消息，消息属性保留事件编号和发送时间。消费接口逐条确认成功消息，非法 JSON 拒绝且不重入队，处理失败拒绝并重入队；消费取消或连接关闭会结束消费。未加入发布确认、发件箱、自动重连或发布重试；发布失败不回滚已提交行为，初始化失败时记录日志并禁用发布器。此次仅提供画像消费接口，没有启动画像计算 worker，推荐查询现通过 PostgreSQL 过滤近七天曝光，详见推荐模块。

| YAML 配置项（rabbitmq 下） | 环境变量 | 默认值 |
| --- | --- | --- |
| exposure_exchange | RABBITMQ_EXPOSURE_EXCHANGE | videofeed.exposure |
| view_event_recorded_queue | RABBITMQ_VIEW_EVENT_RECORDED_QUEUE | videofeed.exposure.view_event_recorded |
| view_event_recorded_routing | RABBITMQ_VIEW_EVENT_RECORDED_ROUTING | exposure.view_event_recorded |

两个配置文件 `configs/config.yaml`、`configs/config.docker.yaml` 均包含新增键。根目录 Compose 沿用 PostgreSQL、Redis、RabbitMQ、现有数据卷、健康检查和启动依赖，无新增服务或依赖。覆盖容器配置需将上述环境变量同时传入 API 和 worker 容器；RabbitMQ 账号必须与实际服务一致。API 9000、Web 5173、宿主 PostgreSQL 5433。启动命令仍为 `docker compose up -d --build`。

在仓库根目录运行 `go test ./...`、`go vet ./...`、`git diff --check` 和 `docker compose config --quiet`。可选真实服务测试使用 `VIDEOFEED_TEST_POSTGRES_URL` 与 `VIDEOFEED_TEST_AMQP_URL` 指向专用测试服务；测试使用独立数据库 schema 或消息拓扑并清理，未配置时跳过。验证结果与限制见 [迁移审计](../migration-fa476283.md)。

真实服务验证可运行 `python scripts/verify_exposure_live.py`：脚本自动启动并清理独立 Docker 服务，测试数据库并发与消息消费后，再验证实际 API 的注册登录、曝光上报和故障回退。已执行结果见 [曝光真实服务验证](../exposure-live-verification.md)。

### 推荐场景

`scene=recommend` 由 `internal/application/feed/recommendation.go` 处理，保留现有 `/api/feed/query` 路径。候选按推荐服务顺序组装，只复用公共卡片和计数缓存，缓存失败回源；不缓存个性化页。`context.request_id` 传给推荐服务。未登录返回 401；推荐领域非法游标或过长请求标识沿用源提交的 Feed 映射返回 500，内部候选接口对此返回 400。详见 [推荐模块](recommendation.md)。

### 新关注作品回填

启用 Redis 时，关注保存成功后对粉丝数不足 10000 的作者同步查询最近最多 100 条已发布且有发布时间的作品，按发布时间、视频编号倒序写入当前用户收件箱，容量为 1000，有效期 30 天。大作者不回填；取关不回填。两个现有关注入口 `PUT /api/relations/{targetUserId}`、`PUT /api/users/me/following/{targetUserId}` 都使用此逻辑。

回填依赖通过 `internal/router/follow_backfill.go` 组合，数据库查询位于 `internal/infrastructure/persistence/feed/backfill.go`，应用逻辑位于 `internal/application/relation/backfill.go`。每条 Redis 写入沿用事务管道中的去重、裁剪和续期，跨作品不属于一个事务。任一步失败返回统一 500，已提交关注关系和此前缓存写入不会回滚；客户端可用原幂等键显式重试，关系计数沿用数据库幂等逻辑，缓存相同成员重复写入去重。没有自动重试或新增 MQ 消息。

当前关注流传入全部有效关注作者并校验索引可见性，因此小作者收件箱不会被大作者名单误过滤，取关和删除视频仍受既有校验保护。索引不足、缺失或故障仍回源 PostgreSQL。回填无需新环境变量、数据表、Compose 服务、数据卷或连接；复用现有数据库与 Redis 配置。验证命令为 `go test ./...` 和配置 GCC 后的 `go test -race ./...`。

### Hot 窗口缓存

Redis 按分钟记录热度，最近 60 桶汇总；点赞/收藏/评论权重为 3/4/5，取消或删除写对应负分。分钟桶 `feed:hot:minute:v1:{yyyyMMddHHmm}` 有效 2 小时，窗口 `feed:hot:window:v1:{windowEndUnix}` 有效 2 分钟。未配置缓存时查询数据库累计热度；已配置 Redis 时窗口读取失败返回 500；两种游标不应混用。

## 5. 测试建议

`internal/application/feed` 覆盖场景、分页、批量卡片、缓存故障、共享回源取消；`internal/infrastructure/cache/feed` 覆盖 Redis 并发、分片与关注索引；`internal/router`、`internal/handler/feed` 覆盖 HTTP 鉴权与错误；`internal/infrastructure/persistence/exposure` 和 MQ 测试覆盖曝光并发及失败重试。真实环境验证命令见业务规则和 [部署说明](../deployment.md)。

## 6. 前端接入点

打开 `/following` 或点击侧栏“关注流”进入。前端 `web/src/App.jsx` 在没有访问令牌时直接显示“登录后查看关注流”，不请求 Feed 接口；有令牌时调用 `GET /api/feed-items?scene=following&limit=10`。关注流请求返回 401 时同样显示登录提示，其他错误保留重新加载操作。登录按钮进入 `/auth`，登录成功进入推荐流。

桌面导航按“推荐流、最新视频、关注流、热门榜单”排列；900px 及以下宽度使用四等分底栏，样式位于 `web/src/style.css`。现有 Vite 和 Nginx 的单页回退支持直接访问 `/following`，不新增服务端路由。页面切换或令牌变化时，旧 Feed 请求的返回结果不会再更新状态。

前端构建命令为 `cd web` 后运行 `npm run build`；开发服务默认端口为 5173。迁移映射和验证结果见 [前端入口迁移审计](../migration-7d58cd0.md)。

真实账号、数据库和浏览器联调结果及修复说明见 [联调报告](../following-live-integration.md)。

推荐分页、单次曝光、旧响应隔离与 401 处理见 [推荐模块](recommendation.md)。
