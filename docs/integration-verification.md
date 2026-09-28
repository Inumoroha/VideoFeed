# 真实服务联调记录

本次联调于 2026-09-28 在 Windows 本机执行，目标是让本仓库当前代码（含最近四次迁移成果）真实跑起来，用真实 PostgreSQL、Redis、RabbitMQ 验证鉴权、上传、发布、Feed、互动、消息、指标与 Worker 异步落库。

## 1. 环境与前置条件

| 项 | 值 |
| --- | --- |
| 编排 | Docker 29.6.2（Docker Desktop，本次启动） |
| 容器 | `postgres:17-alpine`、`redis:7.4-alpine`、`rabbitmq:3.13-management-alpine`，均为 health |
| 应用进程 | 宿主机直接运行 `bin/videofeed-api.exe`、`bin/videofeed-worker.exe` |
| API | `http://127.0.0.1:9000` |
| Worker 指标 | `http://127.0.0.1:9091/metrics` |
| 数据库 | `postgres://postgres:video_feed_dev@localhost:5433/video_feed` |
| 脚本 | `scripts/integration-smoke.ps1`（32 项断言，纯 ASCII，Windows PowerShell 5.1 兼容） |

Docker Hub 在本网络不可达（`Get "https://registry-1.docker.io/v2/": EOF`），因此：

- `prom/prometheus:v3.1.0` 与 `grafana/grafana:11.4.0` **无法拉取**，本次没有启动这两个服务。
- `golang:1.26.1-alpine` 与 `alpine:3.22` 也无法拉取，因此**没有构建 Compose 镜像**，改为用宿主机进程运行 API 与 worker（同一份源码、同一份迁移逻辑）。

## 2. 执行的检查与结果

### 2.1 静态与编排校验

| 检查 | 结果 |
| --- | --- |
| `docker compose config --quiet` | **通过**（此前的联调记录中该项因守护进程未运行而缺失，本次补齐） |
| `docker compose config --services` | 8 个服务：`postgres`、`redis`、`rabbitmq`、`api`、`worker`、`web`、`prometheus`、`grafana` |
| 基础设施健康 | 三个容器均 `running (healthy)`；`pg_isready` 接受连接，`redis-cli ping` 返回 PONG，`rabbitmq-diagnostics -q ping` 成功 |

### 2.2 端到端断言（32 项，31 通过 / 1 失败）

通过的 31 项，按主题分组：

**鉴权**

- `register` 201；`login returns access_token` 200 且返回令牌
- `users/me` 200 且账号匹配；`users/me rejects anonymous` 401

**上传与校验**

- `upload rejects bad extension` 400（`.exe` 被拒）
- `upload cover png` 201，返回 `/uploads/cover/<timestamp>-<rand>.png`
- `upload requires auth` 401

**发布与幂等**

- `create video` 201 且返回 ID
- `create video idempotent replay` 相同 `Idempotency-Key` 返回同一 ID

**Feed 与查看者状态**

- `timeline feed contains new video` 200
- `feed item exposes liked/favorited`：匿名/未互动时为 `false`
- `anonymous timeline allowed` 200
- `recommend requires login` 401；`recommend works for logged in viewer` 200
- `feed reflects like+favorite`：点赞收藏后 `liked=true`、`favorited=true`

**互动与评论**

- `like video` 200 `active=true` `like_count=1`
- `favorite video` 200 `active=true`
- `create comment` 201 `comment_count=1`；`list comments` 200 且条数 ≥1

**内部消息与内部令牌**

- `internal message carries actor fields` 201，且 `actor_id=77`、`actor_nickname`、`actor_avatar_url` 原样返回（本次迁移新增字段）
- `message idempotent replay keeps actor`：重放返回同一 ID 且触发者字段保留
- `internal message rejects missing token` 401；`internal message rejects wrong token` 401
- `internal recommendation reachable` 200

**指标**

- `api /metrics exposed` 200
- `metrics use videofeed namespace`：存在 `videofeed_http_requests_total`
- `metrics have no gcfeed leakage`：无 `gcfeed_` 前缀
- `metrics record /api/feed-items route`：`videofeed_http_requests_total{method="GET",route="/api/feed-items",status="200"}` 有值
- `worker /metrics exposed on 9091` 200

**Worker 与健康检查**

- `interaction counts readable after worker consumed events`：`like_count`、`favorite_count`、`comment_count` 可从 Feed 读出
- `health endpoint` 200 `{"status":"healthy"}`

数据库侧写入可核对：`users=6`、`videos=6`、`interaction_action=6`、`interaction_comment=1`、`user_message=2`。

### 2.3 失败项

`unlike clears feed state` 失败。反馈复现见下节。

## 3. 联调发现的缺陷（非本次迁移引入）

### 现象

用户点赞成功后再取消点赞，`DELETE /api/videos/{id}/like` 返回 200，但：

