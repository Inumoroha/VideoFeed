# 上传校验与快启迁移审计

参考仓库：`C:\Users\HP\Desktop\GCFeed`。目标：`C:\Users\HP\Desktop\VideoFeed`。
参考提交：`34f66822d842cbcd3fe113b9a9a180a4b04fced5`，feat(upload): 增加视频校验和 faststart。

## 来源与架构核对

已执行 git show --stat、git show --name-only、git show --format=fuller，并逐个阅读四个文件的完整 diff（531 行新增、7 行删除）。目标分支 main，开始迁移时工作区干净；重新读取 AGENTS.md。核对了根目录 Dockerfile、上传 Handler/DTO/测试、JWT 路由、OpenAPI、Compose 和部署配置。

目标结构为 internal/handler/upload，构造器为 NewHandler，DTO 独立放置；API 9000、Web 5173，API/worker 共用运行镜像。数据库 PostgreSQL、Redis 缓存和 RabbitMQ 拓扑沿用此前迁移架构，本提交只影响本地上传与外部媒体命令，没有数据库、事务、缓存原子性或消息确认/拒绝/重试变更。

## 逐文件迁移表

| 原文件 | 目标文件 | 新建 | API、依赖、配置、测试与文档对应 |
| --- | --- | --- | --- |
| apps/api/Dockerfile | Dockerfile | 否 | 运行层安装 ffmpeg（含 ffprobe），保留目标构建入口、二进制名、配置目录和端口；Compose 卷、服务顺序及健康检查无需变更；部署文档记录本地 PATH 要求。 |
| apps/api/internal/interfaces/http/upload/handler.go | internal/handler/upload/handler.go；原有 dto.go | 否 | New → NewHandler；NewWithProcessor、UploadProcessor 及全部领域无关校验/处理函数保留。uploadResponse 复用 dto.go，路径、认证及 JSON 字段不变。测试见下述三个处理器测试文件和路由测试；OpenAPI 与视频模块文档同步。 |
| apps/api/internal/interfaces/http/upload/handler_test.go | internal/handler/upload/metadata_test.go | 是 | 保留 TestValidateVideoMetadata 所有正常、超时长、超尺寸及不支持编码断言，避免覆盖目标已有 handler_test.go。 |
| apps/api/test/upload_api_test.go | internal/router/upload_api_test.go | 是 | 使用目标 upload 包及 NewWithProcessor；同名测试、桩处理器、样本与请求辅助函数全部保留；复用已迁移的 decodeJSON/requireStatus。 |

## 逐符号审计

以下清单覆盖源 Handler 文件全部 47 个顶层声明，包含未改变但被依赖的 DTO、常量和辅助函数；核对代码正文而非仅检查符号存在。43 项在构造器命名映射并忽略排版/注释后完全一致，4 项明确适配如下。

