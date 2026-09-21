# 视频模块设计

## 1. 模块职责

负责视频发布、详情读取、列表查询和删除（软删除）。

上传入口由 `internal/handler/upload` 提供，独立 worker 处理发布后的关注分发与文本向量。

## 2. 接口设计

| 方法   | 接口路径                     | 作用                   | 鉴权       | 幂等键 |
| ------ | ---------------------------- | ---------------------- | ---------- | ------ |
| POST   | `/api/videos`                | 创建并发布视频         | Bearer JWT | 支持   |
| GET    | `/api/videos/{videoId}`      | 查询视频详情           | 可匿名     | -      |
| DELETE | `/api/videos/{videoId}`      | 删除视频（软删除）     | Bearer JWT | -      |
| GET    | `/api/users/{userId}/videos` | 根据用户ID查询作品列表 | 可匿名     | -      |
| GET    | `/api/users/me/videos`       | 查询当前用户作品列表   | Bearer JWT | -      |

### 创建视频字段

`POST /api/videos` 的请求体字段如下。`description` 为可选字段，服务端会去除首尾空白，并限制为最多 512 个字符。

| 字段          | 类型   | 必填 | 说明                      |
| ------------- | ------ | ---- | ------------------------- |
| `title`       | string | 是   | 视频标题，最多 128 个字符 |
| `description` | string | 否   | 视频描述，最多 512 个字符 |
| `media_url`   | string | 是   | 视频地址                  |
| `cover_url`   | string | 是   | 封面地址                  |

视频详情、作者作品列表和当前用户作品列表响应均包含 `description` 字段。描述为空时返回空字符串。

`POST /api/uploads` 使用 Bearer JWT、multipart 文件上传，不支持 `Idempotency-Key`。视频删除本身可重复执行，但不读取幂等键。

## 3. 数据表设计

### `videos`

`videos` 保存视频主体信息和生命周期状态。

| 字段              | 类型         | 约束                 | 说明                    |
| ----------------- | ------------ | -------------------- | ----------------------- |
| `id`              | BIGINT       | PK                   | 视频ID                  |
| `author_id`       | BIGINT       | NOT NULL             | 作者ID                  |
| `title`           | VARCHAR(128) | NOT NULL             | 标题（最多128个字符）   |
| `description`     | VARCHAR(512) | NOT NULL, DEFAULT '' | 描述（最多512个字符）   |
| `media_url`       | VARCHAR(512) | NOT NULL             | 视频地址                |
| `cover_url`       | VARCHAR(512) | NOT NULL             | 封面地址                |
| `status`          | INTEGER      | NOT NULL, DEFAULT 2  | 1草稿/2上线/3下架/4删除 |
| `published_at`    | TIMESTAMPTZ  | NULLABLE             | 发布时间                |
| `idempotency_key` | VARCHAR(128) | NULLABLE             | 创建请求幂等键          |
| `created_at`      | TIMESTAMPTZ  | NOT NULL             | 创建时间                |
| `updated_at`      | TIMESTAMPTZ  | NOT NULL             | 更新时间                |

索引：`idx_author_status(author_id, status, created_at)`、`idx_video_timeline(status, published_at DESC, id DESC)`；`(author_id, idempotency_key)` 为唯一索引。

### `video_stat`

`video_stat` 保存视频高频统计数据，供视频详情和 Feed 列表读取。

| 字段             | 类型     | 约束                | 说明                   |
| ---------------- | -------- | ------------------- | ---------------------- |
| `video_id`       | BIGINT   | PK                  | 视频ID，对应 `videos.id` |
| `like_count`     | INT      | NOT NULL, DEFAULT 0 | 点赞数                 |
| `comment_count`  | INT      | NOT NULL, DEFAULT 0 | 评论数                 |
| `favorite_count` | INT      | NOT NULL, DEFAULT 0 | 收藏数                 |
| `created_at`     | TIMESTAMPTZ | NOT NULL          | 创建时间               |
| `updated_at`     | TIMESTAMPTZ | NOT NULL          | 更新时间               |

索引建议：`pk_video_id(video_id)`。

