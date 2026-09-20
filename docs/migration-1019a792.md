# 1019a792 独立 worker 迁移审计

来源：`C:/Users/HP/Desktop/GCFeed`，提交 `1019a792a3061412aef37de2ed304a806a60bdda`（feat(deploy): 拆分独立 worker 部署）。目标：`C:/Users/HP/Desktop/VideoFeed`，起始 main / `cdb13b1`；原有未跟踪 Python 缓存保留。

已读取 git show --stat、--name-only、--format=fuller 和全部七个文件完整 diff；检查 AGENTS.md、目录、配置加载/环境绑定、PostgreSQL 模型、Redis 原子写入、RabbitMQ 消费及 HTTP 路由架构。原仓库使用 MySQL、8080、apps/api；目标使用 PostgreSQL、9000、根目录 Go 模块，不复制这些旧布局。

## 全部文件映射

| 源文件 | 目标文件 | 新建与符号对应 | 数据、配置及依赖适配 | 测试/文档 |
| --- | --- | --- | --- | --- |
| apps/api/Dockerfile | Dockerfile | 修改；增加 worker 构建和复制 | ./cmd/feed → ./cmd/server；保留已有 gcfeed-api，新增 videofeed-worker | Docker 构建、容器启动和 SIGTERM 验证；deployment.md |
| apps/api/cmd/worker/main.go | cmd/worker/main.go | 新建；configPath、main、startWorkers 保留职责；新增 run、eventConsumer；closeSQL/closeRabbitMQ 用 run 中 defer Close 等价代替 | NewService 替代源 embedding.New；database.New 返回目标 PostgreSQL GORM；缓存分包；显式关闭 Redis | main_test.go；真实独立进程联调 |
| apps/api/internal/infra/persistence/migration/migration.go | internal/infrastructure/persistence/migration/migration.go | 新建；AutoMigrate；新增 migrateTables | 保留目标全部模型、两类幂等表和错误上下文；EnsureStats 原样调用；EnsureTimelineIndex 由现有视频模型索引标签等价实现；事务锁串行迁移 | migration_test.go；真实并发重启与索引/表断言 |
| apps/api/internal/interfaces/http/router/router.go | internal/router/register.go | 修改 Register，移除三个消费者启动与向量服务装配，改调共享迁移 | API 保留消息发布、缓存、回填、推荐与全部 HTTP 路由 | 全量路由测试；真实 API 消费者数量为零 |
| apps/deploy.yaml | deploy.yaml | 新建，完整映射 17 个资源 | Namespace/Secret/PVC/Deployment/Service 全部保留；PostgreSQL 替代 MySQL；9000；目标配置与环境变量；目标镜像名及 worker 命令 | YAML、Kubernetes 结构模式与引用校验；deployment.md |
| apps/docker-compose.yml | docker-compose.yaml | 修改，新增 worker 服务 | 根目录上下文、配置挂载、数据库/JWT 环境变量、健康依赖；补齐已有 RabbitMQ 配置所用账户 | compose config；真实依赖与容器验证 |
| scripts/start.sh | scripts/start.sh | 修改 cleanup、新增 WORKER_PID 启动/监控；补齐 Unix 子进程清理 | 保留目标端口扫描、绑定重试、自动安装依赖及 Windows 进程树清理 | bash -n；部署文档 |

附加适配：`configs/config.docker.yaml` 使用服务 DNS rabbitmq；`scripts/verify_exposure_live.py --embedding` 构建/启动独立 worker，故障日志与 Redis 不可达用例也跟随 worker，保留既有断言；新增 `scripts/verify_worker_live.py` 验证独立进程行为。README、architecture、embedding/interaction/recommendation 模块文档与 deployment.md 更新真实路径、命令和行为。OpenAPI 无接口形状变化，仅说明独立消费者依赖。

## 逐函数、配置和资源核对

- configPath 为 ./configs/config.yaml；main 加载配置并订阅 SIGINT/SIGTERM。原 main 的依赖装配移入 run，使数据库/消息启动失败也先执行资源释放，再由 main 非零退出。
- startWorkers 依次启动 ActionWorker、带 FeedPreheater 的 FanoutWorker、VideoEmbeddingWorker；任一错误立即返回，不吞掉错误或增加隐式重试。API 的宽松 MQ 失败回退维持原行为，独立 worker 则启动失败退出。
- closeSQL、closeRabbitMQ 的关闭职责迁为 defer sqlDB.Close/broker.Close；Redis 在目标中额外明确关闭。取消上下文触发已有 ConsumeWithContext 停止消费；没有宣称新增排空在途消息能力。
- AutoMigrate 保留源十一类模型对应能力，并额外保留目标 ActionIdempotencyModel、FollowIdempotencyModel。视频状态、字段、唯一约束、统计补齐没有变化；时间线复合索引仍为 status、published_at DESC、id DESC。
- PostgreSQL 同时启动会有 DDL 竞争，因此目标共享迁移增加 pg_advisory_xact_lock(736496102)，所有 DDL/统计补齐在同一事务，失败回滚且自动释放锁；没有引入 MySQL 驱动或旧表名。
- 消息连接、交换机、队列、绑定和路由键沿用目标配置。三消费者共享现有 RabbitMQ 对象；向量与分发绑定各自队列，互不争抢同一队列。手动 Ack、失败 Nack(requeue=true)、无效 JSON Nack(requeue=false) 以及无自动重连保持原样。
- Redis 写入实现未修改，关注分发继续使用现有事务进行 ZADD、裁剪和有效期刷新；互动继续沿用已有并发控制、幂等与回退，向量继续唯一覆盖写。
- Kubernetes 四个 PVC 分别对应 PostgreSQL 10Gi、Redis 1Gi、RabbitMQ 2Gi、API 上传 10Gi；六个 Deployment 为 postgres/redis/rabbitmq/api/worker/web，五个 Service 不含 worker。Web NodePort 30080，API 9000，与现有 Nginx 一致。
- 数据库探针改为 pg_isready，其余服务就绪/存活探针保留；worker 无源健康探针，不添加不存在端点。API/worker 增加依赖端口等待；持久化单副本使用 Recreate 避免 RWO 卷滚动挂载冲突。
- Secret 内配置按目标 app/server/database/auth/redis/rabbitmq 重写，增加目标需要的 refresh_ttl；使用占位凭据，API/worker 环境变量一致。Compose 与 Kubernetes 都保留配置只读挂载，上传卷不提供给 worker。
- 源提交没有新增测试；目标补充启动顺序/阶段失败/取消传播/依赖校验、迁移锁失败回滚，并真实验证消息积压及恢复。

