# 互动模块设计（MVP）

## 1. 模块职责

互动模块负责用户对视频的点赞、收藏和评论能力，并同步维护视频统计表。

模块边界：

| 模块          | 职责                                                         |
| ------------- | ------------------------------------------------------------ |
| `interaction` | 记录点赞、收藏、评论事实，处理显式状态设置、评论发布和评论删除 |
| `video`       | 保存视频主体信息                                             |
| `video_stat`  | 保存`like_count`、`favorite_count`、`comment_count` 统计字段 |
| `account`     | 提供登录用户身份，评论响应展示用户昵称和头像                 |

## 2. 目录建议

```text
internal/domain/interaction/
internal/application/interaction/
internal/infrastructure/persistence/interaction/
internal/handler/interaction/
```

## 3. 接口设计

| 方法   | 接口路径                                 | 作用          | 鉴权       | 幂等键 |
| ------ | ---------------------------------------- | ------------- | ---------- | ------ |
| PUT    | `/api/videos/{videoId}/like`             | 设置点赞      | Bearer JWT | 支持   |
| DELETE | `/api/videos/{videoId}/like`             | 取消点赞      | Bearer JWT | 支持   |
| PUT    | `/api/videos/{videoId}/favorite`         | 设置收藏      | Bearer JWT | 支持   |
| DELETE | `/api/videos/{videoId}/favorite`         | 取消收藏      | Bearer JWT | 支持   |
| POST   | `/api/videos/{videoId}/comments`         | 发表评论      | Bearer JWT | 支持   |
| GET    | `/api/videos/{videoId}/comments`         | 获取评论列表  | 可匿名     | -      |
| DELETE | `/api/comments/{commentId}`               | 删除评论      | Bearer JWT | 操作本身幂等 |

### 3.1 设置或取消点赞

#### PUT `/api/videos/{videoId}/like`

请求头：

| 参数              | 必填 | 说明                    |
| ----------------- | ---- | ----------------------- |
| `Authorization`   | 是   | `Bearer <access_token>` |
| `Idempotency-Key` | 否   | 客户端幂等键，最长 128  |

请求体为空；`videoId` 是正整数路径参数。`PUT` 始终将点赞设置为激活状态。

响应：

```json
{
    "video_id": 1001,
    "action_type": "LIKE",
    "active": true,
    "like_count": 18,
    "favorite_count": 7
}
```

#### DELETE `/api/videos/{videoId}/like`

请求头与 `PUT` 相同，请求体为空。`DELETE` 始终将点赞设置为未激活状态，响应结构相同，其中 `active` 为 `false`。

### 3.2 设置或取消收藏

#### PUT `/api/videos/{videoId}/favorite`

请求头：

| 参数              | 必填 | 说明                    |
| ----------------- | ---- | ----------------------- |
| `Authorization`   | 是   | `Bearer <access_token>` |
| `Idempotency-Key` | 否   | 客户端幂等键，最长 128  |

请求体为空；`videoId` 是正整数路径参数。`PUT` 始终将收藏设置为激活状态。

响应：

```json
{
    "video_id": 1001,
    "action_type": "FAVORITE",
    "active": true,
    "like_count": 18,
    "favorite_count": 7
}
```

#### DELETE `/api/videos/{videoId}/favorite`

请求头与 `PUT` 相同，请求体为空。`DELETE` 始终将收藏设置为未激活状态，响应结构相同，其中 `active` 为 `false`。

### 3.3 发表评论

#### POST `/api/videos/{videoId}/comments`

请求头：

| 参数              | 必填 | 说明                    |
| ----------------- | ---- | ----------------------- |
| `Authorization`   | 是   | `Bearer <access_token>` |
| `Idempotency-Key` | 否   | 客户端幂等键，最长 128  |

请求体：

```json
{
    "content": "这个剪辑节奏很好"
}
```

`videoId` 是正整数路径参数。

响应：

```json
{
    "id": 3001,
    "video_id": 1001,
    "user_id": 12,
    "user_nickname": "tester",
    "user_avatar_url": "https://example.com/avatar.png",
    "content": "这个剪辑节奏很好",
    "created_at": "2026-05-04T12:00:00Z",
    "comment_count": 4
}
```

