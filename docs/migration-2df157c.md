# 2df157c 推荐前端迁移审计

来源仓库 `C:/Users/HP/Desktop/GCFeed`，提交 `2df157c54f0c4b22b6865f962e85cb2c10118cdc`（feat(web): 接入推荐流前端）。目标仓库 `C:/Users/HP/Desktop/VideoFeed`，开始时 main 工作区干净，HEAD 为 0d658d1。

已读取 git show --stat、--name-only、--format=fuller，完整阅读以下两个文件的 diff。目标目录、仓库 AGENTS.md 规范、React/Vite 依赖、现有 Gin 查询/曝光路由、配置、代理和移动端导航已核对。

## 文件映射

| 原文件 | 目标文件 | 新建 | API、函数和常量映射 | 依赖/配置适配 | 测试和文档 |
| --- | --- | --- | --- | --- | --- |
| apps/web/src/App.jsx | web/src/App.jsx | 否 | FEED_SCENES、App、LoginPage/handleSubmit、TopNav、FeedPage、normalizeRoute、requiresAuthFeed、createFeedRequestID、fetchFeedPage、appendFeedItems、mapFeedItem | /api/feed-queries 改为 /api/feed/query；context.request_id 保持不变；保留 videofeed 本地存储键与品牌；无需后端依赖变更 | web/tests/recommendation.test.mjs；README.md；docs/modules/recommendation.md、feed.md |
| apps/web/src/styles.css | web/src/style.css | 否 | feed-loading-pill 全部样式属性同值迁入；sidebar 改为四列 | 使用目标单数 style.css，复用现有 900px 移动断点 | 前端构建；推荐模块文档 |

## 逐项行为核对

- FEED_SCENES 增加 recommend、中文标签、/recommend 路径及 auto_awesome 图标；其余场景保留目标命名和图标。
- App 根路径按 token 选择推荐/时间线，登录成功进入推荐，TopNav 按 authenticated 选择首页。注销、其他显式时间线链接保留现状。
- normalizeRoute 通过目标已有 FEED_SCENES 动态判定接纳 /recommend；无需复制源硬编码路径数组。
- FeedPage 保留源新增 nextCursor、hasMore、loadingMore、feedRequestID 状态及 loadingMoreRef、feedRequestIDRef、exposedRef 引用。首页重置、401、错误及空列表行为均迁入。
- loadMoreFeed 保留认证检查、串行请求、游标、同轮请求标识、按 ID 去重、失败保持卡片、401 注销跳转及 finally 释放加载锁。
- 最后三条自动预取、加载中提示、最后一条无后续页时末尾提示均迁入。
- 曝光上报仅对登录用户当前视频发起；scene/request_id/video_id 联合键去重，发送前标记，普通失败不重试；认证失效处理保持源语义。
- requiresAuthFeed、createFeedRequestID、fetchFeedPage、appendFeedItems、mapFeedItem 的职责及所有新增参数/返回字段保留。请求大小为 10；场景、请求标识、分页元数据原样传递。
- 目标适配额外使用 feedGenerationRef 区分加载轮次，并在切换、重新加载及卸载后拒绝旧响应；曝光及续页还检查卡片/请求标识，避免旧 render 向新场景发送请求。不改变正常路径语义。
- CSS 17 行属性全部保留；四列导航适配目标已有关注入口。

## 数据、配置、文档和测试

本提交没有 Go、数据库、Redis、RabbitMQ、配置、Compose 或原始测试文件变更。通过已有认证曝光 API 保存数据，复用现有 PostgreSQL 事务和 MQ 发布行为；前端不直接调用无 JWT 的内部曝光接口。API 请求字段和状态码已对照 api/openapi.yaml，后端契约无变化，因此不修改该文件。

新增 web/tests/recommendation.test.mjs 直接编译实际 App.jsx 并用 React 测试渲染器挂载组件，模拟网络响应。覆盖路径与上下文、路由解析、匿名登录提示、串行预取、ID 去重、曝光去重、过期响应隔离、请求标识延续、失败保留卡片和 401。新增 npm test 命令及测试开发依赖，并同步 package-lock.json。新增或修改的命名函数已补中文注释。

## 验证

- `npm --prefix web test`：5 项组件/请求测试通过。
- `npm --prefix web run build`：生产构建通过。
- `go test ./...`、`go vet ./...`：通过（Go 文件没有变更）。
- `gofmt`：本次没有新增或修改 Go 文件，不适用。
- `git diff --check`、`docker compose config --quiet`：通过。
- OpenAPI YAML 解析、operationId 唯一性及前端调用路径检查：通过。
- 重新列出源提交全部文件，核对命名函数、状态/引用、新增常量入口及 CSS 全部属性：两个源文件均已覆盖，没有发现未迁移的提交行为。
- 限制：组件测试使用模拟 HTTP 响应；未进行真实浏览器视觉验收、真实账号播放或数据库/MQ 端到端联调，因此不声称这些环境已验证。

没有修改数据库数据；按用户要求将依赖、前端功能、测试和文档分别创建本地 Git 提交，未推送远程。
