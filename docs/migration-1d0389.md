# 1d0389 文档与 OpenSpec 基线迁移审计

## 来源与边界

- 来源：`C:/Users/HP/Desktop/GCFeed`，提交 `1d0389f72b9d7fe177aa66f15fb599fe21196bd2`。
- 目标：`C:/Users/HP/Desktop/VideoFeed`，起点 `377b631`，分支 `main`。
- 源提交仅修改 Markdown 文档：27 个文件，无运行时代码、测试、依赖、配置或数据库迁移。
- 已读取提交统计、文件清单、完整提交信息和逐文件 diff；删除文档也纳入合并审查。
- 工作区原有 `scripts/__pycache__/` 不属于本次迁移。

## 逐文件迁移表（实施前建立，交付时 27 项全部复核）

| 原文件 | 新仓库对应文件 | 新建 | 对应关系与适配方式 | 测试或文档落点 |
| --- | --- | --- | --- | --- |
| `README.md` | README.md | 否 | 保留文档主题，按当前实现和路径适配 | 文档链接、路由对照、OpenSpec 严格校验；既有 Go/Web 回归 |
| `docs/engineering.md` | docs/engineering.md | 是 | 根 Go 模块、infrastructure/handler/router；NewService/NewHandler；PostgreSQL users/videos；中文注释与 OpenAPI 规范 | 文档链接、路由对照、OpenSpec 严格校验；既有 Go/Web 回归 |
| `docs/modules/README.md` | docs/modules/README.md | 是 | 保留文档主题，按当前实现和路径适配 | 文档链接、路由对照、OpenSpec 严格校验；既有 Go/Web 回归 |
| `docs/modules/account.md` | docs/modules/user.md | 否 | account→user；User/Repository/Service/Handler 沿用现有包；注册与资料更新无幂等键 | 文档链接、路由对照、OpenSpec 严格校验；既有 Go/Web 回归 |
| `docs/modules/admin.md` | docs/modules/admin.md | 是 | 保留文档主题，按当前实现和路径适配 | 文档链接、路由对照、OpenSpec 严格校验；既有 Go/Web 回归 |
| `docs/modules/feed.md` | docs/modules/feed.md | 否 | 真实缓存键、WATCH/事务、TTL、异步 fanout 与同步回填；区分现状和规划 | 文档链接、路由对照、OpenSpec 严格校验；既有 Go/Web 回归 |
| `docs/modules/governance.md` | docs/modules/governance.md | 是 | 保留文档主题，按当前实现和路径适配 | 文档链接、路由对照、OpenSpec 严格校验；既有 Go/Web 回归 |
| `docs/modules/interaction.md` | docs/modules/interaction.md | 否 | 保留文档主题，按当前实现和路径适配 | 文档链接、路由对照、OpenSpec 严格校验；既有 Go/Web 回归 |
| `docs/modules/message.md` | docs/modules/message.md | 是 | 保留文档主题，按当前实现和路径适配 | 文档链接、路由对照、OpenSpec 严格校验；既有 Go/Web 回归 |
| `docs/modules/monitoring.md` | docs/modules/monitoring.md | 是 | 保留文档主题，按当前实现和路径适配 | 文档链接、路由对照、OpenSpec 严格校验；既有 Go/Web 回归 |
| `docs/modules/playback.md` | docs/modules/playback.md | 是 | 保留文档主题，按当前实现和路径适配 | 文档链接、路由对照、OpenSpec 严格校验；既有 Go/Web 回归 |
| `docs/modules/recommendation.md` | docs/modules/recommendation.md | 否 | 三个内部路由无 JWT；批内去重、跨请求累计；exposures 替代规划 exposure_log，reco_rule 未实现 | 文档链接、路由对照、OpenSpec 严格校验；既有 Go/Web 回归 |
| `docs/modules/relation.md` | docs/modules/relation.md | 否 | 保留文档主题，按当前实现和路径适配 | 文档链接、路由对照、OpenSpec 严格校验；既有 Go/Web 回归 |
| `docs/modules/review.md` | docs/modules/review.md | 是 | 保留文档主题，按当前实现和路径适配 | 文档链接、路由对照、OpenSpec 严格校验；既有 Go/Web 回归 |
| `docs/modules/standard.md` | docs/engineering.md（删除重复规范） | 否 | 根 Go 模块、infrastructure/handler/router；NewService/NewHandler；PostgreSQL users/videos；中文注释与 OpenAPI 规范 | 文档链接、路由对照、OpenSpec 严格校验；既有 Go/Web 回归 |
| `docs/modules/video.md` | docs/modules/video.md | 否 | 保留文档主题，按当前实现和路径适配 | 文档链接、路由对照、OpenSpec 严格校验；既有 Go/Web 回归 |
| `docs/optimization.md` | docs/optimization.md | 否 | 真实缓存键、WATCH/事务、TTL、异步 fanout 与同步回填；区分现状和规划 | 文档链接、路由对照、OpenSpec 严格校验；既有 Go/Web 回归 |
| `docs/product.md` | docs/product.md | 是 | 保留文档主题，按当前实现和路径适配 | 文档链接、路由对照、OpenSpec 严格校验；既有 Go/Web 回归 |
| `docs/quickread.md` | docs/quickread.md | 否 | 保留文档主题，按当前实现和路径适配 | 文档链接、路由对照、OpenSpec 严格校验；既有 Go/Web 回归 |
| `docs/sdd.md` | docs/engineering.md；docs/standard.md 兼容入口 | 否 | 根 Go 模块、infrastructure/handler/router；NewService/NewHandler；PostgreSQL users/videos；中文注释与 OpenAPI 规范 | 文档链接、路由对照、OpenSpec 严格校验；既有 Go/Web 回归 |
| `docs/system optimization.md` | docs/optimization.md（已存在，整合替换） | 否 | 真实缓存键、WATCH/事务、TTL、异步 fanout 与同步回填；区分现状和规划 | 文档链接、路由对照、OpenSpec 严格校验；既有 Go/Web 回归 |
| `docs/uiux.md` | docs/uiux.md | 否 | 保留文档主题，按当前实现和路径适配 | 文档链接、路由对照、OpenSpec 严格校验；既有 Go/Web 回归 |
| `openspec/changes/fill-platform-basics/design.md` | openspec/changes/fill-platform-basics/design.md | 是 | 迁移项目上下文、提案、设计、任务或四项规格要求；严格校验 | 文档链接、路由对照、OpenSpec 严格校验；既有 Go/Web 回归 |
| `openspec/changes/fill-platform-basics/proposal.md` | openspec/changes/fill-platform-basics/proposal.md | 是 | 迁移项目上下文、提案、设计、任务或四项规格要求；严格校验 | 文档链接、路由对照、OpenSpec 严格校验；既有 Go/Web 回归 |
| `openspec/changes/fill-platform-basics/specs/platform-basics/spec.md` | openspec/changes/fill-platform-basics/specs/platform-basics/spec.md | 是 | 迁移项目上下文、提案、设计、任务或四项规格要求；严格校验 | 文档链接、路由对照、OpenSpec 严格校验；既有 Go/Web 回归 |
| `openspec/changes/fill-platform-basics/tasks.md` | openspec/changes/fill-platform-basics/tasks.md | 是 | 迁移项目上下文、提案、设计、任务或四项规格要求；严格校验 | 文档链接、路由对照、OpenSpec 严格校验；既有 Go/Web 回归 |
| `openspec/project.md` | openspec/project.md | 是 | 迁移项目上下文、提案、设计、任务或四项规格要求；严格校验 | 文档链接、路由对照、OpenSpec 严格校验；既有 Go/Web 回归 |

