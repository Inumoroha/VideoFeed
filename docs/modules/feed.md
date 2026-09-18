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

Feed 依赖已有 `videos`、`account` 和 `video_stat` 表：`videos` 保存视频内容、作者 ID、发布状态和发布时间；`account` 提供作者昵称和头像；`video_stat` 提供互动计数。查询只返回已发布且 `published_at` 不为空的视频，关注流还使用 `feed_inbox`、`user_follow` 和 `user_relation_stat` 表。

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

- 普通作者：粉丝数小于 `BigCreatorFollowerThreshold`（10,000），发布视频时在同一数据库事务内创建视频、初始统计，并向当时有效关注者的 `feed_inbox` 写入条目。
- 大作者：粉丝数达到阈值时跳过扇出，查询关注流时根据有效关注关系和当前粉丝数拉取视频。
- 两部分通过 SQL `UNION` 去重，仅保留当前仍有效关注、已发布且发布时间非空的视频，按 `published_at DESC, video_id DESC` 分页。同一时间戳以视频 ID 保证稳定顺序。
- 收件箱保存 ID、用户 ID、视频 ID、作者 ID、发布时间和创建时间。`(user_id, video_id)` 联合唯一索引配合 PostgreSQL `ON CONFLICT ... DO UPDATE` 原子更新重复条目的发布时间；写入失败会回滚整个发布事务，后续可使用原幂等键重试。
- 关注页每次读取数据库，不复用公共分页缓存；仅共用卡片和计数缓存，缓存读取失败回源，回填失败不影响成功响应。

保持参考提交的边界：普通作者不会向新关注用户补发历史视频，作者跨阈值时没有历史收件箱回填。取关后旧收件箱条目保留，但查询时排除。最后一页非空时仍有 `next_cursor`，应以 `has_more=false` 停止翻页。

### 部署与验证

启动时 `internal/router/register.go` 通过 AutoMigrate 创建 `feed_inbox` 表及索引，无新增配置、环境变量、服务或依赖。开发配置在 `configs/config.yaml`，容器配置在 `configs/config.docker.yaml`；沿用根目录 `docker-compose.yaml` 的 PostgreSQL 数据卷和健康依赖。

在仓库根目录运行 `go test ./...`、`go vet ./...` 和 `docker compose config --quiet`。使用 `docker compose up -d --build` 启动后，API 端口为 9000，Web 端口为 5173，宿主 PostgreSQL 端口为 5433。迁移明细和验证边界见 [迁移审计](../migration-1517e5a.md)。
