# Feed 播放、评论展示与导航顺序迁移

来源仓库：`C:\Users\HP\Desktop\GCFeed`。目标仓库：`C:\Users\HP\Desktop\VideoFeed`。

- `b63bb83bb6980090b329902fdcadc933cb97fb5b`：优化 Feed 播放和评论展示。
- `57008cb76649e4e67aa8ad20d155ed5d9617de81`：调整 Feed 导航顺序，父提交即上一个来源提交。

## 检查与文件映射

迁移前 main 工作区干净。已读取 AGENTS.md，检查目录结构、React/Vite 依赖、Go 配置和依赖、Gin 路由入口及分层架构。后端沿用 PostgreSQL、Redis、RabbitMQ 和独立 worker；本次来源无后端或部署变更。

两个提交均在来源仓库确认，分别执行 git show --stat、git show --name-only、git show --format=fuller，并逐文件阅读完整 diff。

| 提交 | 原文件 | 对应文件 | 新建 | 符号与适配 | 测试、文档位置 |
| --- | --- | --- | --- | --- | --- |
| b63bb83 | apps/web/src/App.jsx | web/src/App.jsx | 否 | VideoStage 移除 Feed 视频 controls；保留相邻中文函数注释 | web/tests/playback.test.mjs；本审计 |
| b63bb83 | apps/web/src/styles.css | web/src/style.css | 否 | .comment-list 增加 align-content:start、align-items:start；.comment-item 增加 align-items:start | 前端构建、完整差异核对；本审计 |
| 57008cb | apps/web/src/App.jsx | web/src/App.jsx | 否 | FEED_SCENES 顺序改为 timeline、recommend、following、hot | 现有前端测试、完整差异核对；本审计 |

样式入口仍由 web/src/main.jsx 引入 style.css，未复制原仓库 styles.css 路径。保留目标已有 timeline 的 schedule 图标；来源此次只调整顺序，没有修改图标。数据库、配置、依赖无需适配，无新增环境变量、Compose、数据卷、健康检查或依赖顺序变更。

## 逐项审计

- 重新列出来源全部文件：b63bb83 为 App.jsx、styles.css；57008cb 仅 App.jsx，与上表一致。
- VideoStage 唯一行为变化为隐藏浏览器原生控制条；活动视频播放、静音、循环、内联播放、预加载、错误处理、暂停/卸载质量上报及清理逻辑保留。上传预览的 controls 保留。
- 评论网格内容及条目向顶部对齐，间距、滚动、最小高度、头像列宽及回复样式保留。
- FEED_SCENES 四项的键、文字、路由和目标已有图标保留，仅交换前两项。导航显示最新视频在前；FeedPage 与 feedSceneFromRoute 使用第一项作为回退，因此默认场景同步为 timeline。normalizeRoute 的现有路径归一化行为保留。
- 来源没有新增或删除导出类型、接口、结构体、函数；修改函数仅 VideoStage，具有紧邻的准确中文注释；修改常量仅 FEED_SCENES。没有遗漏的符号变更。
- 来源没有测试、文档、配置或依赖文件变更。现有播放测试覆盖预加载释放、失败回退、活动视频播放、质量上报和幂等；推荐和消息测试同时通过。本次轻量展示修改未新增重复实现的测试。
- Redis 原子性、RabbitMQ 连接/拓扑/确认/拒绝/重试/关闭、数据库字段/状态/事务/唯一约束均无变更。HTTP 接口未变化，api/openapi.yaml 无需修改并已解析检查。
- 完整文件内容与迁移前版本的预期最小变换比较一致，确认只有上述导航顺序、控制条和三条 CSS 声明变化。

## 验证

- web 目录 npm test：18 项通过，无跳过。
- web 目录 npm run build：通过。
- go test ./...：通过，使用缓存；外部服务条件测试不代表本次真实服务联调。
- go vet ./...：通过。
- docker compose config --quiet：通过。
- api/openapi.yaml 使用 Python yaml.safe_load 解析：通过。
- git diff --check：通过。
- gofmt：不适用，没有 Go 文件变更。

两个来源提交的全部变更已按目标架构迁移，无未迁移项。浏览器视觉验收和隔离真实后端联调结果见下文。本次变更按前端功能和验收文档两个主题分别创建 Git 提交。


## 补充视觉验收与真实后端联调（2026-09-21）

使用本地 Vite 页面和浏览器完成验收：

- 实际应用导航顺序为最新视频、推荐流、关注流、热门榜单；点击最新视频进入 /timeline，选中样式正常。
- 应用页面验收时默认 9000 端口没有 API，显示接口错误；该页面不作为端到端播放成功证据。
- 另建临时组件验收页，直接使用当前 App.jsx 中的 VideoStage、CommentPanel 和 style.css，配合本地生成的 H.264 测试视频及长短评论；未模拟真实后端联通。
- 在 851×679 浏览器视口中目视确认无原生视频控制条，评论紧凑排列于顶部，长评论正常换行、头像顶部对齐。
- 浏览器读取实际视频状态：controls=false、paused=false、readyState=4、currentTime=2.473544、muted=true、loop=true，确认视频实际播放。
- 实际计算样式：评论列表 align-content/start、align-items/start；两条评论 align-items/start。条目顶部约 93.99、146.11 像素，首条高度约 44.12 像素，间距约 8 像素。
- 临时组件页、媒体文件与浏览器标签已清理。没有修改产品代码以配合验收。

真实后端运行 python -B scripts/verify_worker_live.py，连接隔离 Docker PostgreSQL、Redis、RabbitMQ，并启动实际 Go API/worker，六项通过：

1. API 单独启动无队列消费者。
2. API 可发布三类队列积压，不自行消费。
3. 并发迁移及 worker 互动、分发、向量消费成功。
4. 时间线索引和幂等表保留。
5. worker 重启消化积压，API 持续可用。
6. 进程退出释放消息连接。

结果见 [真实联调证据](worker-live-results.json)。脚本退出码为 0，容器和测试卷清理检查通过。本次尚未覆盖连接真实后端的完整浏览器发布、播放与评论端到端流程，也未扩展至全部响应式尺寸。
