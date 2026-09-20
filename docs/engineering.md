# VideoFeed 工程规范

本文定义 VideoFeed 的目录职责、代码风格、接口设计、数据模型和测试约定。新增功能时优先遵循本文，再查看对应模块文档。

## 1. 技术栈

| 区域 | 技术 |
| --- | --- |
| API | Go、Gin、GORM |
| 数据库 | PostgreSQL |
| 缓存 | Redis |
| 消息队列 | RabbitMQ |
| 鉴权 | JWT |
| Web | React、Vite |
| 规格驱动 | OpenSpec |

## 2. 后端分层

后端 Go 模块位于仓库根目录，采用四层结构：

```text

  cmd/server/main.go
  cmd/worker/main.go
  configs/
  internal/
    domain/{module}/
    application/{module}/
    infrastructure/
    infrastructure/persistence/{module}/
    handler/{module}/
    router/register.go
    config/
    database/
    auth/jwt/
    middleware/
  web/                         # 独立前端，测试位于各 Go 包内
```

| 层 | 职责 | 依赖方向 |
| --- | --- | --- |
| Domain | 实体、领域错误、业务不变量、仓储接口 | 不依赖 HTTP、GORM、Redis 或 MQ；用户密码哈希使用 bcrypt |
| Application | 用例编排、分页游标、幂等、跨实体流程 | 依赖 Domain 接口 |
| Infrastructure | GORM、Redis、RabbitMQ、JWT、配置 | 实现 Domain/Application 所需接口 |
| Interfaces | HTTP Handler、DTO、路由、中间件 | 调用 Application Service |

模块接入顺序：

1. 定义 Domain 实体、错误和仓储接口。
2. 编写 Application Service。
3. 实现 GORM 模型和 Repository。
4. 编写 HTTP DTO 和 Handler。
5. 在 `router.Register` 装配 `Repository -> Service -> Handler -> Route`。
6. 补充 API 流程测试和模块文档。

## 3. 新增后端模块文件组

```text
internal/domain/{module}/{module}.go
internal/domain/{module}/error.go
internal/domain/{module}/repository.go
internal/application/{module}/service.go
internal/infrastructure/persistence/{module}/model.go
internal/infrastructure/persistence/{module}/repository.go
internal/handler/{module}/dto.go
internal/handler/{module}/handler.go
internal/router/{module}_test.go
docs/modules/{module}.md
```

当前模块体量继续增长时，按职责拆出 `cursor.go`、`worker.go`、`event.go`、`errors.go` 等文件。

## 4. Go 包和命名

包名优先采用所在目录的业务名，例如领域层、应用层和处理器均可使用 `package video`；调用处通过导入别名区分层级，沿用所在模块已有模式，例如持久化视频包仍为 `infravideo`，不为文档统一而修改已有包名。

```go
import (
    applicationvideo "github.com/inumoroha/video-feed/internal/application/video"
    domainvideo "github.com/inumoroha/video-feed/internal/domain/video"
)
```

常用类型命名：

| 类型 | 命名 |
| --- | --- |
| 应用服务 | `Service`，通常 `NewService`，推荐模块沿用 `New` |
| HTTP 入口 | `Handler`，通常 `NewHandler`，推荐模块沿用 `New` |
| 仓储接口 | `Repository` |
| 仓储实现 | `Repository`，通过包名区分 |
| GORM 模型 | `{Entity}Model` |
| 响应转换 | `{xxx}ResponseFromDomain`、`{xxx}ResponseFromResult` |
| 参数解析 | `parse{Field}`、`parsePositiveInt64`、`parseLimit` |
| 错误写入 | `write{Module}Error` |

常量位置：

| 常量类型 | 位置 |
| --- | --- |
| 领域枚举、最大长度 | `domain/{module}/{module}.go` |
| 应用默认值 | `application/{module}/service.go` |
| HTTP query 默认值 | `handler/{module}/handler.go` |

## 5. Domain 规则

Domain 层负责业务不变量和领域表达。

放在 Domain 的内容：

- 实体结构体。
- 状态常量和业务限制。
- 领域错误。
- 领域构造函数，例如 `NewPublished`、`NewComment`、`NewFollow`。
- 数据恢复函数，例如 `RestoreVideo`、`RestoreUserWithStats`。
- 实体方法，例如 `Authenticate`、`UpdateProfile`、`DeleteBy`、`Active`。
- 仓储接口。

领域构造函数负责清理字符串输入、校验 ID、校验必填字段、校验长度限制、设置默认状态和业务时间。读取路径使用 `Restore*` 保留数据库状态并做展示字段清洗。

## 6. Application 规则

Application 层负责用例编排。

放在 Application 的内容：