- 响应为 `{"action_type":"LIKE","active":true,"like_count":2}` —— 取消点赞后 `active` 仍为 `true`，计数从 1 **增加**到 2
- 数据库 `interaction_action.status` 仍为 `1`（有效）
- `video_stat.like_count` 为 2
- Feed 中 `liked` 仍为 `true`

复现（`vid=6`，用户 6）：

```text
PUT    /api/videos/6/like  -> {"active":true,"like_count":1}
DELETE /api/videos/6/like  -> {"active":true,"like_count":2}
DB row: LIKE|1     DB stat like_count=2
```

加上幂等键同样复现：`like-3` 与 `unlike-3` 两条幂等记录都写成 `active=t`。

### 根因

`internal/infrastructure/persistence/interaction/repository.go` 第 149 到 154 行，签名中的 `active` 参数被忽略，改成了「同状态取反」的开关语义：

```go
if action.Status == nextStatus {
    action.Status = domaininteraction.ActionStatusCanceled
    delta = -1
} else {
    action.Status = domaininteraction.ActionStatusActive
}
```

以「当前 status=1（有效）、请求 unlike（`active=false`）」为例：

- `nextStatus = actionStatusFromActive(false) = 2`
- `action.Status(1) == nextStatus(2)` 为假，进入 `else`
- 于是 `action.Status = 1`、`delta` 保持 1 → 状态不变且计数 **+1**

正确语义应为 `action.Status = nextStatus`；仅当传入状态与当前状态相同时，`delta` 才取 0。当前写法把「设置状态」实现成了「切换状态」，因此第二次及以后对同一用户同一视频的同类操作都会得到相反结果。

### 归属

`git blame` 显示该分支由提交 `630f1fa`（2026-09-11）引入，早于本次会话的全部迁移提交。本次会话在这两个文件中的改动只有 `GetUserProfile`、`mapUserError` 与 `messageProfileModel`，不涉及状态判定逻辑。因此这是**仓库既有缺陷**，不是迁移回归。

### 影响面

- 所有「取消点赞 / 取消收藏」请求在目标状态与当前状态不同时反而写入有效状态并增加计数。
- 影响 `PUT/DELETE /api/videos/{id}/like` 与 `/favorite` 两条链路，以及依赖计数正确性的热榜分与 Feed 展示。
- `internal/application/interaction` 与 `internal/handler/interaction` 的测试使用的是内存替身，未覆盖该 SQL 分支，所以 `go test ./...` 全绿也无法发现。

### 建议修复（未实施）

1. 在 `repository.go` 的 else 分支改为 `action.Status = nextStatus`，并令 `delta` 为：当前状态已等于目标状态时为 0，否则为 `+1`（`nextStatus` 有效）或 `-1`（`nextStatus` 取消）。
2. 同步修正 Redis 侧 `SetActionState`（`internal/infrastructure/cache/feed/feed.go`）：其空幂等键分支 `old["idempotency_key"] == strings.TrimSpace(idempotencyKey) && idempotencyKey != ""` 在两侧都为空时仍会命中并复用旧状态，缺少幂等键时同样会吞掉状态变更。
3. 补一个使用真实 SQL 或 sqlmock 的仓储测试，覆盖「先点赞后取消」与「重复同类请求」两种序列。

以上属于新的缺陷修复，不在本次 8cf995c 迁移范围内，因此本次**没有修改**，仅记录。

## 4. 未完成或受限的内容

- **Prometheus 与 Grafana 未启动**：镜像在本网络不可拉取。`docker-compose.yaml`、`monitoring/prometheus.yml`、Grafana provisioning 与看板只完成客户端校验（YAML/JSON 解析与 `docker compose config` 通过），未验证实际抓取与出图。
- **未构建 Compose 镜像**：`golang:1.26.1-alpine` 与 `alpine:3.22` 不可拉取，改为宿主机进程运行，因此未验证镜像构建、容器内 `ffmpeg/ffprobe` 与容器网络下的服务发现。
- **Web 未联调**：`vite` 开发服务与 Nginx 生产镜像未启动；前端仅执行过 `npm --prefix web test` 与构建。
- **未压测**：环境无 k6，`scripts/feed-load.js` 未执行。
- **未测视频真实转码**：上传用例使用 PNG 走封面分支，未覆盖 ffprobe 视频时长/分辨率/编码校验与 faststart 处理（这些由既有 `TestVideoProcessorLive` 之类的条件式测试覆盖，本次未运行）。
- 联调使用的是本地 `video_feed` 库，测试期间写入了 6 个用户、6 个视频等数据，未在结束后清理。

## 5. 结论

