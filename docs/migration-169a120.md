# RabbitMQ 健康检查超时迁移

来源仓库：`C:\Users\HP\Desktop\GCFeed`。目标仓库：`C:\Users\HP\Desktop\VideoFeed`。
来源提交：`169a120a058fe2ed5a36a9eaf13ec576204fe90c`，fix(compose): 放宽 RabbitMQ 健康检查超时。

## 逐文件迁移表

| 原文件 | 目标文件 | 新建 | 配置、依赖、测试和文档对应 |
| --- | --- | --- | --- |
| apps/docker-compose.yml | docker-compose.yaml | 否 | services.rabbitmq.healthcheck.timeout 从 3s 改为 30s；解析配置检查对应 docker compose config --quiet；说明同步 docs/deployment.md，审计入口同步 docs/migrations.md。 |

已执行 git show --stat、git show --name-only、git show --format=fuller 并阅读唯一文件完整 diff。目标起始 main 工作区干净，沿用已读取的 AGENTS.md；复核主 Compose 的六服务结构、RabbitMQ 健康检查、API/worker 的 service_healthy 依赖及部署文档。

## 逐项审计

- 来源唯一改动为 healthcheck.timeout，已按目标实际文件名映射；再次列出参考提交文件，确认没有其他变更。
- 来源与目标 RabbitMQ healthcheck 解析结果完全一致：CMD rabbitmq-diagnostics -q ping，interval=5s，timeout=30s，retries=20。
- 比较目标修改前后的完整 YAML 对象，唯一差异为上述超时。镜像、连接账户、端口、卷、启动依赖和其他服务探针保留目标配置。
- scripts/compose.exposure-test.yaml 是独立测试部署，包含额外端口连通性探测，未作为主 Compose 的映射目标，本次不修改其独立策略。
- 来源没有 API、类型、函数、常量、测试、数据库、缓存或消息队列客户端代码变更，相关逐符号迁移不适用；消息连接、交换机、队列、路由键、确认/拒绝、重试和关闭逻辑不受本次健康检查超时修改影响。
- 不新增依赖、环境变量或数据卷；HTTP 行为不变，无需修改 OpenAPI。

## 验证结果

- docker compose config --quiet：通过。
- 完整 YAML 对象差异及来源 healthcheck 等价检查：通过。
- go test ./...：通过；未配置外部服务或媒体工具的测试按原有条件跳过。
- go vet ./...：通过。
- git diff --check：通过。
- OpenAPI YAML 解析：通过。
- gofmt：不适用，无 Go 文件变更。

未启动 RabbitMQ 或重建容器，因此本次验证配置语义和可解析性，不宣称已观察真实启动耗时。全部来源改动已迁移，无遗漏。本次变更按配置修复和文档两个主题分别创建 Git 提交。
