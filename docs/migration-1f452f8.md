# 播放接口与静态文件 Range 测试迁移审计

来源仓库：`C:\Users\HP\Desktop\GCFeed`。目标仓库：`C:\Users\HP\Desktop\VideoFeed`。
参考提交：`1f452f8ddc4f4492d57a9320f8938955552bdc42`（test(playback): 覆盖播放接口和上传 Range）。

## 提交与目标架构检查

已确认提交位于原仓库，执行 `git show --stat`、`git show --name-only`、`git show --format=fuller` 并逐个阅读两个新增文件的完整 diff（共 372 行）。目标起始分支 main，工作区干净；已读取 AGENTS.md，沿用上一轮核对的配置、数据库、缓存、消息队列和目录架构，并复核当前 JWT、播放处理器、路由注册和静态上传注册。

## 逐文件迁移表

| 原文件 | 目标文件 | 新建 | API、类型、函数、配置及测试文档映射 |
| --- | --- | --- | --- |
| `apps/api/test/playback_api_test.go` | `internal/router/playback_api_test.go` | 是 | 测试及辅助类型同名保留；JWT 改为 internal/auth/jwt 与 config.JWTConfig；处理器改为 internal/handler/playback，调用真实 registerPlaybackRoutes；补齐原测试包外部辅助函数。文档为本记录及 docs/modules/playback.md。 |
| `apps/api/test/upload_static_range_test.go` | `internal/router/upload_static_range_test.go` | 是 | 保留 TestUploadStaticRangeRequest、临时目录、32 字节样本、bytes=0-15 请求及全部断言；Gin Static 与目标注册方式一致；复用同包 requireStatus。文档为本记录及 docs/modules/playback.md。 |

没有新增业务类型、接口、导出常量、数据库字段、事务、唯一约束、依赖、环境变量、Compose、数据卷、健康检查或启动依赖。原提交不包含 Redis 和 RabbitMQ 操作，现有原子事务、消息确认、拒绝、重试和关闭行为均不受影响。内存仓储仅用于 API 测试，不替代生产数据库并发验证。

## 逐类型与逐函数审计

| 原符号 | 目标位置 | 核对结果 |
| --- | --- | --- |
| `playbackConfigAPIResponse` | `internal/router/playback_api_test.go` | 声明、字段或函数体逐项比较一致；补充中文注释。 |
| `preloadVideoAPIResponse` | `internal/router/playback_api_test.go` | 声明、字段或函数体逐项比较一致；补充中文注释。 |
| `preloadVideosAPIResponse` | `internal/router/playback_api_test.go` | 声明、字段或函数体逐项比较一致；补充中文注释。 |
| `qosReportAPIResponse` | `internal/router/playback_api_test.go` | 声明、字段或函数体逐项比较一致；补充中文注释。 |
| `memoryPlaybackRepo` | `internal/router/playback_api_test.go` | 声明、字段或函数体逐项比较一致；补充中文注释。 |
| `memoryPlaybackConfigKey` | `internal/router/playback_api_test.go` | 声明、字段或函数体逐项比较一致；补充中文注释。 |
| `memoryPlaybackReportKey` | `internal/router/playback_api_test.go` | 声明、字段或函数体逐项比较一致；补充中文注释。 |
| `memoryPlaybackVideo` | `internal/router/playback_api_test.go` | 声明、字段或函数体逐项比较一致；补充中文注释。 |
| `newMemoryPlaybackRepo` | `internal/router/playback_api_test.go` | 声明、字段或函数体逐项比较一致；补充中文注释。 |
| `memoryPlaybackRepo.FindConfig` | `internal/router/playback_api_test.go` | 声明、字段或函数体逐项比较一致；补充中文注释。 |
| `memoryPlaybackRepo.ListPreloadVideos` | `internal/router/playback_api_test.go` | 声明、字段或函数体逐项比较一致；补充中文注释。 |
| `memoryPlaybackRepo.CreateQoSReport` | `internal/router/playback_api_test.go` | 声明、字段或函数体逐项比较一致；补充中文注释。 |
| `cloneQoSReport` | `internal/router/playback_api_test.go` | 声明、字段或函数体逐项比较一致；补充中文注释。 |
| `TestPlaybackAPIFlow` | `internal/router/playback_api_test.go` | 声明、字段或函数体逐项比较一致；补充中文注释。 |
| `TestPlaybackAPIValidation` | `internal/router/playback_api_test.go` | 声明、字段或函数体逐项比较一致；补充中文注释。 |
| `newPlaybackRouter` | `internal/router/playback_api_test.go` | 适配配置式 JWT 构造与真实路由注册，四条路径和认证等价。 |
| `performPlaybackJSONRequest` | `internal/router/playback_api_test.go` | 声明、字段或函数体逐项比较一致；补充中文注释。 |
| `performInternalPlaybackRequest` | `internal/router/playback_api_test.go` | 声明、字段或函数体逐项比较一致；补充中文注释。 |
| `TestUploadStaticRangeRequest` | `internal/router/upload_static_range_test.go` | 声明、字段或函数体逐项比较一致；补充中文注释。 |

