# API 与独立 worker 部署

API 处理 HTTP 请求并发布消息；独立 worker 负责互动异步落库、关注流分发/预热以及视频向量生成。两者必须连接相同 PostgreSQL、Redis、RabbitMQ，并使用一致的交换机、队列和路由键配置。worker 没有 HTTP 端口，也没有 `/health` 接口。

## 本地开发

准备 `configs/config.yaml` 和开发环境变量，启动基础服务后在仓库根目录分别运行：

```bash
go run ./cmd/server
go run ./cmd/worker
npm --prefix web run dev
```

也可以使用 `bash scripts/start.sh` 同时启动三个进程。脚本保留原有可用端口选择和 API 绑定失败重试；任一服务退出时清理其他服务，退出时停止 worker。Windows 使用 taskkill 清理进程树；Unix 使用 pgrep（需要系统提供）递归清理 go run/npm 的子进程。默认 API 9000、前端 5173，worker 不占监听端口。

worker 读取 `./configs/config.yaml`，要求 RabbitMQ URL、Redis 地址非空。复用现有完整配置加载器，所以即使 worker 不提供 HTTP/JWT 接口，仍须配置有效的 server、auth 等字段。默认优先使用环境变量；不要混用本地配置的 `gcfeed.interaction` 与 Docker 配置的 `videofeed.interaction` 队列。

## Docker Compose

```bash
docker compose config --quiet
docker compose up -d --build
docker compose logs -f api worker
docker compose restart worker
docker compose down
```

根目录 `Dockerfile` 同时构建已有 `gcfeed-api`（保留现有启动命令）和新 `videofeed-worker`。Compose 的 api 与 worker 均使用根目录构建上下文，挂载 `configs/config.docker.yaml`，传入相同 `DATABASE_URL` 和 `JWT_SECRET`。二者均等待 PostgreSQL、Redis 和 RabbitMQ 健康后启动；worker 启动失败时按 `on-failure` 重启。API 仍通过 `/health` 检查，worker 不配置不存在的 HTTP 健康检查。

容器内地址：PostgreSQL `postgres:5432`、Redis `redis:6379`、RabbitMQ `rabbitmq:5672`；宿主机端口分别为 5433、6379、5672/15672，API 9000，Web 5173。上传卷仅挂载 API，worker 不读取媒体文件；数据库、缓存、消息和上传卷保持原有名称与路径。

Compose 为新 RabbitMQ 数据卷初始化与 Docker 配置匹配的开发用户 `rabbitmq`。已有数据卷不会因 `RABBITMQ_DEFAULT_USER/PASS` 改变已有账户；升级时应配置实际可用账户或在 RabbitMQ 中创建匹配用户并授予对应虚拟主机权限，不需要删除数据卷。示例开发密码应在部署环境中替换。

## Kubernetes 示例

根目录 `deploy.yaml` 含 17 个资源：命名空间、配置 Secret、四个 PVC、六个 Deployment 和五个 Service。数据库按目标仓库改为 PostgreSQL 17，挂载 `/var/lib/postgresql/data`，设置 PGDATA 子目录；API 使用 9000 端口，与 Web Nginx 上游一致。

```bash
docker build -t videofeed-api:latest .
docker build -t videofeed-web:latest ./web
# 将镜像加载到目标集群或推送到自己的镜像仓库，并修改清单中的 image。
# 替换 deploy.yaml 中全部 Secret 占位值；数据库与消息连接 URL 中的密码须一致。
kubectl apply -f deploy.yaml
kubectl -n videofeed get pods,pvc,svc
kubectl -n videofeed logs deployment/worker
```

集群需提供默认 StorageClass，或为四个 PVC 指定实际存储类。PostgreSQL、Redis、RabbitMQ 和 API 的持久化挂载保留单副本并使用 Recreate 更新策略；多副本或外部对象存储需要另行设计。Web NodePort 为 30080；也可运行 `kubectl -n videofeed port-forward service/web 5173:80` 后打开 `http://127.0.0.1:5173`。

API 和 worker 使用同一 Secret 配置与环境变量。initContainer 等待三个依赖端口开放；服务启动后仍执行真实连接/迁移，失败由 Kubernetes 重启。PostgreSQL、Redis、RabbitMQ、API 和 Web 保留各自就绪与存活探针；worker 无虚构的健康端点。端口开放不等于完整业务就绪，需结合日志和队列消费者数量确认运行状态。Secret 使用 subPath 挂载，修改后需要重启 api/worker。

## 数据与失败语义