- `Service`。
- 用例入参结构，例如 `FeedRequest`。
- 用例返回结构，例如 `LoginResult`、`CreateResult`、`CommentListResult`。
- 跨实体流程，例如发布视频时处理幂等键。
- 游标解析和编码。
- 默认分页大小和最大分页裁剪。
- 基础设施能力的最小接口，例如 `TokenSigner`。

Service 依赖 Domain 的 `Repository` 接口，构造函数注入依赖。Redis、RabbitMQ、JWT 这类能力通过小接口注入，便于测试。

## 7. Infrastructure 规则

Infrastructure 层负责外部资源和技术实现。

主要目录：

```text
internal/config/
internal/database/
internal/infrastructure/cache/
internal/infrastructure/mq/
internal/auth/jwt/
internal/infrastructure/persistence/{module}/
internal/infrastructure/persistence/migration/
```

GORM Repository 规则：

- 每个模块独立 `model.go` 和 `repository.go`。
- `model.go` 只定义数据库模型和 `TableName`。
- `repository.go` 实现 Domain Repository 接口。
- 写操作尽量保持事务边界清晰。
- 列表查询使用稳定排序字段和游标。
- 返回 Domain 实体，避免把 GORM 模型泄漏到 Application。

## 8. Interfaces 规则

Interfaces 层负责 HTTP 入口。

Handler 职责：

- 解析 path、query、body 和 header。
- 从鉴权上下文读取用户 ID。
- 调用 Application Service。
- 将结果转换为响应 DTO。
- 将业务错误映射成 HTTP 状态码。

Handler 避免承载业务规则。业务判断放在 Domain 或 Application。

## 9. HTTP API 规范

路径使用资源名，方法表达动作：

| 方法 | 用途 |
| --- | --- |
| `GET` | 查询资源 |
| `POST` | 创建资源或提交复杂查询 |
| `PUT` | 设置确定状态 |
| `PATCH` | 部分更新 |
| `DELETE` | 删除或取消 |

路径约定：

```text
POST   /api/users
POST   /api/sessions
DELETE /api/sessions/current
GET    /api/users/me
PATCH  /api/users/me
POST   /api/videos
GET    /api/videos/{videoId}
GET    /api/feed-items
POST   /api/feed/query
PUT    /api/videos/{videoId}/like
DELETE /api/videos/{videoId}/like
```

状态码约定：

| 状态码 | 场景 |
| --- | --- |
| `200` | 查询、更新、删除成功 |
| `201` | 创建成功 |
| `204` | 退出登录成功，无响应体 |
| `400` | 参数格式或业务输入错误 |
| `401` | 登录态缺失或 Token 异常 |
| `403` | 已登录但权限不足 |
| `404` | 资源缺失 |
| `409` | 幂等冲突或唯一性冲突 |
| `500` | 服务内部错误 |

写接口支持 `Idempotency-Key` 时，按领域常量校验最多 128 个 UTF-8 字节；不能只依赖应用层预查询保证并发安全，仓储需用唯一约束和事务。只在状态真正变化时更新计数。Redis 快速路径和数据库快照重放语义分别见互动模块。注册、资料更新、上传、视频删除和曝光上报不支持此请求头；`request_id` 不是幂等键。

## 10. 分页和游标

列表接口优先使用游标分页。游标内容使用排序字段，编码为 URL-safe 字符串。

常见排序：

| 列表 | 排序 |
| --- | --- |
| Timeline Feed | `published_at DESC, id DESC` |
| 评论列表 | `created_at DESC, id DESC` |
| 关注列表 | `updated_at DESC, target_user_id DESC` |
| 粉丝列表 | `updated_at DESC, user_id DESC` |

返回结构：

```json
{
  "items": [],
  "next_cursor": "",
  "has_more": false
}
```

## 11. 数据库规范

表名使用小写蛇形命名。现有主表为 `users`、`videos`，不得为统一命名重建表。关系和行为表使用业务事实名，例如 `user_follow`、`interaction_action`。

通用字段：

| 字段 | 说明 |
| --- | --- |
| `id` | BIGINT 主键 |
| `created_at` | 创建时间 |
| `updated_at` | 更新时间 |
| `status` | 软状态字段 |
| `idempotency_key` | 写操作幂等键 |

高频计数独立成统计表，例如 `video_stat`、`user_relation_stat`。计数更新与事实写入放在同一事务中完成。缓存计数允许短暂偏差，持久化表保存最终事实。

## 12. 错误处理

Domain 定义明确业务错误：

```go
var ErrInvalidVideoID = errors.New("invalid video id")
```

Application 可以包装跨资源错误，但保留可判断性：

```go
return nil, fmt.Errorf("%w: %d", domainvideo.ErrVideoNotFound, videoID)
```

HTTP 层使用 `errors.Is` 映射状态码。响应保持简洁：

