# 消息中心前端迁移审计

来源：`C:/Users/HP/Desktop/GCFeed`，提交 `0b7a4b35fe4f51562eb0207d7040a97962e332bd`。目标：VideoFeed，起始提交 `5cdacc9`，工作区干净。

## 迁移表

| 原文件 | 新仓库对应文件 | 新建 | API、函数和适配 | 测试与文档 |
| --- | --- | --- | --- | --- |
| `apps/web/src/App.jsx` | `web/src/App.jsx` | 否 | App、AppShell、TopNav、MessagesPage、PageMessage、normalizeRoute、fetchMessages、markMessagesRead、appendMessages、messageIcon、formatBadgeCount；复用 apiRequest、会话和三个现有消息 API | `web/tests/message.test.mjs`；README、消息模块及本审计 |
| `apps/web/src/styles.css` | `web/src/style.css` | 否 | 消息页面、状态、角标和两个断点的全部源规则；移动导航从目标的四列扩成五列 | 前端构建与页面布局检查 |

源提交没有新增依赖、配置、数据库、缓存、消息队列、HTTP 路由、后端类型或测试。已检查目标根 AGENTS.md、前端结构、React/Vite/测试依赖、JWT 会话、Gin 消息路由、PostgreSQL、Redis WATCH/事务、RabbitMQ、Compose 与 Nginx。复用上一批迁移的消息后端和现有 API 代理，不新增基础设施主题空提交。


## 逐函数和状态审计

已执行 `git show --stat`、`git show --name-only`、`git show --format=fuller`，逐个阅读两个文件完整 diff。源提交没有新增导出接口、类型、常量或测试文件。

| 源符号/行为 | 目标核对结果 |
| --- | --- |
| `App`、`unreadCount`、`refreshUnreadCount` | 全局角标、会话/路由变化刷新、未登录清零、401 清零、其他失败保留；传给所有 AppShell；新增 `/messages` 登录保护及页面分支 |
| `AppShell` | 接收角标并传给 TopNav；四个 Feed 场景后追加消息入口、选中态及角标 |
| `TopNav` | 铃铛导航、访客转登录、悬浮角标；保留目标品牌和原首页/上传/账户行为 |
| `MessagesPage` | items、nextCursor、hasMore、state、error、busyID、markingAll 七项状态保留；加载、续页、失败、空列表、单条和全部已读 UI 全覆盖 |
| `loadMessages` | token 缺失导航；20 条分页；追加去重；next_cursor 与 has_more 联合控制；401 清理会话，其他错误保留原列表并显示重试 |
| `markMessageRead` | 已读项不写；PATCH 指定 ID；成功后写本地 read_at 并刷新角标；失败保留状态；finally 释放操作锁 |
| `markAllRead` | 全部已读或无消息时跳过；空数组更新全部消息；保留已有 read_at，新增已读时间；刷新角标；401/普通失败/finally 路径保留 |
| `PageMessage` | 通用图标、标题、可选重试入口；与源函数正文一致 |
| `normalizeRoute` | 增加 `/messages`，保留目标 `/feed` 别名、四个场景及默认跳转 |
| `fetchMessages` | 默认空游标、limit=20、URLSearchParams 编码、Bearer token；正文与源一致 |
| `markMessagesRead` | PATCH `/api/messages`、默认空 message_ids、Bearer token；正文与源一致 |
| `appendMessages` | ID 去重、保留当前顺序、跳过续页内部重复；正文与源一致 |
| `messageIcon` | LIKE/favorite、COMMENT/chat_bubble、FOLLOW/person_add、SYSTEM/campaign、未知/notifications；正文与源一致 |
| `formatBadgeCount` | 非有限/非正数为空，超过99为99+，其余转字符串；正文与源一致 |

六个独立展示/请求/格式化辅助函数进行了去除空白后的正文比对，完全一致。其余组件和回调按上表逐项核对。

## 样式逐项核对

源文件映射到目标实际导入的 `style.css`，未新增无人引用的 `styles.css`。使用 CSS 解析器比较参考提交父版本和目标版本的变更声明：**41 组选择器/媒体断点、163 项属性全部匹配**。

