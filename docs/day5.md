# Day 5 开发记录

## 一、Feed 领域模型扩展

- 增加热榜 Feed 支持，定义 `HotCursor` 和 `HotScore` 字段。
- 增加热度评分函数：`like_count * 3 + comment_count * 5 + favorite_count * 4`。
- 扩展 Feed 仓储接口，支持时间线和热榜查询。

## 二、Feed 应用层完善

- 增加通用 `FeedRequest`、`FeedResult` 和 `Strategy` 抽象。
- 保留时间线策略，同时增加 `HotStrategy` 热榜策略。
- 支持按 `scene` 分发 Feed 查询，默认场景为 `timeline`。
- 增加时间线和热榜游标的解析与编码。
- 完善 nil 服务、nil 仓储、负数分页数量和无效游标校验。
- 修正没有下一页时仍返回 `next_cursor` 的问题。
- 补充应用层测试仓储对 `ListHotFeed` 的适配。

## 三、Feed 持久化实现修复

- 增加热榜查询，按热度、发布时间和视频 ID 稳定排序。
- 对热榜查询补充仓储初始化、分页数量、游标和 context 校验。
- 修正 Feed 查询关联表名，使用实际数据库表 `videos`、`users` 和 `video_stat`。
- 同步更新 SQL mock 测试，确保查询语句与当前数据库模型一致。

## 四、Feed Handler 和路由调整

- 为复杂 Feed 查询 Handler 增加服务初始化保护。
- 增加 `POST /api/feed/query` 路由，支持场景、游标、分页和上下文参数。
- 支持可选 JWT 身份注入，匿名用户仍可访问 Feed。
- 补充请求体分页参数校验和统一错误响应。
- 适配 Handler 测试仓储的热榜接口。

## 五、JWT 中间件修复

- 增加可选 JWT 中间件的 nil 管理器保护。
- 管理器未初始化时，公共 Feed 请求继续按访客请求处理，避免空指针异常。
- 增加对应单元测试。

## 六、前端与文档更新

- 更新 React 前端 Feed、登录、个人资料、关注、评论和上传页面交互。
- 调整前端整体样式、响应式布局和视频 Feed 展示效果。
- 新增项目开发规范文档 `docs/standard.md`。
- 新增 Feed 性能优化方案 `docs/optimization.md`。
- 更新系统架构文档和 Feed 模块设计文档，补充时间线、热榜、游标和数据表说明。

## 七、验证结果

- Go 后端相关测试通过。
- 前端 `npm run build` 构建通过。
- OpenAPI YAML 可正常解析。
- 修复 SQL mock 与实际表结构不一致的问题。
