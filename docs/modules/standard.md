# GCFeed 后端项目标准

本文是 GCFeed 后端开发的基底规范。新增模块、改接口、改数据模型、写测试、更新文档，都以本文为准。

## 1. 核心原则

- 架构优先保持清晰边界，代码优先保持可读和可演进。
- 包名承载上下文，类型和函数名表达当前包内的具体职责。
- 路径表达资源，HTTP 方法表达动作。
- 业务规则放在 domain 和 application，HTTP 层负责协议适配，infra 层负责外部系统实现。
- 领域错误向上传递，应用错误表达用例失败，HTTP 层统一映射状态码。
- 幂等、事务、权限、分页、软删除等稳定性规则在新增功能时同步设计和测试。
- 文档是交付物的一部分，接口、模块边界、数据语义变化时同步更新文档。

## 2. 目录结构

后端代码位于仓库根目录。

```text
cmd/server/                                      # 程序入口
configs/                                         # 本地配置文件
api/openapi.yaml                                 # OpenAPI 契约
api/debug.http                                   # 本地接口调试请求
internal/
  application/{module}/                          # 用例服务、应用结果对象、分页和幂等编排
  auth/jwt/                                      # JWT 签发与校验
  config/                                        # 配置加载与校验
  database/                                      # PostgreSQL 和 GORM 连接
  domain/{module}/                               # 领域实体、领域错误、仓储接口
  handler/{module}/                              # HTTP handler、DTO、错误映射
  infrastructure/persistence/{module}/           # GORM 仓储实现
  middleware/                                    # Gin 中间件
  router/                                        # 路由注册与依赖装配
  server/                                        # HTTP 服务生命周期
```

新增业务模块使用同一套目录：

```text
internal/domain/{module}/{module}.go
internal/domain/{module}/error.go
internal/domain/{module}/repository.go
internal/application/{module}/service.go
internal/infrastructure/persistence/{module}/repository.go
internal/infrastructure/persistence/{module}/model.go
internal/handler/{module}/dto.go
internal/handler/{module}/handler.go
```

模块发展到需要某一层时，再创建对应文件。创建文件时保持职责完整。

## 3. 依赖方向

依赖方向固定为：

```text
cmd/server
  -> router
  -> handler/{module}
  -> application/{module}
  -> domain/{module}

infrastructure/persistence/{module}
  -> domain/{module}
```

`router` 负责装配 `handler`、application service 和 persistence repository。`application` 依赖 `domain.Repository` 接口。`infrastructure/persistence` 实现该接口。`handler` 依赖 application service。`domain` 保持纯业务模型，依赖范围限于标准库和领域内部包。

跨模块调用优先通过上层用例编排；领域层跨模块依赖要克制，只引用稳定枚举或错误时需要明确原因。仓储实现可以在查询聚合视图时读取其他表，例如 Feed 查询关联 account、video、video_stat。

## 4. 包命名

包声明默认使用业务模块名并保持全小写；导入多个同名包时，用“层 + 模块”作为 import alias。`internal/infrastructure/persistence/video` 当前使用历史包名 `infravideo`，新增模块不以此作为默认模式。

```go
// 常用 import alias。
applicationvideo
applicationfeed
applicationinteraction
applicationuser

domainvideo
domainfeed
domaininteraction
domainuser

videopersistence
feedpersistence
interactionpersistence
userpersistence

videohandler
feedhandler
interactionhandler
userhandler
```

import alias 要显式写出层和模块，调用处一眼能看出层和业务模块。

```go
import (
	applicationvideo "github.com/inumoroha/video-feed/internal/application/video"
	videopersistence "github.com/inumoroha/video-feed/internal/infrastructure/persistence/video"
	videohandler "github.com/inumoroha/video-feed/internal/handler/video"
)
```

## 5. 构造函数命名

构造函数命名遵循当前层的既有模式。

```go
repo := videopersistence.New(gormDB)
service := applicationvideo.NewService(repo)
handler := videohandler.NewHandler(service)
```

适用规则：

- persistence repository 使用 `New`。
- application service 使用 `NewService`，HTTP Handler 使用 `NewHandler`。
- 包内出现多个同级主要对象时使用 `New{Name}`，例如 JWT 的 `NewManager`。
- 领域实体工厂可以使用语义化名称，例如 `domainvideo.NewPublished(...)`、`domainvideo.RestoreVideo(...)`。
- 调用方变量名表达对象角色：`videoRepo`、`videoService`、`videoHandler`。

新增构造函数遵循所在层的既有命名模式。已有构造函数重命名时按模块批量调整调用点和测试。

## 6. 类型命名

每层内主类型使用短名，依靠包名表达上下文。

```go
type Service struct {}
type Repository struct {}
type Handler struct {}
type Video struct {}
type User struct {}
```