已覆盖 badge-button、nav-badge、floating、sidebar-link 定位及角标对齐、messages-page、messages-header 及标题/描述、messages-actions/count/list-wrap/list、message-item 所有交互态、message-icon/active、message-copy、message-title-row 标题/时间、message-state/unread、messages-more、page-message 图标/标题/按钮，以及 900px 和 560px 下的所有变化。

## 为目标架构补充的适配

1. 源 App 内联导航回调会随角标更新而重建，使消息页 effect 再次请求。目标新增稳定的 `handleNavigate`，避免请求循环；回归测试断言角标返回后消息列表只请求一次。
2. 请求版本与会话身份共同隔离过期响应；组件 effect 清理使旧请求失效。离开页面或更换账号后，旧列表、已读错误和角标结果不会覆盖或清理新会话。没有新增轮询、自动重试或未实现的取消请求保证。
3. ref 锁立即拦截重复点击，列表刷新、分页、单条/全部已读串行；所有成功、普通失败和401路径保留，并在 finally 释放当前操作锁。
4. 移动导航由目标现有四列扩为五列。原 `.sidebar-link span:last-child` 会把新增角标误当文字隐藏，改为只隐藏第二个文字元素，并将移动端角标定位在图标旁。消息入口增加可访问名称。
5. 本次不修改后端 API、OpenAPI 契约、数据库事务、Redis WATCH、RabbitMQ 交换机/队列/路由键/确认拒绝重试关闭逻辑。前端已有 esbuild 与 react-test-renderer 足够验证，不新增依赖或基础设施空提交。

## 测试与验证

参考提交未新增测试，因此没有遗漏的源测试。新增 `web/tests/message.test.mjs` 共 8 项测试，保留现有推荐流 5 项测试：

- 请求方法/路径/Bearer、游标编码、空数组全部已读、ID 去重、四类图标和角标边界。
- 首次加载、500 错误及重试、空列表。
- 分页串行、重复项、续页失败保留列表、手动重试。
- 单条和全部已读互斥、重复点击、500 不乐观修改、成功刷新角标。
- 访客不请求、列表和两种已读操作401均清理会话并导航。
- 会话切换及卸载后旧请求失效，不清理新会话。
- 角标更新不触发列表循环，桌面和侧栏消息入口正确。
- 旧账号角标不覆盖新账号；角标401清零，500保留。

执行结果：

| 检查 | 结果 |
| --- | --- |
| `npm --prefix web test` | 13/13 通过 |
| `npm --prefix web run build` | 通过 |
| `go test ./...` | 通过；真实服务条件测试可能跳过 |
| `go vet ./...` | 通过 |
| `gofmt` | 无新增/修改 Go 文件，不适用；既有 Go 代码未重排 |
| `git diff --check` | 通过 |
| `docker compose config --quiet` | 通过 |
| OpenAPI YAML 解析及 operationId 唯一性 | 通过，38 个唯一标识；消息请求和既有契约一致，无需更改定义 |
| 浏览器布局和操作 | 本地构建产物、模拟 API；默认桌面及 390×844 窄屏已检查。窄屏页面/侧栏均宽390，无水平溢出、五列导航角标可见；单条已读使角标4→3，全部已读清零，分页从20追加到23条 |

浏览器验证使用临时内存接口夹具和非真实账号，不是 PostgreSQL/Redis/RabbitMQ 全链路联调。临时预览不作为生产配置或仓库依赖交付。

## 最终审计

重新列出源提交的全部文件：`apps/web/src/App.jsx`、`apps/web/src/styles.css`，两项均已映射。逐项核对新增/修改函数、状态、请求、样式、响应式断点、错误与清理路径，没有发现未迁移内容。README、模块索引、消息模块、部署说明和迁移索引同步反映真实目录与端口。

按业务逻辑、测试、文档三个主题提交；本提交无基础设施变化，不创建空提交。未推送远程。真实服务端到端联调不在本次验证结论内。
