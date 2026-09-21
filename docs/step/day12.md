# Day 12：推荐流闭环、关注回填与独立 worker 部署

## 今日目标

在 Day 11 曝光流水和视频文本向量存储的基础上，接入推荐候选、前端推荐页与曝光决策，补齐新关注作者的近期作品回填，并将后台消费者拆为独立进程。同步整理产品、工程、模块和 OpenSpec 文档，使实现、部署方式与接口契约保持一致。

本记录于 2026-09-21 整理，范围为 Day 11 提交 `0504172` 之后至 `ebf4fef` 的六次来源迁移、22 个主题提交。验证结果引用对应迁移审计与 2026-09-20 的真实服务证据；本次撰写开发记录不代表重新执行联调。

## 完成内容

| 来源提交 | 迁移主题 | 当前成果 |
| --- | --- | --- |
| `cafc119` | 推荐服务 | 候选召回、兴趣画像、评分、作者打散、游标分页及批量曝光记录 |
| `2df157c` | 推荐前端 | 推荐入口、登录后默认页面、串行续页、曝光上报及旧响应隔离 |
| `c160f400` | 新关注回填 | 小作者近期作品同步写入用户 inbox，保留关系幂等与缓存失败回退 |
| `f986e1d` | 曝光决策 | 只读判断近七天曝光，补齐输入边界、用户隔离和真实数据库并发验证 |
| `1019a792` | 独立 worker | API 与三个消费者分离，共享数据库迁移，适配本地、Compose 和 Kubernetes 示例 |
| `1d0389` | 文档与规格基线 | 整合产品、工程和模块说明，建立 OpenSpec，补齐既有关系接口文档 |

本轮还在用户目录安装 WinLibs GCC 16.1.0，通过当前进程的 PATH、CC 和 CGO 配置完成 Windows 全仓库竞态检测，补齐前期开发记录中的宿主工具链缺口。

## 关键设计

### 推荐候选与 Feed 接入

```text
GET /api/feed-items?scene=recommend
或 POST /api/feed/query，scene=recommend
  -> JWT 身份校验
  -> 推荐策略传递用户、游标、数量和请求标识
  -> 查询已发布候选，排除用户近七天曝光
  -> 读取观看行为与视频文本向量，构建兴趣向量
  -> 按相似度、热度和新鲜度评分，生成游标并打散作者
  -> 复用公共卡片与计数缓存，按候选顺序组装 Feed
```

兴趣向量基于最近三十天最多二百条 `play`、`complete` 行为，按观看时长和完播标记加权。存在画像时采用 `0.70 × similarity + 0.20 × log1p(hot_score)/10 + 0.10 × freshness`；冷启动采用 `0.65 × log1p(hot_score)/10 + 0.35 × freshness`。

游标包含评分、发布时间和视频 ID，在作者打散前生成。同作者候选尽量延后，但候选池只有同一作者时无法保证全部分离。推荐不缓存个性化页；卡片与计数仍复用 Redis，未命中时回源并尝试写回公共缓存。

推荐复用 `videos`、`video_stat`、`video_embedding`、`video_view_events` 和 `exposures`，没有引入向量数据库或新的推荐规则表。仓储按当前 PostgreSQL 适配，未复制旧仓库的 MySQL 表名和冲突更新语法。

| 接口 | 当前语义 |
| --- | --- |
| `POST /internal/recommendation-candidates` | 召回、评分、打散并返回分页候选 |
| `POST /internal/exposures` | 批内按视频去重，事务追加曝光流水并原子累加聚合 |
| `POST /internal/exposure-decisions` | 只读判断近期曝光，不记录实际曝光 |

三个内部接口当前没有 JWT 或服务鉴权。用户端推荐 Feed 必须登录；推荐领域的非法游标、过长请求标识经过 Feed 现有错误映射返回 500，内部候选接口对应返回 400。本轮保留该行为，不通过文档宣称错误映射已统一。

### 推荐前端与曝光语义

`web/src/App.jsx` 新增 `/recommend`，桌面和移动导航统一为推荐流、最新视频、关注流、热门榜单。已登录用户访问根路径或登录成功后进入推荐流，匿名根路径与退出登录仍进入时间线。