结果对象以业务结果命名：

```go
type CreateResult struct {}
type LoginResult struct {}
type TimelineFeedResult struct {}
type CommentListResult struct {}
type ActionResult struct {}
```

DTO 以请求或响应语义命名：

```go
type CreateVideoRequest struct {}
type videoResponse struct {}
type commentListResponse struct {}
```

外部 API 请求结构体按需导出。只在包内使用的 DTO 保持小写。

## 7. Domain 层

domain 层负责业务事实和业务规则。

文件职责：

- `{module}.go`：实体、值对象、状态常量、构造函数、领域方法。
- `error.go`：领域错误，供 application 和 HTTP 层识别。
- `repository.go`：领域需要的持久化能力接口。

实体规则：

- 新建实体使用 `New...` 工厂完成输入修剪和业务校验。
- 数据库恢复实体使用 `Restore...`，只做必要归一化。
- 状态常量集中定义，例如 `StatusPublished`、`StatusDeleted`。
- 领域方法承载权限和状态变化，例如 `DeleteBy(authorID)`。
- 长度、limit、幂等键等业务上限放在 domain 常量中。

错误规则：

```go
var ErrInvalidVideoID = errors.New("invalid video id")
var ErrVideoNotFound = errors.New("video not found")
var ErrVideoPermissionDenied = errors.New("video permission denied")
```

领域错误要稳定、可被 `errors.Is` 判断。错误文本使用小写英文，保持 API 返回简单可读。

## 8. Application 层

application 层负责编排用例。

职责：

- 校验跨字段和请求上下文中的业务输入。
- 调用 domain 工厂和领域方法。
- 调用 repository 接口完成读写。
- 处理幂等重放。
- 统一分页 limit、cursor、has_more。
- 将领域对象转换成应用结果对象。

结构：

```go
type Service struct {
	repo domainvideo.Repository
}

func New(repo domainvideo.Repository) *Service {
	return &Service{repo: repo}
}
```

方法命名使用业务动作：

```go
CreatePublished
Get
Delete
ListByAuthor
GetTimelineFeed
RefreshTimelineFeed
Like
Unlike
Favorite
Unfavorite
CreateComment
ListComments
DeleteComment
```

错误分层：

```go
var ErrLoadVideoFailed = errors.New("failed to load video")
var ErrSaveVideoFailed = errors.New("failed to save video")
var ErrUpdateVideoFailed = errors.New("failed to update video")
```

领域错误直接返回给 HTTP 层识别。仓储或外部依赖错误转换成应用层错误，HTTP 响应保持稳定的业务语义。

## 9. Repository 接口

repository 接口写在 domain 层，表达领域需要的能力。

```go
type Repository interface {
	Save(ctx context.Context, video *Video) error
	FindByID(ctx context.Context, videoID int64) (*Video, error)
	ListByAuthor(ctx context.Context, authorID int64, limit int, offset int) ([]*Video, error)
}
```

接口设计规则：

- 每个方法都接收 `context.Context`。
- 参数使用领域语义，使用 domain entity 或领域值对象承载业务数据。
- 返回领域实体或领域值对象。
- 幂等能力显式体现在方法名或参数中。
- 写操作需要返回最新统计值时，返回领域实体加计数。

## 10. Infra Persistence 层

GORM 实现放在 `internal/infrastructure/persistence/{module}`。

职责：

- 定义 GORM model 和表名。
- 实现 domain repository 接口。
- 处理事务、行锁、唯一键冲突、软删除、统计计数。
- 将 GORM model 转换成 domain entity。
- 将数据库错误映射成 domain error。

结构：

```go
type Repository struct {
	db *gorm.DB
}

func New(db *gorm.DB) *Repository {
	return &Repository{db: db}
}
```

事务规则：

- 需要同时写业务表和统计表时使用同一事务。
- 修改互动、评论、计数时锁定相关行。
- 幂等键命中时返回接口契约定义的首次执行结果；统计字段是快照还是当前值必须在模块文档和 OpenAPI 中明确。
- 软删除保留原始记录，计数只在状态真正变化时更新。
- 唯一键冲突映射成领域错误或触发幂等读取。

查询规则：

- 列表查询必须有稳定排序。
- 游标分页使用排序字段加主键，例如 `published_at + id`、`created_at + id`。
- 关联查询优先只选择需要字段，返回 domain entity 或领域值对象。
- `gorm.ErrRecordNotFound` 映射为对应 `Err...NotFound`。

## 11. HTTP 层

HTTP 层负责协议适配。

职责：

- 解析 path、query、header、body。
- 从 middleware context 读取用户身份。
- 调用 application service。
- 将应用结果转换成 JSON DTO。
- 将错误转换成 HTTP 状态码。

结构：

