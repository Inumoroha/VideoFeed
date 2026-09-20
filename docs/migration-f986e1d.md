# f986e1d 曝光决策迁移审计

来源：`C:/Users/HP/Desktop/GCFeed`，提交 `f986e1d0fde6e019d8b9d23458efb74a1819bb81`（feat(recommendation): 新增曝光决策接口）。目标：`C:/Users/HP/Desktop/VideoFeed`，main 分支，起始 HEAD `f4138a4`。原有未跟踪 Python 缓存保留。

已读取源提交 `git show --stat`、`--name-only`、`--format=fuller` 和九个文件的完整 diff；检查目标 AGENTS.md、目录、配置、依赖、数据库模型及路由。目标采用 handler/application/domain/infrastructure 分层，沿用 GORM PostgreSQL 仓储和现有曝光模型。

## 逐文件映射

| 源路径（相对于原仓库） | 目标路径 | 新建情况与符号对应 | 数据、配置、依赖适配 | 测试和文档 |
| --- | --- | --- | --- | --- |
| README.md | README.md | 既有推荐候选和曝光状态已完成，补齐曝光决策状态 | 使用目标 9000 端口 | 推荐模块、本文 |
| apps/api/internal/application/recommendation/service.go | internal/application/recommendation/decisions.go | 新建；ExposureDecisionInput、ExposureDecisionResult、DecideExposures、ErrLoadExposureDecisionsFailed 原名迁入 | 复用 Service、时钟和仓储 | decisions_test.go |
| apps/api/internal/domain/recommendation/entity.go | internal/domain/recommendation/decisions.go | 新建；ExposureDecisionRequest、ExposureDecision、NewExposureDecisionRequest、RestoreExposureDecision 和三个常量原名迁入 | 包名适配 recommendation | decisions_test.go |
| apps/api/internal/domain/recommendation/repository.go | internal/domain/recommendation/repository.go | 修改；增加 ListRecentExposures | 签名含 context、用户、视频列表、起始时间 | 服务及仓储测试 |
| apps/api/internal/infra/persistence/recommendation/gorm.go | internal/infrastructure/persistence/recommendation/decisions.go、repository.go | 前者新建；ListRecentExposures 原名迁入，私有 recentExposureWindow 改用领域常量 | 沿用 exposures 表、ExposureModel、restoreExposure；候选查询继续用目标 videos 表 | decisions_test.go |
| apps/api/internal/interfaces/http/recommendation/dto.go | internal/handler/recommendation/decisions.go | 新建；三个 exposureDecision 请求/响应类型保留名称及 JSON 标签 | 无新增依赖 | HTTP 测试、OpenAPI |
| apps/api/internal/interfaces/http/recommendation/handler.go | internal/handler/recommendation/decisions.go | 同上；DecideExposures、exposureDecisionsResponseFromResult 原名迁入 | 复用现有 400/500 错误处理 | HTTP 测试、OpenAPI |
| apps/api/internal/interfaces/http/router/router.go | internal/router/register.go | 修改；路由放入目标 registerRecommendationRoutes | POST /internal/exposure-decisions，不加 JWT | recommendation_test.go |
| apps/api/test/recommendation_api_test.go | internal/handler/feed/exposure_decisions_test.go、recommendation_api_test.go | 前者新建；保留原测试、两个响应结构体和内存仓储 ListRecentExposures；既有测试路由补注册 | 复用已有互斥锁、深拷贝和请求辅助函数 | 原用例完整保留并补边界测试 |

## 语义审计

- RecentExposureWindow 为 7 × 24 小时；原因常量为 fresh、recently_exposed；候选召回改为复用该窗口，数值不变。
- 请求先校验 user_id > 0；scene 去空白并转小写、不能为空且最多 32 字节；request_id 去空白且最多 64 字节；每个 video_id 必须大于零。按首次出现顺序去重，无数量上限。
- 缺省、null 或空 video_ids 仍校验元数据；合法时返回非 null 空数组且不访问仓储。
- 查询只按用户、视频集合、last_exposed_at >= since 筛选，包含七天边界。scene 和 request_id 是元数据，不参与隔离；不检查用户或视频实体是否存在、是否已发布。
- 忽略仓储空条目；曝光命中返回 allowed=false 和最近曝光时间，否则 allowed=true 且不返回时间。时间指针复制；输出保持去重顺序。空 request_id 使用 omitempty。
- 查询失败统一转换 ErrLoadExposureDecisionsFailed，HTTP 返回通用 500；不自动重试或降级为允许曝光。上下文沿用请求；无额外连接和后台资源。
- 决策只读、不写流水、不增加计数、不占用名额，也不保证并发调用只有一个 allowed=true。独立的 SaveExposures 事务、唯一约束、原子累加和跨请求累计行为保持不变。
- 无 Redis 操作或 RabbitMQ 发布/消费变更；现有原子性、连接、拓扑、确认/拒绝/重试和关闭流程不受影响。无数据库字段、状态、索引、依赖、配置、环境变量或 Compose 变更；继续使用现有数据卷、健康检查和依赖顺序。
- 新增函数及结构体均补紧邻中文注释。源提交未修改 OpenAPI，目标按仓库规范补全实际新接口，保持无 JWT 的行为。