本仓库当前代码在真实 PostgreSQL、Redis、RabbitMQ 上可以正常启动并完成注册、登录、上传校验、发布幂等、Feed 读取、查看者互动状态、点赞收藏、评论、内部消息（含本次迁移新增的触发者字段与令牌鉴权）、指标暴露与 Worker 异步落库的主链路，`/metrics` 与 worker `9091` 指标服务均按 `videofeed_` 命名空间正常工作。

同时联调暴露了一个仓库既有的点赞取消缺陷，会让取消操作反向增加计数，其根因、复现与修复建议已记录在本文件第 3 节，等待单独处理。

## 6. 2026-09-28 后续前后端联调

以上第 1 至 5 节保留首次联调的现场结论。本次后续联调使用当前工作区源码启动宿主机 API、worker、Vite，以及真实 PostgreSQL、Redis、RabbitMQ。此前记录的点赞取消问题已有工作区修复；浏览器实测点赞计数增加、取消后恢复，32 项后端冒烟断言全部通过。

浏览器完成了注册登录、推荐流与最新流加载、点赞和取消、个人资料、消息空状态、评论读取与发布。新上传封面经 Vite 代理加载成功。评论写入 PostgreSQL 后，发现 Redis 的短期 JSON 统计过期会回退到旧的评论数基数：评论列表已有 1 条，但 Feed 显示 0。`SetVideoStat` 现以事务管道更新评论基数、保留点赞收藏分片并清除短期快照。浏览器再发布评论后，API 与页面均显示 2 条；超过 15 秒并刷新，计数仍为 2。

上传页原先并行上传视频和封面。宿主机缺少 `ffmpeg/ffprobe` 时，视频返回 `video tool is unavailable`，封面却已存储。现改为先上传视频，再上传封面；前端测试验证视频失败时不会请求封面，成功时按视频、封面、发布顺序执行。

本次检查：`go test ./...`、`go vet ./...`、前端 20 项测试、前端构建、`git diff --check`、`docker compose config --quiet`、OpenAPI YAML 解析均通过。真实视频处理仍需在具备 `ffmpeg/ffprobe` 的环境验证；封面上传或发布步骤失败后，已成功上传的视频仍可能成为未引用文件。此前留下的 Redis 评论基数仅在后续评论变更或键过期后更新。联调写入的本地测试数据和上传文件未清理。

## 7. 视频上传与失败回收验证

安装 Gyan.FFmpeg 9.0.2 后，将 WinGet Links 目录加入宿主机 API 进程的 PATH，并用当前工作区源码重新启动 9000 端口服务。真实媒体测试 `TestVideoProcessorLive`、`TestUploadRealVideoLive` 均执行通过，没有跳过。测试用 ffmpeg 生成 1 秒 H.264 MP4 和 PNG 封面，经 Vite 使用的 API 端口向 `POST /api/videos/upload` 发起 multipart 请求。

| 场景 | HTTP | 上传目录文件数（视频/封面） | 核对结果 |
| --- | --- | --- | --- |
| 请求前 | - | `0/4` | 保留此前联调的封面文件 |
| 真实视频与封面发布 | 201 | `1/5` | 新作品 ID 8，详情接口可读取 |
| 封面内容无效 | 400 | `1/5` | 已处理视频被删除 |
| 标题为空 | 400 | `1/5` | 视频与封面均被删除 |
| 同一幂等键重放 | 200 | `1/5` | 返回同一作品，本次文件均被删除 |
| PostgreSQL 拒绝含 NUL 字节的标题 | 500 | `1/5` | 数据库事务失败，两份文件均被删除 |

对已发布的文件运行 ffprobe 得到时长 `1.000000` 秒、视频编码 `h264`；MP4 顶层 `moov` 块位于 `mdat` 前，确认 faststart 生效。`go test ./...`、`go vet ./...`、Web 21 项测试、Web 构建、`git diff --check`、`docker compose config --quiet` 和 OpenAPI YAML 解析及 operationId 唯一性检查均通过。新增的本地用户、作品和媒体文件保留以供复核。

随后通过 Vite 开发服务 `http://127.0.0.1:5173/api/videos/upload` 再次发送真实视频和封面，代理返回 201，创建作品 ID 9，确认网页使用的代理路径可达。上表文件数是在该代理验证之前记录的快照。

提交前重新执行 `scripts/integration-smoke.ps1` 时，旧 worker 未消费 RabbitMQ 队列，首次得到 31/32。用当前源码重启 worker 后，互动队列从 4 条待处理消息恢复为 0 条、消费者数为 1；脚本等待异步查看者状态后重跑，32/32 项通过。脚本现会在断言失败时返回非零退出码。

组合发布在正常的封面、字段和数据库失败路径上回收文件；进程在文件保存后突然退出或文件删除本身失败仍需要运维清理孤儿文件。旧的独立 `POST /api/uploads` 与 `POST /api/videos` 继续开放，使用这两个接口的客户端仍须自行管理已上传文件的生命周期。
