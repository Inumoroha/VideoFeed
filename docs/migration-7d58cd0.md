# 关注流前端入口迁移审计

来源：`C:/Users/HP/Desktop/GCFeed`，提交 `7d58cd0b1b75a6fc174a069285a0280e66f09392`（feat(web): 实现关注流入口）。目标为当前 VideoFeed 项目，迁移开始时 main 工作区干净，HEAD 为 9235a1a。

已读取原提交的 git show --stat、--name-only、--format=fuller 和两个修改文件的完整 diff，并重新检查 AGENTS.md、前端目录、依赖、路由归一化、认证存储、API 请求封装、Vite 代理和 Nginx 单页路由回退。后端关注流由上一轮迁移提供。

## 文件及符号映射

| 原文件 | 本项目文件 | 新建 | 符号和行为 | 配置、依赖及接口适配 | 测试和文档 |
| --- | --- | --- | --- | --- | --- |
| apps/web/src/App.jsx | web/src/App.jsx | 否 | FEED_SCENES 添加 following；FeedPage/loadFeed 添加未登录和 401 状态处理；登录提示跳转 /auth；normalizeRoute 接受 /following | normalizeRoute 已按场景表识别路由，无需复制旧路径白名单；沿用 videofeed 存储键和现有 apiRequest | 原提交没有新增测试；执行构建和运行检查；README.md、docs/modules/feed.md |
| apps/web/src/styles.css | web/src/style.css | 否 | 900px 以下 sidebar 改为三等分，容纳最新、关注、热门三个入口 | main.jsx 已引用 style.css；不引入 styles.css，不修改其他双列布局 | 检查移动端导航布局 |

## 逐项行为核对

- FEED_SCENES：following、关注流、/following、subscriptions 四个字段完整保留，顺序位于 timeline 与 hot 之间。
- FeedPage/loadFeed：关注流无 token 时清空滑动状态、视频列表、索引、评论面板与错误，进入 auth，不发起 Feed 请求；返回清理函数。
- 带 token 时继续调用 GET /api/feed-items?scene=following&limit=10；401 且为 following 时清空列表、索引和评论面板，进入 auth；其他异常仍进入错误和重试流程。
- 保留 live 标记及清理函数：依赖变化或卸载后，旧请求成功和失败都不得更新状态；依赖仍包含场景及 token。
- auth 状态显示锁图标、登录后查看关注流、登录按钮；按钮跳转 /auth。
- normalizeRoute 已根据 FEED_SCENES 识别合法场景，因此 /following 直接访问和历史导航均复用现有实现；保留 /feed 等旧路径兼容。
- sidebar 只修改 900px 媒体查询下的列数，其他双列网格不变。
- 原提交没有导出类型、结构体、数据库、Redis、RabbitMQ、环境变量、Compose、依赖或后端接口变更。沿用已有原子性、消息确认、资源释放等实现，不新增其他业务能力。
- OpenAPI 已包含 following 参数和 401 响应；本次只增加前端页面路径，无新 HTTP API，无需添加 /following 到 OpenAPI。
- 修改的 FeedPage 和 loadFeed 保留紧邻的中文职责注释；normalizeRoute 不需要修改。

## 验证记录

审计日期：2026-09-18。重新列出并逐项核对源提交的两个文件，所有新增入口、函数分支、常量字段、界面提示和 CSS 变更均已映射，未发现遗漏。

| 检查 | 结果 |
| --- | --- |
| npm run build（web 目录） | 通过，Vite 构建成功 |
| go test ./... | 通过 |
| go vet ./... | 通过 |
| git diff --check | 通过 |
| docker compose config --quiet | 通过 |
| OpenAPI YAML 解析及关注流契约检查 | 通过，两个既有 Feed 入口已描述 following 与 401，无接口变化 |
| gofmt | 不适用，本次无 Go 文件变更 |
| 实际浏览器检查 | /following 直接访问显示登录提示，登录按钮进入 /auth，浏览器返回后恢复关注流；390px 宽度下三个底栏按钮各约 114px，三等分布局成立 |
| 实际回调隔离执行 | 在 Node 中执行源码中的 loadFeed，验证匿名不发请求及清空状态、携带令牌成功请求、关注流 401、一般异常、timeline 原错误分支，以及清理后成功/失败响应均不更新状态 |
| 路由隔离执行 | normalizeRoute 接受 /following、/timeline、/hotfeed，保留 /feed 和未知路径回退 |
| 注释核对 | 两个修改函数 FeedPage、loadFeed 均有紧邻中文注释；没有新增结构体 |

浏览器使用本次临时 Vite 服务 localhost:5174；该端口仅用于验证，没有修改默认 5173 配置。测试后关闭临时页面、恢复视口并停止临时服务。原提交没有测试文件，当前前端没有测试脚本；本次采用构建、实际页面检查及一次性回调隔离检查，不新增测试依赖或伪造测试覆盖。

验证限制：未使用真实登录账号和后端数据进行浏览器端到端联调；401、成功请求和过期响应通过受控请求隔离执行验证，不等同于真实后端联调。没有新增分页、自动回到关注页或其他原提交未实现的行为。

## 最终文件审计与提交拆分

- apps/web/src/App.jsx → web/src/App.jsx：已迁移；normalizeRoute 的场景表机制复用现状，无需退回旧白名单实现。
- apps/web/src/styles.css → web/src/style.css：已迁移；只修改移动端导航列数。
- README.md、docs/modules/feed.md、docs/migration-7d58cd0.md：补充真实前端路径、行为、构建命令和审计记录。

按前端功能和文档两个主题提交。原提交没有基础设施、依赖或测试文件变更，不创建空主题提交。
