# Feed 模块设计（MVP）

## 1. 模块职责

负责刷视频主链路，输出游标分页结果并记录观看行为。当前最小实现提供已上线视频的时间线和热榜读取能力。

## 2. 接口设计（最小实现）

| 方法 | 接口路径 | 作用 | 鉴权 | 幂等键 |
| --- | --- | --- | --- | --- |
| GET | `/api/feed-items` | 获取按 scene 排序的视频流 | 可匿名 | - |

### 2.1 Feed Items API

`GET /api/feed-items` 支持 `cursor`、`limit` 和 `scene` 查询参数。`limit` 默认 10，最大 100，非正数返回 400；`scene` 默认 `timeline`，可选 `timeline`、`hot`。

| 参数 | 位置 | 类型 | 必填 | 默认值 | 说明 |
| --- | --- | --- | --- | --- | --- |
| `cursor` | query | string | 否 | - | 上一页返回的游标 |
| `limit` | query | int | 否 | 10 | 返回数量，最大 100 |
| `scene` | query | string | 否 | `timeline` | Feed 场景：`timeline`、`hot` |

响应包含 `scene`、`items`、`next_cursor` 和 `has_more`。游标使用 URL 安全的 Base64 编码；无下一页时 `next_cursor` 为空字符串，非法游标返回 400。

### 2.2 排序规则

`scene=timeline` 按 `published_at DESC`、`id DESC` 排序。

`scene=hot` 按 `hot_score DESC`、`published_at DESC`、`id DESC` 排序，其中：

```text
hot_score = like_count * 3 + comment_count * 5 + favorite_count * 4
```

### 2.3 游标内容

时间线游标包含 `published_at` 和 `video_id`；热榜游标包含 `hot_score`、`published_at` 和 `video_id`，均取当前页最后一条视频的对应字段。

## 3. 数据表设计（最小实现）

Feed 依赖已有 `videos`、`users` 和 `video_stat` 表：`videos` 保存视频内容、作者 ID、发布状态和发布时间；`users` 提供作者昵称和头像；`video_stat` 提供互动计数。查询只返回已发布且 `published_at` 不为空的视频，不额外维护 Feed 专用数据表。

## 4. Timeline 访问优化

### 4.1 数据库索引

时间线查询只读取已发布视频，并按发布时间和视频 ID 倒序排列。项目通过 GORM 在 `videos` 表上维护复合索引 `idx_video_timeline`，对应以下查询路径：

```sql
CREATE INDEX idx_video_timeline
ON videos (status, published_at DESC, id DESC);
```

其中 `status` 用于过滤发布状态，`published_at` 和 `id` 与排序及游标分页条件保持一致；`id` 作为同一发布时间下的稳定排序字段。

### 4.2 Redis 读缓存

路由装配时，配置了 Redis 地址便为 `scene=timeline` 启用读缓存。缓存保存完整的时间线分页结果，缓存命中时直接返回；未命中或读取异常时回源 PostgreSQL。查询成功后尝试写入缓存，写入失败仍返回已取得的结果，不会再次查询数据库；数据库查询失败仍返回错误。

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
