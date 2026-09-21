# Day 14：播放优化、上传校验与媒体快启

## 今日目标

在 Day 13 消息中心的基础上，接入播放配置、预加载建议和播放质量上报，补齐上传媒体校验与 MP4/MOV 快启处理，并完成接口测试、前端展示调整及分层验证。

本记录于 2026-09-21 整理，范围为 Day 13 记录提交 `f9474a6` 之后至 `fe3d7da` 的八个来源提交、18 个主题提交。下述结果引用本轮迁移审计与验收证据；本次撰写开发记录不代表重新运行代码测试或真实联调。

## 完成内容

| 来源提交 | 主题 | 当前成果 |
| --- | --- | --- |
| `b28ef00` | 播放优化 | 播放领域、持久化、四条接口、前端预加载与质量采集 |
| `1f452f8` | 播放与静态资源测试 | 接口流程、输入校验、并发幂等及 Range 边界覆盖 |
| `34f6682` | 上传校验与快启 | 媒体白名单、元数据校验、FFmpeg 处理和失败清理 |
| `83ef498` | 实现状态文档 | 同步播放与上传的产品实现状态 |
| `7a254df` | 快启输出修复 | 唯一临时文件后缀、固定编码名单及真实媒体回归 |
| `169a120` | 部署配置 | RabbitMQ 主部署健康检查超时由 3 秒改为 30 秒 |
| `b63bb83` | 播放与评论展示 | 隐藏 Feed 原生视频控制条，评论列表和条目顶部对齐 |
| `57008cb` | 导航顺序 | 最新视频、推荐流、关注流、热门榜单依次排列 |

所有来源按当前仓库结构迁移：后端沿用 domain、application、handler、infrastructure 分层；前端使用 web/src/App.jsx 和实际引入的 web/src/style.css，没有复制旧仓库 apps 目录。

## 关键设计

### 播放接口与配置回退

| 方法 | 路径 | 鉴权与结果 |
| --- | --- | --- |
| GET | `/api/playback-config` | JWT；返回指定平台和网络的播放配置 |
| GET | `/api/preload-videos` | JWT；返回公开视频预加载建议 |
| POST | `/api/playback-qos-reports` | JWT；首次创建 201，同键重放 200 |
| POST | `/internal/playback-qos-reports` | X-Internal-Token；首次创建 201，同键重放 200 |

配置依次查找指定平台和网络、同平台 DEFAULT，均不存在时使用领域默认值：预加载 3 条、缓冲值 1200 毫秒。数据库故障返回错误，不伪装成配置缺失。预加载数量最多 10 条。

预加载按 published_at、id 双字段降序读取严格早于当前视频的已发布作品；当前视频不存在或未发布时回退至最新公开视频。这是时间线资源建议，不保证与推荐流排序相同。

用户质量上报以 JWT 身份为准，忽略正文 user_id；内部接口允许 user_id=0。video_id 必须为正数，首帧耗时可空，首帧、卡顿次数及观看时长均不能为负。接口不检查视频是否存在。四条接口及实际错误状态已同步到 [OpenAPI](../../api/openapi.yaml)。

### PostgreSQL 幂等与架构适配

新增 playback_config、playback_qos_log，接入已有共享迁移事务和 PostgreSQL 咨询锁。查询使用目标实际 videos 表；播放唯一索引命名为 uk_playback_user_idempotency，避免与其他模块同名索引冲突。

同用户同 Idempotency-Key 返回首次报告，不覆盖指标；跨用户同键互不影响，空键保存为 NULL、不去重。仓储通过数据库唯一约束处理并发，插入失败回滚后读取首次记录，并识别 PostgreSQL SQLSTATE 23505。播放质量直接写数据库，没有新增缓存、消息队列或消费者。

### 客户端预加载与质量采集