总计 8 个结构体、11 个函数/方法全部有对应项。其中除 newPlaybackRouter 的明确架构适配外，其余 18 项声明/正文在忽略排版后与来源完全一致；原三个测试的全部请求和断言逐字保留。新增或迁移的所有具名函数、方法及结构体均有紧邻中文注释。

原测试包共享依赖已补齐：testInternalToken 保留示例值；signTestToken 使用当前 SignAccessToken；performJSONRequest 复用带空幂等键的请求辅助函数；decodeJSON 使用 json.Unmarshal；requireStatus 保留状态码与错误正文检查。读取了参考提交中 account_api_test.go、video_api_test.go 的对应辅助实现以确认行为。

## 覆盖范围

- 原 TestPlaybackAPIFlow：Web/WiFi 精确配置、Web/4G 领域默认值、Android/5G 同平台网络回退、103 之后的 102/101 预加载顺序、首帧和卡顿字段、同键重放返回首次首帧及 ID、内部质量上报。
- 原 TestPlaybackAPIValidation：三个用户接口未认证、超长平台、limit=0、非法视频游标、无效 video_id、负首帧/卡顿/观看时长、内部令牌缺失/错误、内部负 user_id。
- 原 TestUploadStaticRangeRequest：206、Content-Range=bytes 0-15/32、精确前 16 字节正文，临时目录由测试自动清理。
- 新 TestPlaybackAPIConcurrentReplay：24 个并发同键请求中恰好一次 201，其余 200；返回同一报告；变更内容不能覆盖首次记录；跨用户同键独立、空键每次创建。互斥锁覆盖查重、分配 ID、存储与索引更新整个临界区。
- 新 TestMemoryPlaybackIsolationAndCursor：配置复制隔离、首帧指针深拷贝隔离、相同发布时间按 ID 降序过滤、不存在游标回退。
- 新 TestUploadStaticRangeBoundaries：完整 GET、尾部范围、开放范围、越界 416、HEAD 无正文及完整 Content-Length。
- 前次已存在的失败重试、配置回退、数据库唯一冲突测试保留，由全量测试运行；此次不重复引入业务实现。

## OpenAPI 与文档

当前四条播放接口与测试使用路径一致；认证、请求字段、成功和失败状态仍与 api/openapi.yaml 一致。此次没有修改接口实现或协议，因此不修改 OpenAPI。静态文件继续使用实际 Gin Static 行为；原提交只有其测试，不新增静态服务接口。

## 验证结果

- gofmt：两个新增 Go 文件格式化完成，复核无未格式化文件。
- go test ./...：通过。
- go vet ./...：通过。
- 新增六个顶层测试及五个 Range 子用例：通过。
- go test -race ./internal/router -run 'TestPlaybackAPI|TestMemoryPlayback|TestUploadStaticRange'：通过，未发现数据竞争。
- git diff --check：通过。
- docker compose config --quiet：通过。
- OpenAPI YAML 解析：通过，42 个 operationId 无重复。

## 完成前再次核对

重新执行参考提交的文件列表查询，仍为上述两个文件；三个来源测试、八个辅助结构体及全部来源函数已逐项核对，未发现遗漏。配置、数据库、缓存、消息队列和前端在该提交中没有新增变更。

此次验证使用真实 Gin HTTP 处理链、当前 JWT 中间件、内存仓储和本机临时静态文件，没有验证真实 PostgreSQL 或部署反向代理的 Range 行为；全量测试中的外部服务用例仍按仓库约定在未配置连接时跳过。