- 推荐页通过 `POST /api/feed/query` 请求，每页 10 条，同一轮首页及续页沿用 `context.request_id`。
- 进入最后三条时预加载下一页，同一时刻只允许一个续页请求；合并结果按视频 ID 去重。
- 场景、身份、加载轮次变化或组件卸载后，旧请求不能覆盖新状态。
- 续页失败保留已有卡片；推荐或关注场景续页、曝光收到 401 时清理登录并跳转认证页，首页 401 显示登录提示。
- 登录用户当前视频通过 `POST /api/video-view-events` 上报 `exposed`；同轮同视频只尝试一次，普通失败不自动重试。

前端本轮去重不等于后端请求幂等。后端相同 `request_id` 再次写曝光仍追加流水并增加 `exposure_count`；内部批量曝光只在单批内去重，任一视频无效或数据库写入失败回滚整个批次。

### 只读曝光决策

请求校验正数用户及视频 ID；场景去空白、转小写后非空且最多 32 个 UTF-8 字节，请求标识去空白后最多 64 字节。视频列表按首次出现顺序去重；空列表仍校验元数据，成功返回 `decisions: []`，不查询仓储。

查询按用户、视频集合与 `last_exposed_at >= since` 筛选，包含七天边界；场景与请求标识不参与隔离。命中返回 `allowed=false`、`reason=recently_exposed` 和最近曝光时间，其余返回 `allowed=true`、`reason=fresh` 并省略时间。

该接口不检查用户或视频实体是否存在，也不检查发布状态；不写流水、不增加计数、不预占名额。因此并发请求可以同时获得允许结果，调用方仍需单独记录实际曝光。数据库失败返回 500，不自动重试或放行。

### 新关注作品回填

启用 Redis 后，两条既有关注入口均在关系事务提交成功后执行回填：

```text
保存关注、双方计数和幂等快照
  -> 读取目标作者粉丝数
  -> 不足 10000：查询最近最多 100 条已发布且有发布时间的作品
  -> 按发布时间和视频 ID 倒序，逐条写当前用户 inbox
  -> 每条写入保留事务内去重、容量裁剪和 30 天续期
```

inbox 最多保留 1000 条。达到大作者阈值、取关或未配置 Redis 时不回填。重复有效关注及幂等重放会再次尝试回填，相同 Redis 成员不会重复计数。

数据库和缓存不是一个事务：回填中途失败返回统一 500，已经保存的关系、统计与部分缓存写入不会回滚。客户端可用原幂等键显式重试，没有新增后台重试任务或 MQ 消息。

目标仓库本来就将全部有效关注作者传给索引读取，并核验公开视频状态；本轮保留这些保护。小作者作品不会被大作者名单误过滤，取关、删除和缓存故障仍沿用现有过滤或数据库回源。

### 独立 worker 与共享迁移

API 现在只处理请求、访问存储并发布消息。`cmd/worker` 按顺序启动三个消费者：互动异步落库、关注流分发与缓存预热、视频文本向量生成。

| 事项 | 当前实现 |
| --- | --- |
| 数据库迁移 | API 与 worker 共用 `internal/infrastructure/persistence/migration.AutoMigrate` |
| 并发启动 | PostgreSQL 事务级咨询锁 `pg_advisory_xact_lock(736496102)` 串行化迁移，失败回滚并释放锁 |
| 数据保留 | 全部 13 个模型、互动/关注幂等表、时间线复合索引及统计补齐均保留 |
| 启动失败 | worker 任一依赖初始化或订阅失败即返回错误，释放已取得资源并退出 |
| 正常关闭 | 接收 SIGINT/SIGTERM，取消消费上下文并关闭 RabbitMQ、Redis 和数据库连接 |
| 消息处理 | 成功逐条确认；非法 JSON 拒绝且不重入队；处理失败重新入队 |

worker 暂停时 API 仍可服务，异步任务在队列积压，恢复后继续处理。worker 没有 HTTP 端口或 `/health`。独立部署没有增加发布确认、断线自动重连、失败退避、死信重放或在途消息全部排空保证。

这也更新了 Day 11 的历史部署边界：向量消费者不再随 API 路由装配启动，而由独立 worker 启动；worker 订阅失败会退出，不是只记录日志后继续运行。

### 文档与 OpenSpec 基线

