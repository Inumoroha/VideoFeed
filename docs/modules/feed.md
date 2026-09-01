# Feed 模块设计（MVP）

## 1. 模块职责

负责刷视频主链路，按发布时间和视频 ID 倒序分页输出已发布视频。

## 2. 接口设计

| 方法 | 接口路径             | 作用                       | 鉴权   |
| ---- | -------------------- | -------------------------- | ------ |
| GET  | `/api/feed/timeline` | 获取按发布时间排序的视频流 | 可匿名 |
| GET  | `/api/feed/refresh`  | 从时间线起点刷新视频流     | 可匿名 |

### 2.1 时间线信息流

`GET /api/feed/timeline` 查询状态为已发布且已设置发布时间的视频，排序为 `published_at DESC, id DESC`。

`limit` 为可选查询参数，省略时默认返回 20 条，最大返回 100 条；传入非正数时返回 `400`。`cursor` 为上一页响应中的 `next_cursor`，无游标时从时间线起点查询。

游标是 URL 安全的 Base64 编码字符串，载荷包含当前页末尾视频的发布时间和视频 ID。服务不会持久化游标，也不设置过期时间；无法解析或字段无效的游标返回 `400`。有下一页时返回 `next_cursor`，没有下一页时为空字符串。

响应为 `items`、`next_cursor` 和 `has_more`：

```json
{
    "items": [
        {
            "video_id": 1001,
            "author_id": 12,
            "author_nickname": "作者昵称",
            "author_avatar_url": "https://example.com/avatar.png",
            "title": "first video",
            "description": "first video description",
            "media_url": "https://example.com/video.mp4",
            "cover_url": "https://example.com/cover.jpg",
            "like_count": 0,
            "comment_count": 0,
            "favorite_count": 0,
            "published_at": "2026-05-03T12:00:00Z"
        }
    ],
    "next_cursor": "eyJwdWJsaXNoZWRfYXQiOiIyMDI2LTA1LTAzVDEyOjAwOjAwWiIsInZpZGVvX2lkIjoxMDAxfQ",
    "has_more": true
}
```

### 2.2 刷新信息流

`GET /api/feed/refresh` 接收与时间线接口相同的 `limit` 参数，不接收 `cursor`，总是从时间线起点查询，并返回相同的分页响应结构。

## 3. 数据来源

信息流仓储读取 `videos` 表，并通过 `author_id` 关联 `users` 表获取作者昵称和头像。仅返回 `status` 为已发布且 `published_at` 不为空的视频，不额外维护信息流专用数据表。