### 3.4 评论列表

#### GET `/api/videos/{videoId}/comments`

请求参数：

| 参数       | 位置  | 类型   | 必填 | 默认值 | 说明               |
| ---------- | ----- | ------ | ---- | ------ | ------------------ |
| `cursor`   | query | string | 否   | -      | 上一页返回的游标   |
| `limit`    | query | int    | 否   | 20     | 返回数量，最大 100 |

`videoId` 是正整数路径参数。

响应：

```json
{
    "items": [
        {
            "id": 3001,
            "video_id": 1001,
            "user_id": 12,
            "user_nickname": "tester",
            "user_avatar_url": "https://example.com/avatar.png",
            "content": "这个剪辑节奏很好",
            "created_at": "2026-05-04T12:00:00Z"
        }
    ],
    "next_cursor": "eyJjcmVhdGVkX2F0IjoiMjAyNi0wNS0wNFQxMjowMDowMFoiLCJjb21tZW50X2lkIjozMDAxfQ",
    "has_more": true
}
```

排序规则：

| 排序字段     | 方向 | 说明                       |
| ------------ | ---- | -------------------------- |
| `created_at` | DESC | 新评论靠前                 |
| `id`         | DESC | 同一创建时间下按评论ID倒序 |

游标内容：

| 字段         | 说明                         |
| ------------ | ---------------------------- |
| `created_at` | 当前页最后一条评论的创建时间 |
| `comment_id` | 当前页最后一条评论ID         |

### 3.5 删除评论

#### DELETE `/api/comments/{commentId}`

请求头：

| 参数              | 必填 | 说明                    |
| ----------------- | ---- | ----------------------- |
| `Authorization`   | 是   | `Bearer <access_token>` |

响应：

```json
{
    "comment_id": 3001,
    "status": 2,
    "comment_count": 3
}
```

权限规则：

| 用户身份 | 权限                   |
| -------- | ---------------------- |
| 评论作者 | 可删除自己的评论       |
| 视频作者 | 可删除自己视频下的评论 |
| 管理员角色（`admin`） | 可删除任意评论         |

## 4. 数据表设计

### 4.1 `interaction_action`

`interaction_action` 保存点赞和收藏，这两类行为共享 `user_id + video_id + action_type` 的唯一性约束。

| 字段              | 类型         | 约束                | 说明                |
| ----------------- | ------------ | ------------------- | ------------------- |
| `id`              | BIGINT       | PK                  | 记录ID              |
| `user_id`         | BIGINT       | NOT NULL            | 用户ID              |
| `video_id`        | BIGINT       | NOT NULL            | 视频ID              |
| `action_type`     | VARCHAR(16)  | NOT NULL            | `LIKE` / `FAVORITE` |
| `status`          | BIGINT       | NOT NULL, DEFAULT 1 | 1有效/2取消         |
| `idempotency_key` | VARCHAR(128) | NULLABLE            | 最近一次写入幂等键  |
| `created_at`      | TIMESTAMPTZ  | NOT NULL            | 创建时间            |
| `updated_at`      | TIMESTAMPTZ  | NOT NULL            | 更新时间            |

索引建议：

| 索引                    | 字段                             | 说明                                         |
| ----------------------- | -------------------------------- | -------------------------------------------- |
| `uk_user_video_type`    | `user_id, video_id, action_type` | 保证同一用户对同一视频的同类行为只有一条记录 |
| `idx_video_type_status` | `video_id, action_type, status`  | 支持按视频统计有效行为                       |
| `idx_user_type_status`  | `user_id, action_type, status`   | 支持后续我的点赞、我的收藏列表               |

### 4.2 `interaction_action_idempotency`

`interaction_action_idempotency` 保存带 `Idempotency-Key` 的点赞和收藏首次执行结果。相同用户、动作类型和幂等键的重试直接返回该记录，不会再次执行状态写入。