- README 保留启动方式和文档入口，功能状态集中到 `docs/product.md`。
- `docs/engineering.md` 合并工程规范；删除重复的模块规范，`docs/standard.md` 保留兼容链接。
- 账户文档映射到现有 `user`；七个已实现模块与六个规划模块采用职责、接口、数据、规则、测试和前端接入六项结构。
- 修正架构、阅读路线和优化专题中的旧目录、MySQL、端口、缓存及分发描述，规划能力明确标注。
- 新增 OpenSpec 项目上下文、提案、设计、任务和规格，保留四项要求与六个场景，严格校验通过；尚未归档变更。
- 路由审计补记两条既有取关 DELETE 和四条关系列表 GET。OpenAPI 的 34 个方法与路径和 Gin 一致，不含静态文件通配路由，也不包含规划接口。

## 文件与部署对应

| 能力 | 当前项目位置 |
| --- | --- |
| 推荐领域、服务与处理器 | `internal/domain/recommendation/`、`internal/application/recommendation/`、`internal/handler/recommendation/` |
| 推荐查询与曝光事务 | `internal/infrastructure/persistence/recommendation/` |
| Feed 推荐策略 | `internal/application/feed/recommendation.go` |
| 推荐页面及测试 | `web/src/App.jsx`、`web/src/style.css`、`web/tests/recommendation.test.mjs` |
| 关注回填 | `internal/application/relation/backfill.go`、`internal/router/follow_backfill.go`、`internal/infrastructure/persistence/feed/backfill.go` |
| 独立消费与迁移 | `cmd/worker/main.go`、`internal/infrastructure/persistence/migration/migration.go` |
| 部署 | `Dockerfile`、`docker-compose.yaml`、`deploy.yaml`、`scripts/start.sh`、`configs/config.docker.yaml` |
| 真实联调工具 | `scripts/verify_backfill_live.py`、`scripts/verify_worker_live.py`、`scripts/verify_exposure_live.py` |
| 工程与规格 | `docs/engineering.md`、`openspec/project.md`、`openspec/changes/fill-platform-basics/` |

Compose 现有六个服务为 postgres、redis、rabbitmq、api、worker、web。API 与 worker 共用配置和数据库/JWT 环境变量，等待三个基础服务健康；worker 使用 `on-failure` 重启。上传卷只供 API 使用，四个既有持久卷保留。API、Web、宿主 PostgreSQL 端口分别为 9000、5173、5433。

Docker 镜像同时包含 `gcfeed-api` 和 `videofeed-worker`。Kubernetes 示例包含 17 个资源，已适配 PostgreSQL、目标端口、持久卷、依赖等待及探针；没有为 worker 虚构健康接口。

仓库根目录的本地启动命令为：

```bash
go run ./cmd/server
go run ./cmd/worker
npm --prefix web run dev
```

三个命令分别运行；也可使用 `bash scripts/start.sh` 管理三个进程，或 `docker compose up -d --build` 启动容器环境。具体环境变量、账号一致性和关闭边界见[部署说明](../deployment.md)。

## 测试与真实联调

### 迁移阶段验证

- `go test ./...`、`go vet ./...`、配置 GCC 后的 `go test -race ./...` 通过；未提供专用连接时，外部服务测试按原约定跳过，不能用全量测试通过替代真实联调。
- 前端 5 项组件/请求测试与生产构建通过，覆盖串行分页、去重、曝光、失败保留和旧响应隔离；这些测试使用模拟 HTTP。
- `git diff --check`、Compose 配置、OpenAPI YAML、唯一操作标识及引用检查通过；新增契约示例通过模型校验。
- worker 迁移完成 Docker 构建、启动脚本语法、Kubernetes YAML/引用及结构模式检查。
- OpenSpec 全部规格与指定变更的严格检查通过，文档链接和模块模板检查通过。

代码迁移时新增和修改的 Go 文件已运行 gofmt；最后的纯文档迁移对全仓库执行 `gofmt -l`，仍发现 8 个未修改旧文件的格式差异，已逐个确认与基线一致。不能据此声称全仓库格式检查无差异，名单见[文档迁移审计](../migration-1d0389.md)。

### 真实服务证据

| 验证 | 结果与边界 |
| --- | --- |
| 关注回填 | 8 组检查通过：最近 100 条、16 次并发同键关注、用户隔离、取关过滤、9999/10000 阈值、缓存错误后重试及容量裁剪 |
| 推荐与曝光决策 PostgreSQL | 真实数据库下运行竞态测试，验证曝光原子累加、唯一约束、回滚、七天边界、跨场景/用户隔离及 16 路只读并发；HTTP 由 httptest 驱动真实 Gin 链路，不是完整进程或浏览器验收 |
| 独立 worker | 6 组检查通过：API 零消费者、三个队列积压、并发迁移、三类消费、索引/幂等表保留、停止与重启恢复、退出连接释放 |
| 曝光与向量回归 | 15 组检查通过：实际 API 与独立 worker、数据库与消息测试、向量覆盖、存储失败重投、Redis 不可达、曝光事务及 MQ 故障恢复 |
| Linux worker 容器 | 最终镜像连接真实依赖启动，收到 SIGTERM 后输出停止日志，退出码为 0 |