| 原符号 | 目标符号 | 审计结果 |
| --- | --- | --- |
| `maxUploadBytes` | `maxUploadBytes` | 正文一致（忽略注释与排版）。 |
| `maxVideoBytes` | `maxVideoBytes` | 正文一致（忽略注释与排版）。 |
| `maxImageBytes` | `maxImageBytes` | 正文一致（忽略注释与排版）。 |
| `maxVideoDurationSeconds` | `maxVideoDurationSeconds` | 正文一致（忽略注释与排版）。 |
| `maxVideoDimension` | `maxVideoDimension` | 正文一致（忽略注释与排版）。 |
| `sniffBytes` | `sniffBytes` | 正文一致（忽略注释与排版）。 |
| `allowedVideoExt` | `allowedVideoExt` | 正文一致（忽略注释与排版）。 |
| `allowedImageExt` | `allowedImageExt` | 正文一致（忽略注释与排版）。 |
| `allowedVideoMIME` | `allowedVideoMIME` | 正文一致（忽略注释与排版）。 |
| `allowedImageMIME` | `allowedImageMIME` | 正文一致（忽略注释与排版）。 |
| `allowedVideoCodec` | `allowedVideoCodec` | 正文一致（忽略注释与排版）。 |
| `allowedAudioCodec` | `allowedAudioCodec` | 正文一致（忽略注释与排版）。 |
| `errInvalidUploadExtension` | `errInvalidUploadExtension` | 正文一致（忽略注释与排版）。 |
| `errInvalidUploadMIME` | `errInvalidUploadMIME` | 正文一致（忽略注释与排版）。 |
| `errUploadTooLarge` | `errUploadTooLarge` | 正文一致（忽略注释与排版）。 |
| `errVideoTooLong` | `errVideoTooLong` | 正文一致（忽略注释与排版）。 |
| `errVideoTooLargeDimension` | `errVideoTooLargeDimension` | 正文一致（忽略注释与排版）。 |
| `errUnsupportedVideoCodec` | `errUnsupportedVideoCodec` | 正文一致（忽略注释与排版）。 |
| `errInvalidVideoMetadata` | `errInvalidVideoMetadata` | 正文一致（忽略注释与排版）。 |
| `errVideoToolUnavailable` | `errVideoToolUnavailable` | 正文一致（忽略注释与排版）。 |
| `errFaststartFailed` | `errFaststartFailed` | 正文一致（忽略注释与排版）。 |
| `UploadProcessor` | `UploadProcessor` | 正文一致（忽略注释与排版）。 |
| `Handler` | `Handler` | 正文一致（忽略注释与排版）。 |
| `uploadResponse` | `uploadResponse` | 正文一致（忽略注释与排版）。 |
| `New` | `NewHandler` | 按目标构造函数命名映射，行为一致。 |
| `NewWithProcessor` | `NewWithProcessor` | 正文一致（忽略注释与排版）。 |
| `*Handler.Create` | `*Handler.Create` | 保留目标根目录/413 检查和扩展名清理入口；加入源视频处理顺序及失败删除，并补充落盘失败和表单临时文件清理。 |
| `normalizeKind` | `normalizeKind` | 正文一致（忽略注释与排版）。 |
| `uploadValidation` | `uploadValidation` | 正文一致（忽略注释与排版）。 |
| `validateUploadFile` | `validateUploadFile` | 普通 file 使用目标 normalizeExtension；视频/图片的源校验流程保持一致。 |
| `validateUploadSize` | `validateUploadSize` | 正文一致（忽略注释与排版）。 |
| `validateUploadExtension` | `validateUploadExtension` | 正文一致（忽略注释与排版）。 |
| `sniffUploadMIME` | `sniffUploadMIME` | 正文一致（忽略注释与排版）。 |
| `validateUploadMIME` | `validateUploadMIME` | 正文一致（忽略注释与排版）。 |
| `shouldFaststart` | `shouldFaststart` | 正文一致（忽略注释与排版）。 |
| `defaultUploadProcessor` | `defaultUploadProcessor` | 正文一致（忽略注释与排版）。 |
| `defaultUploadProcessor.ValidateVideo` | `defaultUploadProcessor.ValidateVideo` | 正文一致（忽略注释与排版）。 |
| `defaultUploadProcessor.Faststart` | `defaultUploadProcessor.Faststart` | 正文一致（忽略注释与排版）。 |
| `probeResult` | `probeResult` | 正文一致（忽略注释与排版）。 |
| `probeStream` | `probeStream` | 正文一致（忽略注释与排版）。 |
| `probeFormat` | `probeFormat` | 正文一致（忽略注释与排版）。 |
| `probeVideo` | `probeVideo` | 正文一致（忽略注释与排版）。 |
| `validateVideoMetadata` | `validateVideoMetadata` | 保留全部源校验，额外拒绝 NaN 和无穷时长。 |
| `faststartVideo` | `faststartVideo` | 保留同目录临时文件、流复制、全流映射、超时、取消、错误及重命名；额外显式指定 mp4/mov 输出格式。 |
| `writeUploadValidationError` | `writeUploadValidationError` | 正文一致（忽略注释与排版）。 |
| `writeUploadProcessingError` | `writeUploadProcessingError` | 正文一致（忽略注释与排版）。 |
| `randomSuffix` | `randomSuffix` | 正文一致（忽略注释与排版）。 |

接口方法 ValidateVideo/Faststart 的参数和返回类型不变。新文件函数与结构体及修改函数均有紧邻中文注释。

## 原测试与补充测试

源测试声明逐项正文比较一致（仅包路径、导入别名和中文注释适配）：

- `TestValidateVideoMetadata`
- `stubUploadProcessor`
- `*stubUploadProcessor.ValidateVideo`
- `*stubUploadProcessor.Faststart`
- `TestUploadVideoValidationAndFaststart`
- `TestUploadValidationRejectsBadFiles`
- `newUploadRouter`
- `performMultipartUpload`
- `sampleMP4Bytes`
- `samplePNGBytes`

