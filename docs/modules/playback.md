# 播放优化模块

已实现：播放配置读取、公开视频预加载建议、客户端与内部 QoS 上报、Web 播放指标采集。API 定义见 `api/openapi.yaml`，迁移审计见 [migration-b28ef00.md](../migration-b28ef00.md)。

## 接口

| 方法 | 路径 | 认证 | 结果 |
| --- | --- | --- | --- |
| GET | `/api/playback-config` | Bearer JWT | 200 播放配置 |
| GET | `/api/preload-videos` | Bearer JWT | 200 资源列表 |
| POST | `/api/playback-qos-reports` | Bearer JWT | 201 新建，200 幂等重放 |
| POST | `/internal/playback-qos-reports` | X-Internal-Token | 201 新建，200 幂等重放 |

所有接口可能返回 400 参数错误、401 认证失败、500 服务错误，统一正文为 `{"error":"..."}`。内部令牌沿用 `INTERNAL_TOKEN`，未配置时禁止调用。

配置参数 `platform` 默认 Web，`network_type` 默认 DEFAULT；去空白后各最多 16 字节。网络别名 wifi/wi-fi 转换为 WiFi，其余统一大小写。查询顺序为指定平台和网络、同平台 DEFAULT、领域默认值；数据库故障返回错误。默认预加载 3 条、缓冲值 1200 毫秒，数量最多 10。

预加载参数 `current_video_id` 可省略或为 0；指定的视频不存在或未发布时返回最新公开视频。否则按 `published_at DESC, id DESC` 返回严格早于游标的视频。`limit` 省略为 3，显式传入必须为正整数，最多按 10 查询。它是时间线建议，不保证等同于推荐流排序。

QoS 请求字段：`video_id` 必须为正数；`first_frame_ms` 可空且不得为负；`stutter_count` 和 `watch_ms` 非负且缺省为 0。用户接口使用 JWT 身份，忽略正文 user_id；内部接口使用非负 user_id，省略为 0。不检查视频存在性。`Idempotency-Key` 去空白后最多 128 字节，同用户同键返回首次报告；不同用户互不影响，空键不去重。

## 数据与分层

- `internal/domain/playback` 保存领域对象、错误、常量和仓储接口。
- `internal/application/playback` 执行参数校验、配置回退和预加载数量归一化。
- `internal/infrastructure/persistence/playback` 使用 PostgreSQL/GORM 查询 `videos`，写入 `playback_config` 与 `playback_qos_log`。
- `internal/handler/playback` 定义 DTO 和响应映射；`internal/router/playback.go` 注册四条路由。
- `playback_config`：id、platform、network_type、preload_count、buffer_ms、updated_at；平台和网络组合唯一。
- `playback_qos_log`：id、user_id（非空，允许 0）、video_id、first_frame_ms（可空）、stutter_count、watch_ms、idempotency_key（可空）、created_at。唯一索引 `uk_playback_user_idempotency(user_id,idempotency_key)`；保留视频时间、用户视频时间索引。
- 自动迁移复用现有事务和 PostgreSQL 咨询锁；并发上报依赖数据库唯一约束，插入失败回滚后读取首次记录，不依赖进程锁。

## 客户端与已知边界

`web/src/App.jsx` 获取网络配置，配置请求失败回退到默认值；封面与视频元数据按地址去重预加载。隐藏探针在加载完成、失败或四秒超时后移除监听器、计时器与媒体资源。仅活动视频自动播放。

播放器记录首帧耗时、waiting 事件次数和从开始播放到提交的墙钟时长（包含等待），暂停、切换和卸载时上报；每个观看片段生成独立幂等键。未登录不发送质量报告。保留原提交行为：buffer_ms 目前仅获取和归一化，没有动态控制浏览器缓冲；没有自动重试、离页可靠投递或监控聚合。

## 部署与验证

在仓库根目录使用 `docker compose up --build`，API 端口 9000，Web 端口 5173；不新增服务、队列、卷或环境变量。播放模块直接使用数据库，沿用现有健康检查和启动依赖。

运行 `go test ./...`、`go vet ./...`，在 `web` 下运行 `npm test` 和 `npm run build`。真实并发测试需要专用 `VIDEOFEED_TEST_POSTGRES_URL`，执行 `go test ./internal/infrastructure/persistence/playback -run TestPlaybackPostgresLive -v`，测试会创建独立 schema 并在结束时清理。未配置时明确跳过，不能据此宣称真实并发验证通过。