| 字段              | 类型         | 约束                | 说明                         |
| ----------------- | ------------ | ------------------- | ---------------------------- |
| `id`              | BIGINT       | PK                  | 记录ID                       |
| `user_id`         | BIGINT       | NOT NULL            | 用户ID                       |
| `action_type`     | VARCHAR(16)  | NOT NULL            | `LIKE` / `FAVORITE`          |
| `idempotency_key` | VARCHAR(128) | NOT NULL            | 请求幂等键                   |
| `video_id`        | BIGINT       | NOT NULL            | 首次请求的视频ID             |
| `active`          | BOOLEAN      | NOT NULL            | 首次请求完成后的动作状态     |
| `like_count`      | BIGINT       | NOT NULL            | 首次请求完成后的点赞统计     |
| `favorite_count`  | BIGINT       | NOT NULL            | 首次请求完成后的收藏统计     |
| `created_at`      | TIMESTAMPTZ  | NOT NULL            | 首次请求完成时间             |

索引建议：

| 索引                         | 字段                                      | 说明                                   |
| ---------------------------- | ----------------------------------------- | -------------------------------------- |
| `uk_user_action_idempotency` | `user_id, action_type, idempotency_key`   | 保证每个点赞或收藏请求仅执行一次       |

### 4.3 `interaction_comment`

`interaction_comment` 保存评论内容，删除采用状态更新。

| 字段              | 类型          | 约束                | 说明           |
| ----------------- | ------------- | ------------------- | -------------- |
| `id`              | BIGINT        | PK                  | 评论ID         |
| `video_id`        | BIGINT        | NOT NULL            | 视频ID         |
| `user_id`         | BIGINT        | NOT NULL            | 评论用户       |
| `content`         | VARCHAR(1000) | NOT NULL            | 评论内容       |
| `status`          | BIGINT        | NOT NULL, DEFAULT 1 | 1正常/2删除    |
| `idempotency_key` | VARCHAR(128)  | NULLABLE            | 创建评论幂等键 |
| `created_at`      | TIMESTAMPTZ   | NOT NULL            | 创建时间       |
| `updated_at`      | TIMESTAMPTZ   | NOT NULL            | 更新时间       |

索引建议：

| 索引                       | 字段                               | 说明                 |
| -------------------------- | ---------------------------------- | -------------------- |
| `idx_video_status_created` | `video_id, status, created_at, id` | 支持评论列表游标分页 |
| `idx_user_created`         | `user_id, created_at`              | 支持用户评论历史     |
| `uk_user_idempotency`      | `user_id, idempotency_key`         | 支持评论创建幂等     |

## 5. 状态枚举

### 5.1 Action Type

| 值         | 说明 |
| ---------- | ---- |
| `LIKE`     | 点赞 |
| `FAVORITE` | 收藏 |

### 5.2 Action Status

| 值  | 说明 |
| --- | ---- |
| `1` | 有效 |
| `2` | 取消 |

### 5.3 Comment Status

| 值  | 说明 |
| --- | ---- |
| `1` | 正常 |
| `2` | 删除 |

## 6. 核心业务规则

### 6.1 点赞和收藏的显式状态设置

处理流程：

1. 校验登录用户、路径参数 `videoId` 和目标状态。
2. 查询视频，视频状态为 `Published` 时允许互动。
3. 按 `user_id + video_id + action_type` 查询行为记录。
4. `PUT` 将目标状态设置为有效；`DELETE` 将目标状态设置为取消。
5. 记录缺失时创建目标状态对应的记录；只有目标状态为有效时才增加对应计数。
6. 记录存在时仅在状态实际变化时更新记录和计数；重复 `PUT` 或重复 `DELETE` 不改变计数。
7. 命中相同幂等键时直接返回首次执行的动作状态和统计快照。
8. 在同一事务内提交行为记录、`video_stat` 计数更新和首次幂等结果。

计数字段映射：

| `action_type` | 统计字段                    |
| ------------- | --------------------------- |
| `LIKE`        | `video_stat.like_count`     |
| `FAVORITE`    | `video_stat.favorite_count` |

计数约束：

| 操作     | 计数变化            |
| -------- | ------------------- |
| 生效     | `count + 1`         |
| 取消     | `max(count - 1, 0)` |
| 幂等命中 | 返回首次执行的动作状态和两项统计快照 |

### 6.2 评论创建

处理流程：

1. 校验登录用户、`video_id` 和 `content`。
2. 查询视频，视频状态为 `Published` 时允许评论。
3. 创建 `interaction_comment`，状态为 `1`。
4. 在同一事务内将 `video_stat.comment_count` 加 1。
5. 返回评论详情和最新评论数。