- `internal/handler/upload/handler_test.go` 保留目标根目录错误、无效请求、默认 kind 与扩展名清理覆盖。原 TestCreate 使用伪造文本视频，不符合新规则，改以普通 file 验证原有原样落盘/大小/地址行为；真实视频和来源桩测试独立覆盖新增视频链路。
- `internal/handler/upload/validation_test.go`：大小边界无需分配大文件；内容与扩展名白名单、空文件、全部编码、非法流和非有限时长；校验/快启错误映射、失败删除、再次上传成功、multipart 磁盘临时文件释放；缺失工具与默认处理器回退。
- `internal/handler/upload/processor_live_test.go`：使用真实 FFmpeg 生成 MP4/MOV，验证原文件 moov 位于 mdat 后，快启后前移，结果继续通过 ffprobe 校验；验证取消、无效视频和临时文件清理；用默认 Handler 完整上传真实视频并核对原始 size 响应。缺工具时明确跳过，本次临时工具运行实际通过。
- 原 `internal/router` 上传认证与 Range 回归测试继续通过。没有引入上传幂等键或自动重试；失败后可由客户端重新上传。

## 明确适配与修复

1. 目标 NewHandler/root 检查、总请求体超限 413、普通文件扩展名清理行为保留。类型为空仍按 file，媒体文件新增限制符合源提交意图；所有文件为空时现在返回 400。
2. 源 ffmpeg 输出路径后缀为 .faststart，未指定容器，实际工具无法推断格式。迁移显式传入 `-f mp4` 或 `-f mov`，保留临时文件命名、全流映射、流复制和 +faststart；这属于使原有快启意图可执行的修复。
3. 源 ParseFloat 能解析 NaN，而原比较不能拒绝它；迁移补充有限值校验，覆盖 NaN/+Inf/-Inf。
4. 增加 multipart 临时文件和落盘失败残留文件清理；保留源处理失败删除目标文件、探测文件 Close、命令上下文取消、快启临时文件 defer Remove。
5. 全部源白名单、大小/时长/尺寸常量、错误字符串、响应字段与媒体校验顺序保留。不重编码、不新建消息队列，不为配置或 OpenAPI 发明未实现能力。

## OpenAPI 与兼容性

/api/uploads 仍为 POST、JWT 认证、multipart file 必填。同步 201/400/401/413/500，补充成功响应及各种错误示例，复用 ErrorResponse；kind 描述实际忽略大小写与去空白行为。size 是上传原始字节数，非快启后的大小。

文件限制为 video 512 MiB、cover/avatar 20 MiB、file 1 GiB；请求体总限制 1 GiB 含 multipart 开销。视频/图片大小超限为 400，请求体超限为 413。图片仅文件头探测而非完整解码；扩展名与 MIME 分别检查，不强制一一匹配。

新增媒体和空文件校验是客户端可观察的收紧，在 OpenAPI、README、docs/modules/video.md 与 docs/deployment.md 已说明。Compose 配置、数据卷、依赖顺序和健康检查没有源变更，不新增 Go 依赖或环境变量。

## 验证结果

- gofmt：全部新增/修改 Go 文件完成格式化。
- go test ./...：通过；测试进程 PATH 临时加入媒体工具，真实媒体用例实际执行；其他未配置外部数据库的用例仍按原约定跳过。
- go vet ./...：通过。
- go test -race ./internal/handler/upload ./internal/router：通过，含真实媒体测试。
- go test ./internal/handler/upload -run 'TestVideoProcessorLive|TestUploadRealVideoLive' -v：通过，MP4/MOV 两种容器均验证。
- git diff --check：通过；新增文件另行检查行末空白。
- docker compose config --quiet：通过。
- OpenAPI YAML、operationId 唯一性、上传状态码及成功/失败示例 schema：通过。
- Docker 镜像构建未验证：本机 Docker 引擎未运行，管道不存在；已核对 Alpine 运行层依赖变更与现有 Compose 配置。

真实工具版本：ffmpeg version 9.0.2-essentials_build-www.gyan.dev Copyright (c) 2000-2026 the FFmpeg developers。工具仅放在系统临时目录 videofeed-ffmpeg-validation，下载来源为 FFmpeg 官网列出的 Windows 构建提供方 https://www.gyan.dev/ffmpeg/builds/ffmpeg-release-essentials.zip；未修改系统 PATH 或安装项目依赖。Linux 容器及反向代理上传超时行为尚未实测。

## 完成前复核

再次读取参考提交文件列表，四项全部有对应项：

- [x] `apps/api/Dockerfile`
- [x] `apps/api/internal/interfaces/http/upload/handler.go`
- [x] `apps/api/internal/interfaces/http/upload/handler_test.go`
- [x] `apps/api/test/upload_api_test.go`

常量、白名单、错误、接口、结构体、函数、来源测试和运行镜像依赖均已逐项核对，无未迁移项。已知限制是 Docker 镜像与部署代理未联调，不将本机工具成功等同于部署验证。按基础设施、业务逻辑、测试和文档四个主题分别创建 Git 提交。
