# 推荐模块

## 1. 模块职责

推荐流由三层组成：`internal/handler/recommendation` 负责三个内部 JSON 接口，`internal/application/recommendation` 负责候选召回、兴趣相似度、热度和新鲜度排序、游标分页及作者打散，`internal/infrastructure/persistence/recommendation` 负责 PostgreSQL 查询和事务写入。

## 2. 接口设计

三个内部接口均为 POST：`/internal/recommendation-candidates`、`/internal/exposure-decisions`、`/internal/exposures`。当前无 JWT/服务鉴权，不支持幂等请求头。

### Feed 场景

`GET /api/feed-items?scene=recommend` 和 `POST /api/feed/query` 的 `scene=recommend` 要求 JWT。Feed 策略把 JWT 用户 ID、`cursor`、`limit` 和 `context.request_id` 传给推荐服务，再复用已有卡片、互动计数缓存和仓储组装响应。推荐页不缓存个性化页、不发布 RabbitMQ 消息；卡片和计数未命中时仍会写回公共 Redis 缓存；卡片或计数缓存失败时回源数据库。

### 曝光接口

`POST /internal/exposures` 接收用户、场景、请求标识和 `video_ids`。批内按视频去重并保留首次顺序，事务中追加 `exposed` 流水，再通过 PostgreSQL `ON CONFLICT` 原子累加 `exposures.exposure_count`。唯一键仍为 `(user_id, video_id)`；相同 `request_id` 跨请求不会去重。视频不存在、未发布或任意数据库错误都会回滚整个批次。

### 曝光决策接口

`POST /internal/exposure-decisions` 接收 `user_id`、`scene`、可选 `request_id` 和 `video_ids`，不使用 JWT。用户和每个视频标识必须大于零；场景去空白并转小写后非空且最多 32 个 UTF-8 字节，请求标识去空白后最多 64 字节。空视频列表仍校验这些元数据，成功返回 `decisions: []`。

响应按输入视频首次出现顺序去重，近七天（含边界）曝光返回 `allowed: false`、`reason: recently_exposed` 和 `last_exposed_at`；其余返回 `allowed: true`、`reason: fresh` 并省略时间。空请求标识省略。查询跨场景，仅按用户隔离，不检查视频存在性或发布状态。窗口复用领域常量，服务时钟仍按分钟截断，与原提交一致。

这是只读判断，不写曝光流水、不修改计数、不预占名额，多个并发请求可以同时获得允许结果。数据库失败返回 500，不隐式重试或放行；调用方可显式重试。实际曝光仍须通过已有上报接口记录。接口为 200/400/500，不新增认证要求或 404 行为。完整规范见 `api/openapi.yaml`；本地 API 地址为 `http://127.0.0.1:9000`。

## 3. 数据表设计

实现复用 `videos`、`video_stat`、`video_embedding`、`video_view_events` 和 `exposures`，不新增配置、环境变量、Compose 服务、数据卷或 RabbitMQ 拓扑。PostgreSQL 表和唯一索引由 API 与 worker 共用的 `migration.AutoMigrate` 负责迁移。

源文档中的 `reco_rule` 属于后续可配置策略规划（id、scene、config_json、enabled、updated_at）；当前排序参数在代码内。`exposure_log` 的流水职责已由 `video_view_events` 承担，去重查询由 `exposures` 承担，不新增重复表。

## 4. 业务规则

候选池只包含已发布视频，排除用户近七天内曝光的视频，按互动热度、发布时间和视频 ID召回。推荐服务读取最近三十天最多二百条 `play`/`complete` 行为，按观看时长和完成标记加权生成兴趣向量。存在用户画像时使用 `0.70 * similarity + 0.20 * log1p(hot_score)/10 + 0.10 * freshness`，冷启动使用 `0.65 * log1p(hot_score)/10 + 0.35 * freshness`。页内游标基于评分、发布时间和视频 ID，在作者打散前生成；同作者相邻候选会延后。

候选只返回上线视频；近期曝光按用户跨场景过滤。作者打散为尽力策略，同作者占满候选池时无法保证全部分离。批量曝光只在批内去重，同一请求标识再次提交仍累加；曝光决策只读不锁定名额。

## 5. 测试建议

`internal/application/recommendation/service_test.go` 覆盖排序、冷启动、打散与分页；`decisions_test.go` 覆盖近期曝光判断。`internal/infrastructure/persistence/recommendation` 覆盖写入流水、重复请求累计、事务失败回滚、并发及跨用户/七天边界。`internal/router/recommendation_test.go` 和 `web/tests` 覆盖 HTTP 与推荐页。

源提交“重复曝光结果稳定”的建议按本仓库实际累计语义校验，不误写成跨请求幂等。

## 6. 前端接入点

`web/src/App.jsx` 的四个导航场景依次为推荐流、最新视频、关注流和热门榜单。`/recommend` 需要登录；已登录用户访问根路径、点击品牌入口或登录成功后进入推荐流，匿名根路径和退出登录仍进入时间线。移动端 `web/src/style.css` 使用四列导航。

推荐页通过 `POST /api/feed/query` 发送 `scene`、`cursor`、`limit: 10` 和 `context.request_id`；其他场景通过 `GET /api/feed-items` 查询。每轮首页加载生成请求标识，所有续页沿用此标识；进入最后三条时预加载下一页，按视频 ID 去重，显示加载中和末尾提示。同一时刻只允许一次续页请求，场景或身份切换、重新加载和卸载后忽略旧响应。

登录用户的当前视频通过 `POST /api/video-view-events` 上报 `exposed`，携带视频、场景、请求标识及 `watch_ms: 0`、`completed: false`。同一轮请求的同一视频只发起一次上报，翻回旧视频不会重复上报；继承源提交语义，上报失败也不自动重试。分页失败保留已有卡片；推荐或关注场景续页、曝光收到 401 时清理登录并转到 `/auth`，首页收到 401 则显示登录提示。

Vite 仍用 5173 端口，`VITE_API_TARGET` 默认 `http://127.0.0.1:9000`；Nginx 已有 `/api/` 代理和单页回退，可直接访问 `/recommend`。前端接入未新增后端路由、数据库表、Redis 操作或 RabbitMQ 拓扑，配置和 Compose 无需变更。

运行 `npm --prefix web test` 执行模拟 HTTP 响应的 React 组件测试，运行 `npm --prefix web run build` 构建生产资源。测试依赖 `react-test-renderer` 和 `esbuild` 仅用于开发，不进入应用运行依赖。