API 与 worker 共用 `internal/infrastructure/persistence/migration.AutoMigrate`。迁移保留全部目标模型，包括互动与关注幂等表；视频模型负责创建 `idx_video_timeline`，最后幂等补齐视频统计。PostgreSQL 事务级咨询锁串行化并发迁移；事务失败回滚并释放锁。

worker 按互动、分发、向量顺序订阅三个独立队列，任何启动失败均终止进程；关闭时取消上下文，再关闭 RabbitMQ、Redis、数据库。收到 SIGINT/SIGTERM 后正常退出；未增加等待所有在途消息完成的承诺，连接关闭后未确认消息由 broker 重新投递。

消息语义沿用现有实现：持久化 topic 交换机/队列，手动逐条确认；无效 JSON 拒绝且不重新入队，处理失败重新入队，成功 Ack。没有新增 publisher confirm、退避或自动重连。若运行中的 RabbitMQ 连接中断，进程可能仍存活但消费已停止，需要恢复依赖后重启 worker。保持原有互动幂等、向量唯一覆盖写、Redis 事务裁剪/去重和发布失败回退。

API 单独运行仍能提供 HTTP，但异步互动落库、分发、向量生成会积压；这是独立部署后的运行要求。已有曝光画像队列继续只发布，本次没有新增画像消费者。接口路径、认证、请求和响应不变，OpenAPI 仅补充异步任务依赖说明。

## 验证

```bash
go test ./...
go vet ./...
go test -race ./...
python -B scripts/verify_worker_live.py
python -B scripts/verify_exposure_live.py --embedding --output docs/worker-embedding-live-results.json
```

真实联调脚本创建独立 Docker 项目、随机本机端口和临时 API/worker 进程，并清理全部测试资源。Windows 脚本停止进程使用操作系统终止；Linux 容器的 SIGTERM 行为单独验证。结果和限制见 [迁移审计](migration-1019a792.md)。

## 消息中心内部调用

消息创建接口 `POST /internal/messages` 使用独立 `X-Internal-Token`。本地配置和 Compose 默认为空，默认拒绝内部创建请求。部署时设置 `INTERNAL_TOKEN`；Kubernetes 在 `deploy.yaml` 的 Secret 字段 `internal-token` 中填写，API 通过环境变量读取。不要将其传到 Web 客户端。用户消息查询、已读和互动触发通知仍可正常工作，不要求配置该令牌。

消息存储复用 PostgreSQL 和现有迁移咨询锁，不新增数据库、数据卷、队列或健康检查。API/worker 启动会迁移 `user_message`；消息通知由 API 写入，worker 无需内部令牌。端口仍为 API 9000、PostgreSQL 宿主机 5433。详见 [消息模块](modules/message.md) 和 [迁移审计](migration-7a62b5c.md)。


消息中心前端地址为 `http://127.0.0.1:5173/messages`。Vite 复用现有 `/api` 代理，生产 Nginx 的 `try_files ... /index.html` 支持直接访问和刷新该路径；不需要新增服务、环境变量或代理规则。前端不需要 `INTERNAL_TOKEN`。运行 `npm --prefix web test` 和 `npm --prefix web run build` 验证，生产镜像仍由 `web/Dockerfile` 构建。

## 播放优化迁移

播放模块已接入现有 PostgreSQL 自动迁移，无新增服务或环境变量。四条接口、分层适配和验证限制见 [迁移审计](migration-b28ef00.md) 与 [播放模块](modules/playback.md)。

## 上传媒体工具

根目录 Dockerfile 的运行镜像安装 Alpine `ffmpeg` 包（包含 ffprobe）；API 上传视频依赖这两个命令。API 与 worker 继续共用镜像，worker 不新增媒体任务。Compose 的服务、端口、上传卷、健康检查和启动依赖不变。

本地运行 API 前安装 FFmpeg 并将 ffmpeg/ffprobe 加入该进程的 PATH，可用 `ffmpeg -version`、`ffprobe -version` 检查。工具缺失时视频上传返回 500，普通文件和图片仍可按规则上传。不存在新增环境变量或队列。

视频处理为同步请求，须为实际媒体处理时间合理配置既有 HTTP 超时和反向代理超时/请求体限制；ffprobe 和 ffmpeg 分别有 30 秒及 2 分钟上限，部署默认 HTTP 超时可能更短。上传卷应有保存原文件与快启临时副本的空间。

真实媒体测试：`go test ./internal/handler/upload -run 'TestVideoProcessorLive|TestUploadRealVideoLive' -v`，缺少工具时明确跳过；完整验证记录见 [上传迁移审计](migration-34f6682.md)。
