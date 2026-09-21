# 播放与上传实现状态文档迁移

来源：`C:\Users\HP\Desktop\GCFeed`，提交 `83ef498643f8ca136eb4b9cad7be94a5cab561c7`（docs: 更新播放和上传实现状态）。目标：`C:\Users\HP\Desktop\VideoFeed`。

## 来源与目标核查

已确认来源仓库，执行 git show --stat、git show --name-only、git show --format=fuller，并逐个阅读全部五份文档的完整 diff（17 行新增、9 行删除）。目标起始分支 main，工作区干净；重新读取 AGENTS.md，并复核目标播放路由、持久化模型、Web Feed 接入及上传文档和测试位置。

本次仅文档变更；配置、依赖、PostgreSQL、Redis、RabbitMQ、HTTP 路由和部署架构沿用先前迁移成果。不存在新增函数、类型、常量、接口实现或测试，因此逐函数/并发/事务/重试/资源释放的代码迁移不适用；没有为了匹配旧文档而修改代码。

## 逐文件与逐变更映射

| 原文件 | 目标文件 | 新建 | 对应与结果 |
| --- | --- | --- | --- |
| README.md | README.md | 否 | 前次已明确消息中心、播放配置/预加载/QoS 接入，待补列表已移除播放优化；保持目标更详细的实现说明，无需重复编辑。 |
| docs/modules/README.md | docs/modules/README.md | 否 | 前次已标为“已实现（配置、预加载、QoS）”；覆盖来源状态更新，无需重复编辑。 |
| docs/modules/playback.md | docs/modules/playback.md | 否 | 四条接口和用户 QoS POST 已存在；补充 user_id 的应用层缺省语义、幂等键 VARCHAR(128) 与精确时间索引；保留目标专用唯一索引名。 |
| docs/modules/video.md | docs/modules/video.md | 否 | 既有“上传校验与快启”已涵盖全部限制；在业务规则和测试章节补齐源提交的媒体校验、MP4/MOV 元数据前置、非法媒体清理和真实快启验证，并使用目标测试路径。 |
| docs/product.md | docs/product.md | 否 | 播放已实现及四条接口前次已同步；本次补齐预加载建议、QoS 上报、Web Feed 接入说明与客户端能力行，并明确内部接口为服务端上报。 |

所有路径保持 VideoFeed 当前目录结构；源提交没有配置、依赖、Compose、数据卷、健康检查和部署顺序变更。新增本审计文档，并将入口加入 docs/migrations.md。

## 与真实实现一致的适配

- 源文档将 user_id 描述为 NOT NULL, DEFAULT 0，但目标 QoSLogModel 只有 not null，没有数据库默认值标签。目标文档明确“内部请求缺省时由应用层传入 0”，不能据此承诺直接 SQL 省略该列仍能成功。
- user_id=0 可表示内部匿名或系统来源；用户接口仍要求 JWT 且使用认证用户，Web 未登录不发送 QoS。
- idempotency_key 为可空 VARCHAR(128)；源 uk_user_idempotency 映射为实际 uk_playback_user_idempotency，避免旧索引名误导。时间索引使用 idx_video_time 和 idx_user_video_time 的真实字段顺序。
- 目标使用 PostgreSQL 与现有时间字段说明，不复制来源的 DATETIME 数据库类型表。
- 上传校验错误 400 与处理工具/快启错误 500 保持区分；失败清理说明以实际已落盘文件为准。

## 审计与验证结果

- 再次读取参考提交文件列表，确认上述五份文档全部有对应内容，无遗漏变更。
- go test ./...：通过（复用有效缓存）；外部服务测试仍受其连接配置约束，未新增真实服务联调。
- go vet ./...：通过。
- gofmt：不适用，本次未修改 Go 文件。
- git diff --check：通过。
- docker compose config --quiet：通过。
- OpenAPI YAML 解析及 42 个 operationId 唯一性：通过；四条播放路由与文档路径一致。没有接口行为变化，OpenAPI 无需修改。
- 修改文档内的相对 Markdown 链接目标检查：通过。

本次无未迁移内容；没有把参考文档中不符合目标实际模型的数据库默认约束照搬。本次变更按文档主题单独创建 Git 提交。