## 实际迁移与补充适配

- README 改为启动和文档入口；完整 P0/P1 功能表迁入 product。quickread 改用真实代码位置、9000 端口、根目录命令；uiux 关联工程规范并标明规划范围。
- 新 engineering 保留源提交全部十五项工程主题，并合并目标仓库中文注释、Conventional Commits、PostgreSQL 并发迁移、Redis 原子性、MQ 失败语义与 OpenAPI 规范。删除 modules/standard，docs/standard 只保留兼容链接；目标原本没有 sdd 文件，不创建伪旧路径。
- optimization 在目标已有文件上整合，替换失效的 MySQL、缓存键和分发描述；旧长文的扩展方向及性能目标明确归为待落地、待压测内容。
- 六个规划模块新增完整职责、接口、表、规则、测试建议和前端设计，按 PostgreSQL 类型适配，不增加规划接口到 OpenAPI。
- 六个已有业务模块按模板组织；account 映射 user，保留用户 DTO、曝光累加、推荐评分、关注回填、消息失败与重试等目标既有细节。目标特有 embedding 文档也纳入六项模板。
- architecture 修正陈旧技术栈和流程图；business-guide 保留历史正文并明确时效，migrations 索引保留原 README 中所有迁移/联调入口。历史迁移记录、开发步骤与联调结果未修改。
- OpenSpec 新增五个文件：project、proposal、design、tasks、delta spec。四项 Requirement 和六个 Scenario 与源提交逐字比较，仅替换项目名；任务在实际校验完成后勾选。
- 额外修复 OpenAPI 漏项：两条既有取关 DELETE、四条既有关系列表 GET，分别覆盖 users/me 与 relations 入口。复用 FollowResult/ErrorResponse，新增 RelationUser/RelationListResponse；补充参数、鉴权、真实状态码、幂等边界和示例。不修改任何接口代码。

## 符号、行为、测试与配置逐项审计