## 验证记录

2026-09-20 验证结果：

| 检查 | 结果 |
| --- | --- |
| gofmt（16 个新增/修改 Go 文件），再次 gofmt -l | 通过，无未格式化文件 |
| go test ./... | 通过；无专用连接的外部服务测试按既有规则跳过 |
| go vet ./... | 通过 |
| go test -race ./... | 通过，使用已安装 WinLibs GCC |
| git diff --check | 通过 |
| docker compose config --quiet | 通过 |
| OpenAPI YAML 解析、operationId 唯一性、全部本地引用解析 | 通过 |
| 新接口请求、200/400/500 响应示例的 schema 校验 | 通过；路径、请求字段、必填项、错误码与无 JWT 行为已对照实际代码 |
| 源代码声明逐项比较 | 17 个新增类型/函数/测试声明去除格式空白后与源实现一致；三个常量、错误值、仓储接口、路由和候选窗口替换单独核对 |
| 新增/修改函数及结构体中文注释检查 | 通过 |

原 `TestRecommendationExposureDecisionsAPI` 的曝光写入、元数据、顺序去重、已曝光拒绝、未曝光放行及非法视频断言全部保留。补充的验证分别位于：

- `internal/domain/recommendation/decisions_test.go`：校验顺序、规范化、字节长度边界、非法标识、切片及时间指针复制。
- `internal/application/recommendation/decisions_test.go`：空列表不访问仓储、精确窗口参数、空曝光条目、错误转换、无自动重试、显式重试和上下文取消；为原有 stubRepository 补齐接口能力。
- `internal/handler/feed/exposure_decisions_boundary_test.go`：HTTP 绑定/校验错误、空数组和省略字段、内部错误隐藏、七天边界、跨场景与用户隔离、未知视频、24 路并发结果一致且无写入。
- `internal/infrastructure/persistence/recommendation/decisions_test.go`：真实查询结构、用户及视频参数、含边界条件、全部曝光字段还原、空结果、空列表短路与查询错误。
- `internal/router/recommendation_test.go`：通过生产使用的路由注册函数验证新路径与无 JWT 行为。

### 真实 PostgreSQL 验证

在独立 Compose 项目 `videofeed-decisions-00023aee` 中启动 PostgreSQL 17，随机本机端口和独立数据卷；通过临时 `VIDEOFEED_TEST_POSTGRES_URL` 指向该库，执行：

```powershell
$gccRoot = 'C:\Users\HP\Tools\winlibs-mingw\mingw64'
$env:Path = "$gccRoot\bin;$env:Path"
$env:CC = "$gccRoot\bin\gcc.exe"
# VIDEOFEED_TEST_POSTGRES_URL 指向专用测试库；测试会创建并删除隔离 schema。
go test -race ./internal/infrastructure/persistence/recommendation -run '^TestRecommendationPostgresLive$' -count=1 -v
```

结果通过，测试耗时 1.67 秒。既有事务并发累加、唯一约束、回滚和显式重试继续通过；新增 `decisions_live_test.go` 经 Gin HTTP 处理器、应用服务和真实仓储验证：恰好七天拒绝、早于边界一微秒放行、跨场景去重、其他用户隔离、未知视频放行、16 路并发查询结果一致，曝光计数/时间及流水不变。唯一键冲突和不存在视频日志是既有测试主动注入的预期故障。

测试 schema、数据库连接、容器、数据卷与网络已清理，按 Compose 项目标记确认容器和数据卷无残留。这里的 HTTP 请求由 httptest 驱动真实 Gin 处理链，并非启动完整 API 进程或浏览器端到端验收；本次也未重跑 Redis/RabbitMQ 真实服务联调，新功能不调用这些服务。

### 最终完整性结论

重新列出了参考提交全部九个文件，逐项核对映射表中的文件、函数、类型、常量、错误、接口、测试、README 状态，以及目标新增 OpenAPI 和模块文档。源 README 中推荐候选、曝光写入两项已在前序迁移完成，本次保留；其余新增行为已迁入，未发现遗漏。配置、数据库结构、缓存和消息队列无源变更需要迁入。

迁移验证完成后，按用户要求将业务逻辑、测试和文档拆分提交；工作区原有 `scripts/__pycache__/` 未修改且不纳入提交。
