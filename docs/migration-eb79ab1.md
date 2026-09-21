# 消息中心实现状态迁移审计

来源：`C:/Users/HP/Desktop/GCFeed`，提交 `eb79ab109f24286d978c5d519ae9c7e90829c8ea`，标题为 `docs(message): 更新消息中心实现状态`。目标：VideoFeed，起始提交 `0d99cee`，当前分支 `main`，迁移前工作区干净。

## 完整变更与文件映射

已确认提交存在于原仓库，执行 `git show --stat`、`git show --name-only`、`git show --format=fuller`，并阅读三个文件完整 diff。源提交共三个文件，新增七行、删除八行。

| 原文件 | 目标文件 | 新建 | 迁移结果 |
| --- | --- | --- | --- |
| `README.md` | `README.md` | 否 | 删除“重点待补能力”中的消息中心已由前两批迁移完成；当前还列出实际实现的前后端能力，本次核对后复用 |
| `docs/modules/README.md` | `docs/modules/README.md` | 否 | 消息模块已在 `0b7a4b3` 前端迁移中标记“已实现”，本次核对后复用 |
| `docs/product.md` | `docs/product.md` | 否 | 完整迁移消息模块状态和说明、四个接口状态、消息页状态和说明 |

源提交无函数、结构体、类型、接口、常量、测试、配置、依赖、数据库、缓存、消息队列或部署文件变更，不需要新建适配层或迁移运行逻辑。补充本审计及 `docs/migrations.md` 索引。

## 逐项核对

| 项目 | 文档结果与实际证据 |
| --- | --- |
| 消息模块 | 已实现站内通知、未读状态、批量已读和内部事件生成消息，见 `internal/application/message` 与消息模块文档 |
| GET `/api/messages` | 已实现；`internal/router/message.go` 注册，`api/openapi.yaml` 已定义 |
| GET `/api/message-stats/unread` | 已实现；同上 |
| PATCH `/api/messages` | 已实现；同上 |
| POST `/internal/messages` | 已实现；同上，由独立内部令牌鉴权保护 |
| 消息页 | 已实现通知展示、未读状态和已读操作，见 `web/src/App.jsx` 中 `MessagesPage`、`fetchMessages`、`markMessagesRead` 和 `/messages` 路由 |
| README 待补清单 | 不再列出消息中心，已有详细能力描述 |
| 模块索引 | 消息模块状态为“已实现” |

目标产品文档另有两处旧描述与上述实现矛盾，已同步修正：产品定位中的“后续补齐消息”，以及实现边界中的“当前没有消息中心”。保留目标 VideoFeed 名称、现有目录、文本向量模块、原里程碑规划和其他业务状态，不复制原仓库的旧目录或名称。

已检查根 `AGENTS.md`、文档与前后端结构、当前分支和状态，复用前两轮核实的配置加载、PostgreSQL/GORM、Redis WATCH/事务和 RabbitMQ 架构。本次只更新实现状态，没有改变错误处理、幂等、重试、资源释放、并发控制或失败回退。OpenAPI 已记录实际接口，核对后无需修改。

## 验证结果

- `go test ./...`：通过；条件式真实服务测试可能跳过，不代表重新完成服务联调。
- `go vet ./...`：通过。
- `git diff --check`：通过。
- `docker compose config --quiet`：通过。
- OpenAPI YAML 解析、operationId 唯一性及四个消息接口与 Gin 路由对照：通过。
- 源产品文档六项新增内容逐行对照、README 和模块索引等价内容、变更文档本地链接检查：通过。
- `gofmt`：本次无 Go 变更，不适用；无新增或修改函数、结构体，注释规范无需额外变更。
- 未新增测试或重跑浏览器联调：本次仅修改 Markdown，实现与前两轮验证时一致。

## 最终审计

参考提交全部文件重新枚举为 `README.md`、`docs/modules/README.md`、`docs/product.md`，均有对应迁移结果；前两个文件已有等价内容，第三个文件六项状态与说明变更全部落实。没有发现尚未迁移内容。

本次只有文档主题，创建一个中文 Conventional Commits 文档提交，不创建基础设施、业务或测试空提交；不推送远程。
