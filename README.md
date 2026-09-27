# VideoFeed

VideoFeed 是一个面向短视频场景的 Feed 系统工程。项目用 Go API 单体、React Web 客户端、PostgreSQL、Redis 和 RabbitMQ 承载内容供给、分发、消费与互动链路；治理能力仍在规划中。

## 当前状态

已实现能力：

- 后端分层结构：Domain、Application、Infrastructure、Interfaces。
- Gin HTTP 服务入口和 REST API 路由。
- PostgreSQL + GORM 持久化。
- JWT 登录态。
- 消息中心：前端 `/messages` 页面、导航未读角标、游标分页和单条/全部已读，后端支持点赞/评论/关注通知，详见 [消息模块](docs/modules/message.md)。
- Redis Feed 缓存、热榜和互动计数。
- RabbitMQ 异步互动落库、视频发布事件和向量任务。
- 播放配置回退、资源预加载、播放器质量采集和幂等上报，详见 [播放模块](docs/modules/playback.md)。
- 上传视频/图片校验与 MP4/MOV faststart；本地 API 需提供 FFmpeg/ffprobe，容器镜像已包含，详见 [视频模块](docs/modules/video.md)。
- React + Vite Web 客户端。
- Prometheus 指标与 Grafana 看板：API 暴露 `/metrics`，worker 独立 `9091` 指标服务，Compose 提供 Prometheus 与 Grafana，详见 [监控模块](docs/modules/monitoring.md)。
- API 流程测试和 Web 生产构建。

重点待补能力：

- 审核后台。
- 后台运营。
- 系统治理。
- 告警规则与告警事件。

## 快速启动

### Docker Compose

前置依赖：

- Docker
- Docker Compose

启动：

```bash
docker compose up --build
```

后台启动：

```bash
docker compose up -d --build
```

查看日志：

```bash
docker compose logs -f api worker web
```

停止：

```bash
docker compose down
```

清理数据库、Redis 和上传文件数据卷：

```bash
docker compose down -v
```

Compose 启动 `postgres`、`redis`、`rabbitmq`、`api`、`worker`、`web`、`prometheus`、`grafana` 八个服务；健康检查、数据卷和启动依赖见 [部署说明](docs/deployment.md)。

服务地址：

| 服务 | 地址 |
| --- | --- |
| Web | `http://127.0.0.1:5173` |
| API 健康检查 | `http://127.0.0.1:9000/health` |
| API 指标 | `http://127.0.0.1:9000/metrics` |
| PostgreSQL | `127.0.0.1:5433` |
| Redis | `127.0.0.1:6379` |
| RabbitMQ AMQP | `127.0.0.1:5672` |
| RabbitMQ 管理台 | `http://127.0.0.1:15672` |
| Prometheus | `http://127.0.0.1:9090` |
| Grafana 看板 | `http://127.0.0.1:3000/d/videofeed-overview/videofeed-overview` |

Grafana 默认账号密码为 `admin` / `admin`，本地示例值，部署前必须替换。worker 指标服务位于容器内 `worker:9091/metrics`，不对外映射端口；如需本地直连，可在 Compose 中自行添加 `9091:9091` 映射。

### 本地开发

在仓库根目录执行，需要 Bash、Go、Node.js，并按 [部署说明](docs/deployment.md) 配置 PostgreSQL、Redis、RabbitMQ、`DATABASE_URL` 和 `JWT_SECRET`。脚本同时启动 API、worker 与 Web。

```bash
./scripts/start.sh
```

默认地址：

| 服务 | 地址 |
| --- | --- |
| Web | `http://127.0.0.1:5173` |
| API | `http://127.0.0.1:9000` |

## 压力测试

详细步骤见 [docs/performance-testing.md](docs/performance-testing.md)，包括最新流、热门榜单、推荐流、极限 QPS、仓库内 k6 脚本用法和 Grafana 指标解读。

重点查看 `http_reqs` 后面的 `/s`、`http_req_duration p(95)`、`http_req_failed` 和 `feed_success_rate`。

## 文档地图

| 文档 | 用途 |
| --- | --- |
| [docs/product.md](docs/product.md) | 产品范围、模块地图、P0/P1 功能清单 |
| [docs/quickread.md](docs/quickread.md) | 新读者代码阅读路线 |
| [docs/architecture.md](docs/architecture.md) | 系统架构、分层、核心链路、数据模型 |
| [docs/engineering.md](docs/engineering.md) | 工程规范、目录规则、API 风格、测试约定 |
| [docs/optimization.md](docs/optimization.md) | Feed 性能和稳定性专题 |
| [docs/performance-testing.md](docs/performance-testing.md) | k6 压测、QPS/P95 解读、Grafana 指标观察 |
| [docs/interview-questions.md](docs/interview-questions.md) | 围绕本项目的面试问答清单与实现边界说明 |
| [docs/uiux.md](docs/uiux.md) | Web 客户端 UI/UX 规格 |
| [docs/modules/](docs/modules/README.md) | 各业务模块设计 |
| [部署说明](docs/deployment.md) | 本地、Compose、Kubernetes 示例与已知限制 |
| [迁移与联调记录](docs/migrations.md) | 逐文件审计和真实服务验证证据 |
| [openspec/](openspec/project.md) | OpenSpec 项目基线和变更规格 |

## 开发方式

新增功能优先按 OpenSpec 建 change，再按工程规范实现：

```bash
npx --yes @fission-ai/openspec@1.2.0 list
npx --yes @fission-ai/openspec@1.2.0 validate --all --strict
```

新增后端模块时参考 [docs/engineering.md](docs/engineering.md) 的分层模板和 [docs/modules/README.md](docs/modules/README.md) 的模块规格入口。