FeedPage 获取网络配置，失败时使用默认值；按地址去重预加载封面和视频元数据。隐藏媒体探针在成功、失败或四秒超时后释放事件监听器、计时器和媒体资源。

VideoStage 仅播放活动视频，记录首帧耗时、waiting 事件次数和从开始播放至提交的墙钟时长。暂停、切换和卸载时上报，每个观看片段使用稳定幂等键；未登录不发送质量报告。

buffer_ms 当前仅获取和归一化，没有动态控制浏览器缓冲。观看时长包含等待时间；质量上报没有自动重试、离页可靠投递或监控聚合，不将这些能力计入已完成范围。

### 上传校验、快启与失败清理

POST /api/uploads 沿用 JWT、multipart 的 file/kind 字段及原有响应结构。完整请求体上限为 1 GiB，包含表单开销，超限返回 413；空文件或超过类别上限返回 400。

| 校验项 | 当前限制 |
| --- | --- |
| 视频大小 | 最多 512 MiB |
| 封面、头像大小 | 最多 20 MiB |
| 普通文件大小 | 最多 1 GiB，仍受完整请求体上限限制 |
| 视频时长 | 有限正数，最多 600 秒；拒绝 NaN 和无穷值 |
| 视频尺寸 | 每个视频流宽、高均在 1–3840 |
| 视频编码 | h264、h265、hevc、vp8、vp9、av1 |
| 音频编码 | 非空编码须为 aac、mp3、opus、vorbis |

视频和图片按扩展名及嗅探内容类型校验；视频还须经 ffprobe 确认至少一个视频流及合法元数据。MP4/MOV 经 FFmpeg 全流复制和 +faststart 将元数据前置，不重编码；WebM 跳过快启。

快启临时文件在原目录唯一创建，后缀为 .faststart.mp4；实际容器仍显式选择 -f mp4 或 -f mov，保留目标已有 MOV 行为。ffprobe 超时 30 秒，FFmpeg 超时 2 分钟，均继承请求取消。

校验失败返回 400，缺工具、快启失败或落盘失败返回 500。失败清理已上传文件、快启临时文件及 multipart 临时文件，没有自动重试或降级接受未验证媒体。普通 file 保留异常扩展名转 .bin 的规则；成功响应 size 仍为原始上传大小。

运行镜像安装 ffmpeg（含 ffprobe）；本地运行需将两个工具加入 PATH。新增校验收紧了视频、图片及空文件规则，属于客户端可观察的兼容性变化，已同步接口与部署文档。

### 展示与部署调整

Feed 视频隐藏原生控制条，上传预览仍保留 controls。评论列表增加 align-content:start 和 align-items:start，评论条目增加 align-items:start，避免少量评论被剩余高度拉伸。

FEED_SCENES 将最新视频移到首位，保留目标已有 schedule 图标。使用数组首项的场景回退随之变为 timeline；该调整不等于修改全部登录跳转，已登录根路径和登录成功后的推荐流导航沿用既有逻辑。

主 docker-compose.yaml 的 RabbitMQ 健康检查继续使用 rabbitmq-diagnostics -q ping，间隔 5 秒、超时 30 秒、重试 20 次；API/worker 仍等待其健康。独立联调 Compose 的探针策略未随主部署修改。API 端口 9000、Web 端口 5173，无新增服务或数据卷。

## 文件对应与证据

| 能力 | 当前位置或审计 |
| --- | --- |
| 播放领域与应用 | internal/domain/playback、internal/application/playback |
| 播放持久化与路由 | internal/infrastructure/persistence/playback、internal/handler/playback、internal/router/playback.go |
| 播放迁移 | [b28ef00 审计](../migration-b28ef00.md) |
| 接口及 Range 测试 | internal/router/playback_api_test.go、upload_static_range_test.go；[1f452f8 审计](../migration-1f452f8.md) |
| 上传处理及真实媒体测试 | internal/handler/upload；[34f6682 审计](../migration-34f6682.md)、[7a254df 审计](../migration-7a254df.md) |
| 产品状态与部署 | [83ef498 审计](../migration-83ef498.md)、[169a120 审计](../migration-169a120.md) |
| 前端调整及视觉验收 | [b63bb83、57008cb 审计](../migration-b63bb83-57008cb.md) |
| 真实后端联调 | scripts/verify_worker_live.py；[本轮结果](../worker-live-results.json) |

