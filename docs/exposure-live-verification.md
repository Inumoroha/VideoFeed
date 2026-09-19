# 曝光迁移真实服务验证

## 结论

2026-09-19 已完成本次迁移约定的真实服务验证场景，全部通过，未跳过。使用独立 Compose 项目、临时宿主端口和专用测试卷；实际编译并启动 `cmd/server` 应用进程，通过 TCP HTTP 请求完成注册、登录和上报。未使用模拟数据库、内存消息代理或 HTTP 测试替身。

机器可读执行结果见 [exposure-live-results.json](exposure-live-results.json)。本次没有修改业务实现或现有接口行为，验证阶段未创建 Git 提交，随后按基础设施、业务逻辑、测试、文档分组提交。

## 环境与执行

- 执行时间：2026-09-19T20:09:50+0800 至 2026-09-19T20:10:43+0800。
- go：go version go1.26.0 windows/amd64。
- docker：29.6.2。
- postgres：17.11。
- rabbitmq：3.13.7。
- redis：Redis server v=7.4.11 sha=00000000:0 malloc=jemalloc-5.3.0 bits=64 build=40ff01a501d8e4b6。

在仓库根目录运行：

```powershell
python scripts/verify_exposure_live.py
```

需要本机 Docker 引擎、Go 和 Python 3.11 或以上。脚本只依赖 Python 标准库，使用 `scripts/compose.exposure-test.yaml` 的 PostgreSQL 17、RabbitMQ 3.13 和 Redis 7.4 镜像；独立测试账号仅用于临时服务。镜像缺失时 Docker 会拉取。

脚本创建随机项目名，自动发现端口，设置专用连接后实际执行：

```powershell
go test -count=1 -v ./internal/infrastructure/persistence/exposure ./internal/infrastructure/mq -run 'TestExposure(Postgres|Queue)Live'
```

确认两项测试均为 PASS 而非 SKIP 后，编译实际应用并运行以下 HTTP 联调。可用 `--output <相对或绝对路径>` 指定 JSON 结果路径。正常或异常退出均尝试停止本次 API 进程、删除本次容器、网络与数据卷；输出包含最终清理状态。

## 已通过场景

| 范围 | 实际验证结果 |
| --- | --- |
| PostgreSQL 并发 | 32 个并发事务累加为 32；同 request_id 再次上报后为 33；首次曝光时间不变 |
| PostgreSQL 事务与约束 | 原子 ON CONFLICT 更新生效；聚合约束失败导致先插入的流水回滚；重复用户视频插入触发唯一约束 |
| PostgreSQL 迁移 | feed_inbox、videos、video_view_events 和 exposures 索引共存；隔离 schema 测试后删除 |
| RabbitMQ 发布 | 持久化 topic/队列/绑定正确；JSON、delivery_mode、message_id、时间属性正确 |
| RabbitMQ 消费 | 首次处理失败重新入队，第二次成功确认；非法 JSON 丢弃；取消上下文后消费者数量归零，队列无待处理消息 |
| RabbitMQ 资源释放 | Close 后连接和两个通道均关闭，后续发布/消费返回错误；交换机类型冲突导致初始化失败，未遗留连接或拓扑 |
| 真实 API 启动 | 使用 cmd/server、完整路由装配、真实表迁移、注册、登录签发 JWT、发布视频 |
| 真实 HTTP 并发 | 首次曝光加 32 次并发上报得到 33 条曝光流水及聚合计数 33；重复 request_id 未被错误去重 |
| 观看行为 | exposed/play/complete/skip 均通过；complete 自动置 completed=true，非曝光不生成聚合；请求 user_id 无法覆盖 JWT 身份 |
| HTTP/数据库/消息一致性 | 初始 36 条流水对应 36 条消息，逐字段核对用户、视频、场景、请求编号、事件类型、时长、完播、落库时间及曝光次数 |
| HTTP 错误 | 未认证及非法令牌 401，参数错误 400，视频缺失或非发布状态 404；失败请求不保存也不发布 |
| HTTP 存储故障 | 在专用表施加聚合约束错误，上报返回通用 500；流水和聚合回滚，不发布消息、不泄漏数据库错误 |
| 消息运行时故障 | 停止真实 RabbitMQ 容器后，上报仍返回 201 且数据库已保存 |
| 消息启动时故障 | RabbitMQ 停机期间重启 API，日志确认发布器禁用，上报仍保存并返回 201 |
| 持久化与恢复 | RabbitMQ 重启后，停机前未消费的持久化消息仍在；故障期间两条流水保留且没有虚构消息；重启 API 后发布恢复 |
| 最终数据与清理 | 40 条观看流水、曝光计数 33；API 退出后 AMQP 连接归零，测试容器、卷、网络均清理 |

## 测试环境修正

首次自动化执行发现 RabbitMQ 节点 ping 成功不代表 AMQP 已可用。隔离 Compose 健康检查增加端口连通性检查，脚本额外等待 AMQP/管理监听就绪。容器使用动态宿主端口，停止后重新启动可能重新分配端口，因此故障恢复阶段重新发现端口，并等待虚拟主机健康。修正的是测试环境就绪与地址发现逻辑，没有调整业务协议或自动重连语义。

## 验证边界

应用保留参考提交的行为：数据库提交后尽力发布，消息故障期间可能没有对应消息，且恢复 RabbitMQ 本身不会让已经禁用的发布器自动重连；重启 API 后恢复。此次明确验证了该回退行为，不将其描述为可靠必达或零消息丢失。

“全部通过”仅指上表约定场景，不涵盖生产容量、集群网络分区、任意断电时刻或长期稳定性。当前生产 Compose 自定义消息账号的部署前提不因本测试改变；测试使用专用账号和独立部署。

## 补充验证结果

- `go test ./...`：通过。
- `go vet ./...`：通过。
- `gofmt`：本次修改的 Go 测试已格式化，检查无输出。
- `git diff --check`：通过。
- `docker compose config --quiet` 与 `docker compose -f scripts/compose.exposure-test.yaml config --quiet`：均通过。
- Python 脚本语法、函数中文注释、OpenAPI/测试 Compose YAML 解析、结果 JSON 状态检查：通过。
- `go test -race`：在隔离 Linux 容器中使用 Go 1.25.14、GCC 14.2.0 对 application/exposure、domain/exposure、handler/exposure、infrastructure/persistence/exposure、infrastructure/mq、router 六个包运行，全部通过。该轮竞态检查未设置真实服务连接，真实服务测试在前述 Windows 原生 Go 1.26.0 轮次单独执行并通过；不将两者混为同一次检测。

竞态检查使用的等价命令（将宿主模块缓存路径调整为本机 `go env GOPATH` 下的 pkg/mod）：

```powershell
docker run --rm --name videofeed-exposure-race --mount type=bind,source=C:/Users/HP/Desktop/VideoFeed,target=/workspace,readonly --mount type=bind,source=C:/Users/HP/go/pkg/mod,target=/go/pkg/mod --workdir /workspace --env CGO_ENABLED=1 golang:1.25 go test -race ./internal/application/exposure ./internal/domain/exposure ./internal/handler/exposure ./internal/infrastructure/persistence/exposure ./internal/infrastructure/mq ./internal/router
```

宿主 Windows 仍未安装 GCC；此次通过容器完成竞态检测，不修改宿主编译器配置。竞态容器自动删除。最终再次确认真实服务测试项目无剩余容器、卷或网络，原项目数据未被修改。