| 审计对象 | 结果与证据 |
| --- | --- |
| 导出类型、接口、函数、常量 | 源提交没有 Go/JS 等源码变更，因此没有需迁移的新增或修改运行时符号；文档提及的 Service/Handler/Repository、TokenSigner、状态枚举、限制和构造器映射到当前包，不强制重命名代码 |
| 领域及数据库 | users/videos、13 个迁移模型、两个幂等快照表、时间线索引、PostgreSQL 行锁/唯一约束/事务保持原实现；规划 reco_rule 不生成实际表，exposure_log 对应现有流水和曝光聚合 |
| Redis | 对照 cache/feed/feed.go、following.go：WATCH 行为键、16 个计数分片、事务管道与原子累加；关注索引添加/裁剪/30 天续期；无改动，不以普通读写替代 |
| 常量与缓存 | Timeline 5/45 秒加 10%–20% 抖动；卡片 15 分钟、统计 15 秒；Hot 60 个分钟桶，桶 2 小时、窗口 2 分钟；粉丝阈值 10000、分发批 500、inbox 1000、outbox 500、回填 100 条均按当前代码核对 |
| 热榜和回退 | Redis Hot 窗口失败返回错误，不假称自动回源；无缓存时使用数据库累计热度。Timeline/公共卡片/关注索引各自保留真实回源行为；推荐 Feed 错误映射的 500 边界单独说明 |
| RabbitMQ | 沿用交换机、队列、路由键和持久化配置；成功确认、非法 JSON 拒绝不重入队、处理失败重入队、取消/关闭；无发布确认、自动重连、退避或死信重放。API 初始化失败禁用发布器，worker 初始化或订阅失败关闭资源并报错 |
| 幂等与失败 | 用户注册/资料更新、上传、视频删除不支持幂等头；发布和关系快照保持既有语义。曝光批内去重但跨请求累计；推荐决策只读。Redis 互动失败返回错误，MQ 互动发布失败同步写仓储；关注回填失败不回滚已提交关系 |
| 测试迁移 | 源提交未新增测试文件或断言，仅新增文档测试建议；这些建议对应至各模块第 5 节的现有 Go/Web 测试位置。规划模块保留测试设计，不编造已执行测试；本次无运行时逻辑变化，不新增镜像实现的测试 |
| 配置及部署 | 源提交没有配置、环境变量、依赖、Compose、数据卷或健康检查变更。目标六服务和四持久卷不变，API/worker 依赖顺序不变；部署路径/端口/命令按目标适配 |
| 注释 | 本次没有新增或修改函数、方法、结构体；中文注释规范已保留于工程基线 |
| API 文档 | 从 register.go 抽取并与 OpenAPI 比较，34 个方法+路径全部一致，无缺失或虚构路径；operationId 全部唯一；本次六个补记操作的 26 个响应示例及 RelationUser 示例通过模型校验 |
| 变更范围 | 除 Markdown 和 api/openapi.yaml 外无代码、测试、配置或依赖修改；原 scripts/__pycache__/ 保留，不纳入迁移 |

## 验证结果（2026-09-20）

| 检查 | 结果 |
| --- | --- |
| go test ./... | 通过；利用已有有效测试缓存 |
| go vet ./... | 通过 |
| go test -race ./... | 使用已有 WinLibs GCC、CGO_ENABLED=1 通过；利用已有有效测试缓存 |
| npm --prefix web test | 5 项通过 |
| npm --prefix web run build | 通过 |
| git diff --check | 通过 |
| docker compose config --quiet | 通过 |
| OpenAPI YAML 解析、operationId、引用 | 通过，34 个唯一操作，本地引用可解析 |
| Gin/OpenAPI 路由对照 | 34/34，无缺失或多余实现路径（不含静态文件通配路由） |
| 产品已实现 API 表 | 29 项与 OpenAPI 对应；四个 relations 兼容入口另列于产品边界 |
| OpenSpec 1.2.0 validate --all --strict | 1 项通过、0 失败 |
| OpenSpec 1.2.0 validate fill-platform-basics --strict | 通过 |
| 文档链接、代码围栏、模块模板 | 变更 Markdown 本地链接可解析；13 个模块均含六项主题；围栏成对 |
| 源文件重新枚举 | 27/27 均出现在迁移表，含三个删除文件的整合映射 |
| gofmt -l（所有受 Git 跟踪 Go 文件） | 发现 8 个已有文件格式差异；逐个与 HEAD 内容比较确认没有本次改动，未为纯文档迁移扩大源码变更 |

既有 gofmt 差异文件：

- internal/application/video/dto.go
- internal/application/video/error.go
- internal/domain/user/error.go
- internal/domain/user/repository.go
- internal/domain/video/error.go
- internal/domain/video/repository.go
- internal/handler/video/handler.go
- internal/infrastructure/cache/redis/redis.go

## 未验证内容与交付边界

本次源提交的 27 个文档项均已迁移或按目标架构整合，无待迁移运行时能力。完整性结论来自源文件、文档主题和真实架构逐项对照，不来自测试通过这一单一信号。

本次未重新启动真实服务、浏览器或 Kubernetes 联调；需要专用 PostgreSQL/RabbitMQ 连接的测试可按原测试约定跳过，历史联调报告仅作为历史证据。未声明性能目标达标、规划模块已实现、OpenSpec 已归档，或全仓库 gofmt 完全通过。迁移验证完成后，按用户要求将变更拆为接口契约、模块文档、项目与工程文档、OpenSpec 与迁移审计四个主题创建本地提交；提交记录以 Git 历史为准。