```go
type Handler struct {
	service *applicationvideo.Service
}

func New(service *applicationvideo.Service) *Handler {
	return &Handler{service: service}
}
```

Handler 方法只做协议适配，业务规则下沉到 application 或 domain。

DTO 规则：

- 请求体结构体放在 `dto.go`。
- 响应结构体放在 `dto.go`。
- JSON 字段使用 snake_case。
- 时间字段使用 Go 标准 JSON 时间格式。
- 内部辅助转换函数放在 handler 文件底部。

错误映射规则：

```go
func writeVideoError(c *gin.Context, err error) {
	if isBadRequestError(err) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if errors.Is(err, domainvideo.ErrVideoNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "video not found"})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
}
```

状态码规则：

- `200 OK`：查询成功、幂等重放成功、状态变更成功。
- `201 Created`：创建成功。
- `204 No Content`：删除成功且响应体为空。
- `400 Bad Request`：参数错误、请求体错误、游标错误、limit 错误。
- `401 Unauthorized`：访问 token 缺失或校验失败。
- `403 Forbidden`：身份有效，权限校验失败。
- `404 Not Found`：资源查找失败。
- `409 Conflict`：唯一约束冲突或重复业务资源。
- `500 Internal Server Error`：内部错误统一响应。

## 12. REST API 风格

路径使用名词资源，资源名使用复数，path 参数使用 camelCase。

当前资源风格：

```text
POST   /api/users
GET    /api/users/me
PATCH  /api/users/me
GET    /api/users/me/videos
GET    /api/users/:userId/videos

POST   /api/sessions
DELETE /api/sessions/current

POST   /api/videos
GET    /api/videos/:videoId
DELETE /api/videos/:videoId
PUT    /api/videos/:videoId/like
DELETE /api/videos/:videoId/like
PUT    /api/videos/:videoId/favorite
DELETE /api/videos/:videoId/favorite
POST   /api/videos/:videoId/comments
GET    /api/videos/:videoId/comments

POST   /api/uploads
GET    /api/feed-items
DELETE /api/comments/:commentId
```

设计规则：

- 创建资源使用 `POST /resources`。
- 获取集合使用 `GET /resources`。
- 获取单个资源使用 `GET /resources/:resourceId`。
- 部分更新使用 `PATCH`。
- 设置某个目标状态使用 `PUT`。
- 取消某个目标状态使用 `DELETE`。
- Feed 作为条目集合暴露为 `/feed-items`。
- 登录态建模为 session，登录是创建 session，退出是删除 current session。
- 评论创建和列表挂在视频下，删除评论按评论 ID 放在顶层 comments。

## 13. 分页和游标

普通列表可以使用 `limit + offset`。Feed、评论等强时序流使用 cursor。

limit 规则：

- 默认值放在 application 或 handler 常量中。
- 最大值放在 domain 或 application 的模块常量中，并在接口、服务和文档中保持一致。
- `limit <= 0` 视为参数错误或按场景使用默认值，需在模块内保持一致。

cursor 规则：

- cursor 使用 base64 URL 编码 JSON。
- payload 保存最后一条记录的排序字段和主键。
- 服务端多查一条判断 `has_more`。
- 返回字段统一为 `items`、`next_cursor`、`has_more`。

## 14. 幂等

写接口支持 `Idempotency-Key` 时，需要端到端实现。

规则：

- Header 名固定为 `Idempotency-Key`。
- 最大长度使用 domain 常量 `MaxIdempotencyKeyLength`。
- handler 将 `Idempotency-Key` 透传给 application。
- application 可以按用例预读已有结果，但不能依赖预读保证并发安全。
- repository 使用唯一键和事务完成最终的原子幂等判定。
- 重放请求返回同一业务结果。
- 统计计数只在状态真正变化时更新。

适用场景：

- 发布视频。
- 点赞、收藏状态设置。
- 评论创建。
- 后续支付、审核动作、任务领取等会产生副作用的接口。

## 15. 鉴权和上下文

JWT 能力由 `internal/auth/jwt` 提供；除用户登录服务为签发令牌直接依赖 JWT Manager 外，其余 application service 只依赖领域仓储接口。

规则：

- JWT middleware 解析 token 后写入 Gin context。
- context key 统一定义在 middleware 包。
- handler 从 context 读取 `userID` 和 `role`。
- application 方法显式接收 `userID`、`role` 等业务身份参数。
- 权限判断优先放在领域方法或 application 编排中。

## 16. 数据模型

GORM model 与 domain entity 分离。

规则：

- model 只描述数据库表结构和 GORM tag。
- entity 描述业务状态和行为。
- 表名以业务资源命名，保持稳定。
- 统计表使用独立 model，例如 `video_stat`。
- 软删除使用业务状态字段，便于审计和恢复。
- 自动迁移集中在 router 装配阶段，新增 model 时同步加入 `AutoMigrate`。
- 需要初始化或补齐统计行时提供 `Ensure...` 函数。