## 验证结果

2026-09-20 验证完成：

| 检查 | 结果 |
| --- | --- |
| gofmt 和最终 gofmt -l | 通过，所有新增/修改 Go 文件格式正确 |
| go test ./... | 通过，含新增 worker 启动与迁移失败测试 |
| go vet ./... | 通过 |
| go test -race ./... | 通过，使用已安装 GCC |
| git diff --check | 通过 |
| docker compose config --quiet | 通过 |
| bash -n scripts/start.sh | 通过，使用 Git Bash |
| 两个联调 Python 脚本语法检查 | 通过 |
| Dockerfile 最终版本构建 | 通过，镜像 videofeed-worker-migration:1019a792 同时包含两个二进制 |
| deploy.yaml | 17 个资源 YAML 解析、选择器/PVC/Secret 引用与内嵌配置检查通过；Kubernetes v1.34.0 standalone-strict 五类资源结构模式检查通过 |
| api/openapi.yaml | YAML 解析、operationId 唯一性通过；剔除描述文本后与迁移前文档相等，路径、字段、状态码与认证契约未变 |
| 注释及完整性审计 | 新增/修改命名函数与结构体均紧邻中文注释；13 个目标模型全部保留；API 无消费者 Start 调用 |

### 真实服务证据

1. `python -B scripts/verify_worker_live.py`：六组检查全部通过，项目 `videofeed-worker-b00a32fc`。API 单独运行时零消费者；实际注册、关注、发布和点赞产生三个队列积压；API/worker 并发重启完成迁移，worker 处理互动、关注流分发和向量；时间线索引及两个幂等表保留；worker 停止不影响 API，重启后收藏消息继续落库；全部进程退出后 AMQP 连接为零。见 [独立 worker 结果](worker-live-results.json)。
2. `python -B scripts/verify_exposure_live.py --embedding --output docs/worker-embedding-live-results.json`：十五组检查通过，项目 `videofeed-exposure-e2e-a0312964`。覆盖真实数据库/消息队列测试、独立 worker 的向量生成/幂等覆盖/无效消息丢弃、存储失败重新入队、Redis 不可达时向量仍工作、HTTP 并发、曝光事务和消息发布失败回退、RabbitMQ 重启持久性。见 [曝光与向量回归结果](worker-embedding-live-results.json)。
3. 最终 Docker 镜像在独立项目 `videofeed-worker-signal-fbf52eaa` 中连接真实 PostgreSQL、Redis、RabbitMQ，执行 `./videofeed-worker`。等待运行日志后发送 `docker stop --time 15`，确认收到 SIGTERM、输出停止日志且退出码为 0。见 [容器结果](worker-container-results.json)。

测试容器、数据卷、网络、API/worker 进程均已清理并按项目标签核对无残留。构建出的本地镜像保留用于复查；未推送镜像或代码。首次 worker 联调因测试键名误用 `feed:inbox:v1` 超时，修正为目标实际的 `feed:following:inbox:v1` 后完整重跑通过。首次容器日志断言只读标准输出而未读取 Go 日志使用的标准错误，补齐双通道采集后重新验证通过；这两项均为验证工具修正，不是业务失败。

### 最终审计与边界

重新列出源提交全部七个文件，逐一核对迁移表和源变更，确认 Docker 双二进制、独立入口/信号/关闭、三个消费者、共享迁移、全部部署资源、Compose worker、启动脚本监控及清理都已迁入。旧 MySQL 索引创建方法由目标现有模型实现，旧 closeSQL/closeRabbitMQ 由 defer Close 实现；额外目标幂等表、HTTP 行为和失败回退均保留。未发现遗漏的源提交行为。

当前 kubectl 没有配置集群上下文，未执行真实 Kubernetes apply、PVC 绑定、滚动部署或 NodePort 验收；清单通过静态结构检查。未启动完整 Compose Web 界面或浏览器播放；本次实际验证覆盖独立后端进程和 Linux worker 镜像。start.sh 通过语法检查，完整交互式三进程终端流程未实际启动。

worker 的正常终止经过验证，但现有消息层不提供在途处理全部排空或断线自动重连保证；这些边界在部署说明中明确保留。所有新增行为的迁移已完成，以上外部部署边界不作为已验证内容。

验证完成后按用户要求将业务逻辑、基础设施与部署、测试、文档分主题提交；原有未跟踪 `scripts/__pycache__/` 未修改且不纳入提交。