## 验证结果与边界

| 验证 | 本轮结果 |
| --- | --- |
| gofmt | 各次 Go 修改已格式化；纯配置和前端变更不适用 |
| go test ./...、go vet ./... | 通过；未配置外部依赖的条件测试仍可能跳过 |
| 播放接口与 Range 定向竞态测试 | 通过，未发现数据竞争；不是本轮全仓库竞态结论 |
| 前端 npm test、npm run build | 18 项测试及生产构建通过 |
| OpenAPI | YAML 解析通过；播放迁移时核对 42 个唯一 operationId、20 个请求/响应示例 |
| docker compose config --quiet | 通过，仅表示配置有效 |
| git diff --check | 各次迁移及提交差异检查通过 |

真实媒体测试使用本机 FFmpeg/ffprobe 生成 MP4/MOV，确认快启后 moov 位于 mdat 前、元数据仍合法，并验证取消、无效视频及临时文件清理。默认上传处理器的真实视频上传测试通过；这不代表 Linux 容器或反向代理链路已验收。

浏览器先确认实际应用的导航顺序与 /timeline 选中状态；当时默认 API 未启动，主页面显示接口错误。随后使用当前 VideoStage、CommentPanel 和样式构建临时组件页，配合本地视频与长短评论。在 851×679 视口中确认视频实际静音循环播放、controls=false，评论顶部对齐且间距约 8 像素。临时页面和文件已清理；该结果不是浏览器连接真实后端的完整端到端证据。

2026-09-21 的独立后端联调使用隔离 PostgreSQL、Redis、RabbitMQ 和真实 Go API/worker，六项检查通过：API 单独运行无消费者、三类队列积压、并发迁移和消费、时间线索引及幂等表保留、worker 重启恢复、进程退出释放消息连接。脚本退出码为 0，测试容器和卷清理通过。

播放专用 PostgreSQL 并发用例此前因未配置 VIDEOFEED_TEST_POSTGRES_URL 明确跳过。本次 worker 联调没有执行该用例，不能用其结果替代播放同键并发去重验证。Range 测试使用 Gin 和本机临时文件，也不代表部署代理已支持相同边界行为。

## 提交记录

| 主题 | 当前仓库提交 |
| --- | --- |
| 播放基础设施、业务、测试、文档 | b604e90、4b32937、f9be700、41b3def |
| 播放接口与 Range 测试、文档 | e646fca、09cf30d |
| 上传依赖、业务、测试、文档 | 6f36f4e、b466b83、787f271、0a7d9b7 |
| 播放与上传产品状态 | 21858f0 |
| 快启修复、测试、文档 | 16a4bcf、729880a、3d4d642 |
| RabbitMQ 配置与文档 | eb54aa3、57e5743 |
| 前端展示与验收文档 | 6b8e845、fe3d7da |

以上为已完成的本地迁移提交，不包含本篇开发记录，未执行远程推送。本次仅新增 Markdown，核对事实来源、提交、路径、链接和空白错误，不重复运行业务测试。

## 后续计划

- 在专用 PostgreSQL 上执行播放质量并发、唯一约束和首次报告保留测试。
- 完成浏览器连接真实 API 的登录、上传、发布、播放及评论端到端验收，补充更多响应式尺寸。
- 验证 Linux 运行镜像的媒体工具、反向代理 Range、上传请求体与超时配置。
- 明确质量指标口径、上报可靠性和缓冲配置的产品目标，再设计重试、离页投递及监控聚合。
