# Feed 模块设计（MVP）

## 1. 模块职责

负责刷视频主链路，按发布时间分页输出视频，并记录游客或登录用户的观看行为。

## 2. 接口设计

| 方法 | 接口路径 | 作用 | 鉴权 | 幂等键 |
| ---- | -------- | ---- | ---- | ------ |
| GET | `/api/feed/time` | 获取按发布时间排序的视频流 | 可匿名 | - |
| GET | `/api/feed/refresh` | 从时间线起点刷新视频流 | 可匿名 | - |
| POST | `/api/feed/view-events` | 上报曝光、观看或完播事件 | 可匿名，支持 JWT | 支持 |

### 2.1 时间线信息流

`GET /api/feed/time` 按 `published_at DESC, id DESC` 查询已发布视频。`limit` 为可选查询参数，默认值为 20，超过 100 时按 100 返回；`cursor` 为上一页响应中的 `next_cursor`。

响应为 `items`、`next_cursor` 和 `has_more`。有下一页时，服务会生成 `fc.` 前缀的随机游标，保存当前页末尾视频的发布时间和视频 ID；游标在创建 24 小时后失效。未知或已过期游标会返回 `400`。发布前已生成的旧版 Base64 游标仍可继续解析，便于平滑升级。

```json
{
  "items": [
    {
      "video_id": 1001,
      "author_id": 12,
      "title": "first video",
      "media_url": "https://example.com/video.mp4",
      "cover_url": "https://example.com/cover.jpg",
      "like_count": 0,
      "comment_count": 0,
      "favorite_count": 0,
      "published_at": "2026-05-03T12:00:00Z"
    }
  ],
  "next_cursor": "fc.example-token",
  "has_more": true
}
```

### 2.2 刷新信息流

`GET /api/feed/refresh` 接收与时间线接口相同的可选 `limit` 参数，但不接收 `cursor`。它总是从时间线起点查询，并返回与 `GET /api/feed/time` 相同的分页响应和游标语义。

### 2.3 观看事件

`POST /api/feed/view-events` 可匿名调用，也可在 `Authorization` 中携带 JWT。匿名请求必须提供 `visitor_id`；携带有效 JWT 时可省略。提供无效 JWT 时接口返回 `401`。

| 请求字段 | 类型 | 必填 | 说明 |
| -------- | ---- | ---- | ---- |
| `visitor_id` | string | 条件必填 | 匿名请求的游客标识，最大 128 字符 |
| `video_id` | int64 | 是 | 视频 ID，必须大于 0 |
| `event_type` | string | 是 | `IMPRESSION`、`VIEW` 或 `COMPLETE`，不区分大小写 |
| `watch_ms` | int | 否 | 观看时长，单位毫秒，默认 0 |

`Idempotency-Key` 请求头可选，最大 128 字符。首次保存事件返回 `201`；使用同一幂等键重试时返回已保存事件和 `200`。参数无效时返回 `400`。

## 3. 数据表设计

### 3.1 `feed_cursor`

| 字段 | 类型 | 约束 | 说明 |
| ---- | ---- | ---- | ---- |
| `id` | BIGINT | PK | 记录 ID |
| `scene` | VARCHAR(32) | NOT NULL | 信息流场景，当前为 `time` |
| `cursor_token` | VARCHAR(128) | NOT NULL, UNIQUE | 服务端生成的随机游标 |
| `published_at` | DATETIME | NOT NULL | 当前页末尾视频的发布时间 |
| `video_id` | BIGINT | NOT NULL | 当前页末尾视频 ID |
| `expired_at` | DATETIME | NOT NULL | 游标失效时间 |
| `created_at` | DATETIME | NOT NULL | 创建时间 |

索引：`uk_cursor_token(cursor_token)`、`idx_scene_expired(scene, expired_at)`。

### 3.2 `feed_view_event`

| 字段 | 类型 | 约束 | 说明 |
| ---- | ---- | ---- | ---- |
| `id` | BIGINT | PK | 记录 ID |
| `user_id` | BIGINT | NULLABLE | 登录用户 ID |
| `visitor_id` | VARCHAR(128) | NULLABLE | 游客标识 |
| `video_id` | BIGINT | NOT NULL | 视频 ID |
| `event_type` | VARCHAR(16) | NOT NULL | 事件类型 |
| `watch_ms` | INT | NOT NULL, DEFAULT 0 | 观看时长 |
| `idempotency_key` | VARCHAR(128) | NULLABLE, UNIQUE | 幂等键 |
| `created_at` | DATETIME | NOT NULL | 事件时间 |

索引：`idx_user_time(user_id, created_at)`、`idx_visitor_time(visitor_id, created_at)`、`idx_video_time(video_id, created_at)`。