写入规则：

1. 创建视频时同步创建 `video_stat`，三个计数字段初始值为 0。
2. 点赞、收藏、评论写入时，由互动模块在同一事务内更新 `video_stat`。
3. 视频详情、作品列表和 Feed 列表读取时关联 `video_stat` 返回计数。

## 4. 业务规则

创建即发布（status=2），在事务内同时创建视频和零计数统计行。作者可软删除（status=4）；公开视频详情和公开作品只返回上线视频。发布幂等键按作者隔离，重复请求返回已有视频；幂等重放不重复发送发布事件。发布提交后消息失败不回滚视频。标题与简介按 Unicode 字符校验 128/512，幂等键按 UTF-8 字节校验 128。

关注分发与向量消费者部署见 [部署说明](../deployment.md)，向量覆盖与重试边界见 [文本向量模块](embedding.md)。

上传对视频扩展名、内容类型、大小、时长、分辨率和编码执行校验；MP4/MOV 成功上传前执行 faststart 将播放元数据前置，具体限制见下方“上传校验与快启”。

## 5. 测试建议

测试位于 `internal/application/video/service_test.go`、`internal/domain/video/video_test.go`、`internal/infrastructure/persistence/video/repository_test.go`、`internal/router/router_test.go`、`internal/handler/upload/handler_test.go`。覆盖发布及统计创建、未登录 401、可见视频详情、缺失 404、作者删除、非作者权限错误、公开作品过滤、发布幂等、上传错误；消息失败和重复投递见同包 fanout 测试。

上传媒体测试位于 `internal/router/upload_api_test.go` 和 `internal/handler/upload/{metadata,validation,processor_live}_test.go`：非法媒体返回 400，已经落盘的失败文件被清理；合法 MP4/MOV 校验元数据后执行 faststart。真实媒体测试需要 ffmpeg/ffprobe，缺少工具时明确跳过。

## 6. 前端接入点

发布页上传文件并填写标题和简介；Feed 与视频详情展示视频、作者和计数；我的作品及公开主页查询对应作者列表。实现入口为 `web/src/App.jsx`。

## 上传校验与快启

`POST /api/uploads` 继续使用 JWT 和 multipart 字段 file/kind。完整请求体上限 1 GiB（含表单开销），超限返回 413。各类文件为空或超过类别上限时返回 400：video 最多 512 MiB，cover/avatar 最多 20 MiB，普通 file 最多 1 GiB。

- 视频扩展名允许 `.mp4`、`.mov`、`.webm`；探测文件前 512 字节，允许 MP4/QuickTime/WebM 或通用二进制类型，随后由 ffprobe 校验真实视频元数据。
- 封面和头像扩展名允许 `.jpg`、`.jpeg`、`.png`、`.webp`，探测类型允许 JPEG/PNG/WebP；仅校验文件头，不做完整图片解码，也不要求后缀与内容格式一一匹配。
- 视频时长须为有限正数且不超过 600 秒，每个视频流宽高须在 1–3840；支持 h264/h265/hevc/vp8/vp9/av1，音频非空编码须为 aac/mp3/opus/vorbis。至少包含一个视频流。
- MP4/MOV 经 ffmpeg 流复制和 `+faststart` 将元数据前移，不重编码；临时文件在原目录创建，后缀为 `.faststart.mp4`；输出容器仍由显式参数决定（MP4 使用 `-f mp4`，MOV 使用 `-f mov`），WebM 跳过快启。ffprobe 超时 30 秒，ffmpeg 超时 2 分钟，均继承请求取消。
- 校验失败返回 400；工具缺失、快启或落盘失败返回 500。处理失败删除已上传文件，快启临时文件和 multipart 临时文件都会清理；没有自动重试或降级接受未验证视频。
- 普通 file 保留原有异常扩展名转 `.bin` 规则，不做媒体内容校验。成功响应 size 是原始上传大小。此次收紧视频/图片和空文件规则属于客户端可观察的兼容性变化。

接口以 `api/openapi.yaml` 为准；迁移清单与真实媒体验证见 [上传迁移审计](../migration-34f6682.md)。