```json
{"error":"invalid request"}
```

## 13. 前端规范

前端位于 `web`，当前保持轻量结构：

```text
web/src/App.jsx
web/src/main.jsx
web/src/style.css
```

规则：

- 组件使用函数组件。
- API 调用集中使用 `apiRequest`。
- 服务端错误显示为用户可理解文案。
- 页面状态保持清楚：loading、error、empty、success。
- CSS class 使用语义命名。
- 页面继续增长时拆为 `src/pages/`、`src/components/`、`src/lib/api.js`。

## 14. 测试规范

后端测试位于 `internal/**` 和 `cmd/worker` 各包内，HTTP 测试集中在 `internal/router`；前端测试在 `web/tests`。新增接口至少覆盖：

- 成功路径。
- 参数错误。
- 鉴权错误。
- 幂等重复请求。
- 游标分页稳定性。
- 关键状态变化和计数变化。

常用命令：

```bash
go test ./...
go vet ./...
go test -race ./...
git diff --check
docker compose config --quiet
```

Web 构建命令：

```bash
npm --prefix web test
npm --prefix web run build
```

OpenSpec 校验命令：

```bash
npx --yes @fission-ai/openspec@1.2.0 validate --all --strict
```

## 15. 文档同步

改动以下内容时同步文档：

| 改动 | 需要同步 |
| --- | --- |
| 新增接口 | `api/openapi.yaml`、`api/debug.http`、`docs/product.md`、`docs/modules/{module}.md` |
| 新增模块 | `docs/modules/README.md`、模块文档、OpenSpec |
| 改目录或分层 | 本文、`docs/quickread.md` |
| 改核心链路 | `docs/architecture.md` |
| 改前端页面 | `docs/uiux.md` |
| 改性能策略 | `docs/optimization.md` |

新增后端能力检查清单：

- Domain 实体、错误和仓储接口完整。
- Application Service 覆盖核心用例。
- Infrastructure Repository 实现接口。
- Handler 完成参数解析和错误映射。
- Router 完成依赖装配。
- API 测试覆盖成功和失败路径。
- 模块文档和产品状态更新。

## 16. 当前仓库必须保留的约定

- 遵守根目录 [AGENTS.md](../AGENTS.md)：新增或修改的函数、方法、结构体（包括测试辅助类型）前必须紧邻中文注释，以名称开头，说明实际职责、并发、事务、重试或资源释放行为。
- Go 代码运行 `gofmt`；仓储调用传递 `context.Context`，使用 `errors.Is` 判断错误，不把底层细节泄露给客户端。输入清理必须保持领域语义，不能对密码等字段统一去空白。
- PostgreSQL 多表写入使用事务，关系与计数使用行锁和唯一约束；重复键判断沿用模块中 GORM 错误及 PostgreSQL `23505` 的实现，不使用 MySQL 错误码。
- 所有模型由 `internal/infrastructure/persistence/migration` 统一迁移。API 和 worker 共用事务级咨询锁及统计补齐，新增模型必须加入此入口。
- 互动 Redis 状态写入保留 WATCH 和事务管道，计数分片使用原子累加；关注流添加、裁剪、续期在事务管道内完成。不得用普通读写替代并发保护。
- MQ 的交换机、队列、路由键、确认、拒绝重入队、取消和关闭语义必须逐项审查。当前没有发布确认、持久化发件箱、退避或自动重连，不得在文档中承诺这些能力。
- `cmd/server` 启动 API，`cmd/worker` 启动三个消费者。配置位于 `configs/`；Compose 六服务、健康检查、数据卷与依赖顺序见 [部署说明](deployment.md)。HTTP 9000、Web 5173、宿主 PostgreSQL 5433、Redis 6379、RabbitMQ 5672/15672。
- Windows 运行竞态测试需要开启 CGO 并将 GCC 加入当前进程 PATH；不能用普通测试通过代替并发语义审计。真实 PostgreSQL/MQ 测试需要专用服务，未提供测试环境时会跳过。
- OpenAPI 使用 `api/openapi.yaml` 的 3.0.3 版本。先实现 Gin 路由再记录接口；逐项核对方法、完整路径、字段、必填与范围、状态码、认证、错误响应及示例，`operationId` 必须唯一，YAML 必须可解析。规划接口仅留在产品与模块文档。
- 提交仅在明确要求时执行。标题使用 `<type>(<scope>): <中文摘要>`，正文使用中文无序列表；PowerShell 多行正文用反引号加 n 换行，不使用字面量反斜杠 n。
- 新增能力交付前检查成功、校验、权限、幂等、游标、软删除、计数、事务失败回滚、并发、外部故障与回退；接口、模型、模块文档、产品状态、部署配置保持一致。