## 17. Go 代码风格

基础规则：

- 所有 Go 文件运行 `gofmt`。
- 函数保持短小，复杂分支拆成私有 helper。
- 早返回处理错误。
- `context.Context` 作为第一个参数传递给仓储和服务方法。
- 字符串输入进入业务前统一 `strings.TrimSpace`。
- 错误判断使用 `errors.Is`。
- 常量集中放在靠近使用处或 domain 层。
- 新增或修改函数必须在声明前紧邻添加简洁、准确的中文注释，说明职责、输入、输出或关键副作用。
- 新增或修改结构体必须在声明前紧邻添加简洁、准确的中文注释，说明职责、用途或重要约束。
- 注释解释实际业务意图、并发安全、事务边界、幂等原因和权限规则，不重复代码或编造未实现行为。

命名规则：

- 包名全小写。
- 导出类型使用业务名。
- 私有 helper 使用动词或转换语义，例如 `parseLimit`、`writeVideoError`、`videoResponseFromDomain`。
- bool 参数名表达目标状态，例如 `active bool`。
- 计数变量使用明确名称，例如 `likeCount`、`commentCount`、`favoriteCount`。

提交规则：

- 提交标题使用 Conventional Commits：`<type>(<scope>): <中文摘要>`。
- 标题和正文使用中文；正文采用 `- ` 开头的无序列表说明关键变更。
- 根据变更选择 `feat`、`fix`、`refactor`、`test`、`docs` 或 `chore`。
- 在 PowerShell 中创建多行正文时，使用反引号加 `n` 换行，不使用字面量 `\n`。

## 18. 测试标准

后端测试与被测代码共同位于 `internal/` 对应包内，文件名使用 `*_test.go`。

测试目标：

- API 主流程。
- 参数校验。
- 权限校验。
- 幂等重放。
- 分页和游标。
- 软删除重复请求。
- 统计计数变化。
- 错误状态码映射。

测试风格：

- 使用 Gin router 直接测试 HTTP 行为。
- 使用内存仓储实现 domain repository 接口。
- 测试命名采用 `Test{Module}APIFlow`、`Test{Module}APIValidation`。
- helper 命名保持明确，例如 `performJSONRequest`、`requireStatus`、`decodeJSON`。
- 新增 endpoint 必须补 API 测试。
- 新增 repository 事务逻辑时补仓储级测试或 API 级覆盖关键行为。

验证命令：

```bash
go test ./...
go vet ./...
git diff --check
```

## 19. 文档同步

这些文件共同维护后端长期一致性：

- `docs/modules/standard.md`：项目基底标准。
- `docs/architecture.md`：系统架构和关键链路。
- `docs/modules/*.md`：模块级需求、接口、数据和流程。
- `api/openapi.yaml`：已实现 HTTP 接口的正式契约。
- `api/debug.http`：本地接口调试请求。

变更同步规则：

- 新增模块：更新 `docs/modules/{module}.md`、架构图、标准中相关规则。
- 新增或修改接口：先完成 Gin 实现，再同步更新 `api/openapi.yaml`、模块文档、API 测试和受影响的 `api/debug.http` 请求。
- 修改数据模型：更新模块文档和架构数据模型。
- 修改命名或目录规范：更新本文。
- 修改核心链路：更新架构图和测试。
- OpenAPI 路径、字段、必填项、状态码、认证和示例必须与 Gin 实现一致；不要记录未实现接口。
- OpenAPI 使用项目采用的 `3.0.3` 版本；每个接口必须使用全局唯一的 `operationId`，并通过 YAML 解析校验。

## 20. 新增后端能力检查清单

开发前：

- 模块边界已经确定。
- domain 实体、错误、仓储接口已经设计。
- REST 路径和方法符合本文规则。
- 幂等、权限、分页、事务需求已经明确。

开发中：

- 除用户登录服务使用 JWT Manager 外，application 只依赖 domain repository 接口。
- infrastructure 只向上返回领域实体和领域错误。
- handler 只做协议适配和错误映射。
- 构造函数遵循所在层的既有模式：`New`、`NewService` 或 `NewHandler`。
- 新增 model 已加入迁移。

交付前：

- `gofmt` 已执行。
- `go test ./...` 通过。
- API 测试覆盖主流程和失败路径。
- `api/openapi.yaml`、模块文档和受影响的 `api/debug.http` 已同步。
- OpenAPI 的路径、字段、状态码和示例已与 Gin 路由和 handler 一致，并可被正常解析。
- 新增或修改的函数和结构体均已添加紧邻的中文注释。
