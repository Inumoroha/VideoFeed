# [姓名]

**求职意向：Go 后端开发**  |  [电话]  |  [邮箱]  |  [城市]

## 教育背景

**[学校名称]**  |  [学历] · [专业]  |  [入学年月] - [毕业年月]

## 技术经历

- **Go 与服务开发**：在 VideoFeed 项目中使用 Go、Gin、GORM 实现 HTTP 接口和业务逻辑，使用 PostgreSQL 保存业务数据，编写 Go 测试并运行 `go test ./...`、`go vet ./...`。
- **缓存与异步处理**：在项目中使用 Redis 实现 Feed 缓存、互动状态和计数，使用 RabbitMQ 与独立 worker 处理互动落库、视频发布和向量任务。
- **交付与观测**：使用 Docker Compose 启动 API、worker、Web、PostgreSQL、Redis、RabbitMQ、Prometheus、Grafana；通过 Prometheus 指标和 Grafana 看板观察服务，并使用 k6 进行本机压测。

## 项目经历

### VideoFeed · 短视频 Feed 系统

**项目时间：**[按实际填写]  |  **参与方式：**[个人项目或团队项目，按实际填写]

**技术栈：**Go、Gin、GORM、PostgreSQL、Redis、RabbitMQ、React、Docker Compose、Prometheus、Grafana

- 实现时间线、关注、热门、推荐等 Feed 场景；使用游标分页、轻量页缓存、视频卡片与计数缓存，以及批量详情组装。时间线首页缓存未命中时使用 `singleflight` 合并同键回源，并为页缓存 TTL 添加抖动；缓存读取异常时回源 PostgreSQL。
- 将网页发布接入一次 multipart 请求，提交视频、封面及作品资料；使用 `ffprobe` 校验视频元数据，并对 MP4/MOV 执行 `ffmpeg` faststart。封面校验或数据库发布失败时回收本次文件，幂等重放返回已有作品并清理重复上传的文件。
- 对点赞、收藏等互动使用 Redis 状态与计数缓存，交由 RabbitMQ worker 异步落库；消息发布失败时回退同步落库。发布事件驱动关注流分发和 Feed 预热，worker 使用显式 ACK/NACK 处理消息。
- 完成 Compose 八项服务的构建与联调，验证网页注册、登录、真实视频发布、Feed 读取、媒体 Range 请求及监控抓取。使用 k6 在本机 Compose 网络对公开时间线首页进行 20 VU、60 秒测试：当时数据库有 12 位用户、15 个视频，完成 5,880 次请求，平均 **97.96 RPS**、P95 **5.62 ms**、HTTP 失败率 **0%**。该结果仅是小数据量单机基线，不代表系统极限或生产容量。

## 补充经历

[仅填写真实的实习、竞赛、开源贡献或获奖经历；没有则删除本节。]