回填测试通过隔离数据库直接构造部分作品和粉丝统计，媒体 URL 为占位值；不代表一万名真实粉丝压测或浏览器播放成功。各轮临时进程、测试容器、数据卷与网络均按报告清理。

复现入口如下，脚本会创建隔离测试服务；结果路径为重新运行时的输出位置，不表示本次撰写执行了这些命令：

```powershell
python -B scripts/verify_backfill_live.py
python -B scripts/verify_worker_live.py
python -B scripts/verify_exposure_live.py --embedding --output docs/worker-embedding-live-results.json
```

证据见[回填报告](../backfill-live-verification.md)、[回填结果](../backfill-live-results.json)、[曝光决策真实数据库审计](../migration-f986e1d.md)、[worker 结果](../worker-live-results.json)、[曝光与向量回归结果](../worker-embedding-live-results.json)和[容器结果](../worker-container-results.json)。

本次 Day 12 仅新增 Markdown，核对事实来源、提交、路径、链接和差异，不重复运行代码测试或真实服务联调。

## 提交记录

下表按主题列出本轮已完成的本地提交，哈希顺序与各主题的开发顺序一致。

| 主题 | 当前仓库提交 |
| --- | --- |
| 推荐后端：基础设施、业务、测试、文档 | `437f738`、`68e7602`、`0dc1fde`、`0d658d1` |
| 推荐前端：依赖、页面、测试、文档 | `a20d056`、`03a5931`、`9ad5ed4`、`e5f0479` |
| 关注回填：业务、测试及联调脚本、文档 | `7aa46ed`、`6092352`、`f4138a4` |
| 曝光决策：业务、测试、文档 | `54a9020`、`0429060`、`cdb13b1` |
| 独立 worker：业务、部署、测试、文档 | `e74fd81`、`fa013ee`、`aa653fa`、`377b631` |
| 文档基线：接口契约、模块、项目工程、OpenSpec 与审计 | `640ce6b`、`d3fd512`、`5f6d1e3`、`ebf4fef` |

逐文件、逐符号及配置映射见[推荐迁移](../migration-cafc119.md)、[前端迁移](../migration-2df157c.md)、[回填迁移](../migration-c160f400.md)、[曝光决策迁移](../migration-f986e1d.md)、[worker 迁移](../migration-1019a792.md)和[文档迁移](../migration-1d0389.md)。

## 当前边界与后续计划

推荐查询、页面分页、观看上报和曝光过滤已经连通，但尚无推荐质量评估、实验平台或后台画像消费者；兴趣画像由推荐查询时读取历史行为生成。本地哈希向量不等同于训练语义模型，也未引入向量索引召回。

数据库与 MQ 发布仍非同一事务，持续故障可能丢失后续事件。消费者处理失败直接重入队，无退避或死信；运行中断线后可能需要重启 worker。正常退出验证不等于在途任务完整排空。视频向量仍缺乏事件版本保护、历史重建和删除联动。

小作者新关注回填只覆盖近期最多 100 条作品，不等于全量历史补偿。关注索引仍有容量、有效期和消费延迟，也没有完整性水位；一次足够大的缓存页不保证包含所有最新候选。

Kubernetes 尚无可用集群上下文，未实际验证 apply、PVC 绑定、滚动部署或 NodePort；三进程启动脚本仅做语法检查，未运行完整交互式终端流程。本轮未完成真实浏览器推荐播放、完整 Compose Web 验收或生产容量测试。

- 建立推荐质量指标与离线评估，明确重复曝光、作者打散和冷启动的验收目标。
- 规划内部推荐接口鉴权，统一 Feed 与内部接口的输入错误映射；落实前保持文档准确描述现状。
- 补充事件发件箱、发布确认、重连、退避、死信重放和停机处理协议。
- 设计向量版本保护、历史重建，以及关注索引过期和作者跨阈值后的补偿机制。
- 增加队列积压、消费者存活、缓存回源和推荐延迟观测，并补真实浏览器及集群部署验收。
