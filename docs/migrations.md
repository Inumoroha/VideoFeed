# 迁移与真实联调记录

这些文档保存各次提交迁移的文件映射、验证和历史限制；当前功能状态以 [产品范围](product.md) 和 [模块文档](modules/README.md) 为准，历史执行结果不代表本次重新联调。

- [backfill-live-verification](backfill-live-verification.md)
- [exposure-live-verification](exposure-live-verification.md)
- [following-live-integration](following-live-integration.md)
- [migration-1019a792](migration-1019a792.md)
- [migration-1517e5a](migration-1517e5a.md)
- [migration-1d0389](migration-1d0389.md)
- [migration-2df157c](migration-2df157c.md)
- [migration-4272741](migration-4272741.md)
- [migration-7d58cd0](migration-7d58cd0.md)
- [migration-9834644](migration-9834644.md)
- [migration-98d04ee](migration-98d04ee.md)
- [migration-c160f400](migration-c160f400.md)
- [migration-cafc119](migration-cafc119.md)
- [migration-f986e1d](migration-f986e1d.md)
- [migration-fa476283](migration-fa476283.md)

- [7a62b5c：消息中心后端、通知与内部鉴权](migration-7a62b5c.md)

- [0b7a4b3：消息中心前端页面、角标和已读操作](migration-0b7a4b3.md)

- [eb79ab1：同步消息中心产品实现状态](migration-eb79ab1.md)

## 播放优化迁移

播放模块已接入现有 PostgreSQL 自动迁移，无新增服务或环境变量。四条接口、分层适配和验证限制见 [迁移审计](migration-b28ef00.md) 与 [播放模块](modules/playback.md)。

- [1f452f8：播放接口流程、并发幂等与静态文件 Range 测试](migration-1f452f8.md)

- [34f6682：上传媒体校验、快启处理与真实媒体验证](migration-34f6682.md)

- [83ef498：同步播放与上传实现状态文档](migration-83ef498.md)

- [7a254df：快启临时文件格式与固定编码名单测试](migration-7a254df.md)

- [169a120：放宽主部署 RabbitMQ 健康检查超时](migration-169a120.md)

- [b63bb83、57008cb：Feed 播放、评论展示与导航顺序](migration-b63bb83-57008cb.md)

## 简历亮点迁移

简历亮点文档按 VideoFeed 真实实现逐条改写，50 条能力声明的核对结论与边界说明见 [迁移审计](migration-9da1d52.md) 和 [简历亮点](resume-highlights.md)。

- [9da1d52：补充简历项目亮点](migration-9da1d52.md)

## 互动状态与通知展示迁移

Feed 条目新增当前用户点赞收藏状态，点赞、评论、关注通知携带触发者昵称和头像，评论计数在写入后同步缓存；分层映射、PostgreSQL 适配与验证限制见 [迁移审计](migration-2a21685.md)。

- [2a21685：修复互动状态和通知用户展示](migration-2a21685.md)

## 监控迁移

新增 Prometheus 指标模块、API `/metrics` 与 worker `9091` 指标服务，并在 Feed、缓存、消息队列、上传和 worker 埋点；Compose 增加 Prometheus 与 Grafana 及概览看板。命名空间、端口与路径适配见 [迁移审计](migration-2b9eaa5.md) 和 [监控模块](modules/monitoring.md)。

- [2b9eaa5：接入 Prometheus 和 Grafana 监控](migration-2b9eaa5.md)