内容约束：

| 字段      | 规则                           |
| --------- | ------------------------------ |
| `content` | 去除首尾空白后长度为 1 到 1000 |

### 6.3 评论删除

处理流程：

1. 校验登录用户和 `commentId`。
2. 查询评论和所属视频。
3. 校验删除权限。
4. 评论状态为 `1` 时更新为 `2`，并将 `video_stat.comment_count` 更新为 `max(comment_count - 1, 0)`。
5. 评论状态为 `2` 时直接返回当前结果。

## 7. 幂等设计

点赞、收藏和评论创建接口读取请求头 `Idempotency-Key`。点赞和收藏使用相同用户、动作类型和幂等键返回首次动作状态及统计快照；评论创建使用相同用户和幂等键返回已创建的评论及当前评论数。删除评论不读取该请求头，但重复删除会返回当前删除结果且不会重复扣减计数。

互动动作幂等键的唯一范围不包含视频 ID 或 HTTP 方法，因此客户端必须为每次状态设置生成新键，不能将同一键复用于不同视频、点赞与取消点赞，或收藏与取消收藏请求。

MVP 实现建议：

| 场景             | 处理方式                                                           |
| ---------------- | ------------------------------------------------------------------ |
| 点赞/收藏状态设置 | 使用`uk_user_action_idempotency(user_id, action_type, idempotency_key)` 返回首次执行的状态和统计快照 |
| 评论创建         | 使用`uk_user_idempotency(user_id, idempotency_key)` 返回已创建评论 |
| 评论删除         | 删除操作本身幂等，重复删除返回当前删除结果                         |

## 8. 错误码建议

| HTTP 状态 | 场景                                                             | 响应                                    |
| --------- | ---------------------------------------------------------------- | --------------------------------------- |
| 400       | `videoId`、`commentId`、`limit`、`cursor` 或 `content` 校验失败 | `{"error":"video id must be positive"}` 等具体校验错误 |
| 401       | 登录态缺失或 Token 失效                                          | `{"error":"invalid access token"}`      |
| 403       | 删除评论权限校验失败                                             | `{"error":"comment permission denied"}` |
| 404       | 视频或评论记录缺失                                               | `{"error":"resource not found"}`        |
| 500       | 服务内部错误                                                     | `{"error":"internal server error"}`     |

## 9. 测试用例

| 用例                     | 预期                                     |
| ------------------------ | ---------------------------------------- |
| 登录用户首次 `PUT` 点赞公开视频 | 返回`active=true`，`like_count + 1`      |
| 登录用户重复 `PUT` 同一视频 | 返回`active=true`，`like_count` 不变     |
| 登录用户 `DELETE` 已点赞视频 | 返回`active=false`，`like_count - 1`     |
| 登录用户首次 `PUT` 收藏公开视频 | 返回`active=true`，`favorite_count + 1`  |
| 登录用户 `DELETE` 已收藏视频 | 返回`active=false`，`favorite_count - 1` |
| 使用相同幂等键重试点赞或收藏 | 返回首次请求的动作状态和两项统计快照，不重复执行状态写入 |
| 登录用户发表评论         | 返回评论详情，`comment_count + 1`        |
| 匿名用户查询评论列表     | 返回状态为正常的评论，按创建时间倒序     |
| 评论作者删除评论         | 返回`status=2`，`comment_count - 1`      |
| 视频作者删除视频下评论   | 返回`status=2`，`comment_count - 1`      |
| 普通用户删除他人评论     | 返回 403                                 |
| 重复删除同一评论         | 返回`status=2`，评论计数保持稳定         |

## 10. 前端接入点

| 页面                | 接入能力                                            |
| ------------------- | --------------------------------------------------- |
| Feed 视频右侧操作栏 | 根据当前状态调用点赞和收藏的 `PUT` 或 `DELETE`，并使用响应计数刷新按钮文案 |
| Feed 评论抽屉       | 按当前 `videoId` 拉取评论列表                        |
| 评论输入框          | 登录用户提交评论，成功后插入列表顶部                |
| 未登录用户互动      | 引导到登录页                                        |
