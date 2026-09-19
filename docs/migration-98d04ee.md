# 本地哈希向量化迁移审计

来源：C:/Users/HP/Desktop/GCFeed，提交 `98d04ee7ca2df3b6beb3eb0390a68876336cdad7`，属于来源 main/origin/main。目标为当前 VideoFeed，开始时 main 分支工作区干净，基线 faf18ee。

## 逐文件迁移表

| 原文件 | 当前文件 | 新建 | 声明与适配 | 测试和文档 |
| --- | --- | --- | --- | --- |
| apps/api/internal/domain/embedding/errors.go | internal/domain/embedding/error.go | 是 | ErrDimensionMismatch 原样保留，按当前领域文件命名使用 error.go | 原维度错误测试；本记录 |
| apps/api/internal/domain/embedding/hash_ngram.go | internal/domain/embedding/hash_ngram.go | 是 | 包 domainembedding 改为 embedding；模型常量、接口、结构体与全部函数保持 | hash_ngram_test.go、edge_test.go；本记录 |
| apps/api/internal/domain/embedding/hash_ngram_test.go | internal/domain/embedding/hash_ngram_test.go | 是 | 四个测试和 norm 辅助函数保留，只改包名并补齐中文注释 | 原位置映射到当前领域目录 |

## 架构与行为边界

- 已读取来源 --stat、--name-only、--format=fuller 以及每个文件的完整 diff；按当前 AGENTS.md 补全所有函数与结构体的紧邻中文注释。
- 当前 Go 模块为 github.com/inumoroha/video-feed，领域层位于 internal/domain；本组件仅依赖 Go 标准库，不引入旧模块路径或包名。
- 保留 HashNgramModel="hash-ngram-v1"、HashNgramDimension=128、ErrDimensionMismatch 及 Vectorizer 接口的 Model/Dimension/Vectorize 三个方法。
- 保留 HashNgramVectorizer.dimension、NewHashNgramVectorizer、Model、Dimension、Vectorize、CosineSimilarity、normalizeText、addFeature、normalizeVector。
- Dimension 对空接收者和非正维度返回 128；每次 Vectorize 分配新切片。文本转小写并去首尾空白，保留 Unicode 字母/数字，把空白/标点/符号折叠为空格，其他字符按原实现忽略。
- token 使用 tok: 前缀；二元、三元字符特征基于去空格后的 rune 串，可跨 token 边界。使用 FNV-1a 64 位哈希，模向量维度决定位置、最高位决定符号，单位特征权重累加并 L2 归一化；哈希碰撞按原算法叠加或抵消。
- CosineSimilarity 支持未归一化向量；长度不同返回原错误；任一零范数（含等长空向量）返回 0。保留原浮点溢出和非有限数处理行为，没有额外截断或数值修正。
- 当前配置由 Viper 加载；数据库为 PostgreSQL/GORM、缓存为 Redis、消息为 RabbitMQ，Gin 路由在 internal/router 注册。该提交不调用这些设施，无事务、幂等、重试、资源关闭或故障回退增量需要迁移。
- 没有新增或修改 HTTP 接口，因此 api/openapi.yaml 无需修改；Compose、环境变量、数据卷、健康检查、依赖顺序与 go.mod/go.sum 均无需变更。
- 这是确定性哈希特征，不是语义模型；此次没有增加向量数据库、推荐召回接口或画像消费者。

## 测试映射

- TestHashNgramVectorizerDeterministic：保留相同输入相同输出、模型与维度、L2 范数断言。
- TestHashNgramVectorizerSimilarity：保留相关文本得分高于无关文本的样例。
- TestHashNgramVectorizerEmptyText：保留空白文本生成 128 维零向量。
- TestCosineSimilarityDimensionMismatch：保留返回同一个错误值的断言。
- norm：保留原测试范数计算。
- 补充边界和并发测试放在 internal/domain/embedding/edge_test.go。

## 验证结果

- 重新列出来源全部 3 个文件，均与迁移表逐项对应；没有遗漏文件。
- 对每个来源文件统一包名、去除注释和排版后，与目标文件完整代码比对一致，涵盖导出与非导出的类型、接口、函数、常量、错误及原测试断言。
- 新增测试 TestVectorizerDefaults、TestUnicodeNormalization、TestCosineSimilarityEdges、TestFeatureAndNormalizationEdges、TestVectorizerConcurrentIsolation 分别验证维度回退、Unicode 清洗、余弦边界、权重抵消/归一化以及并发结果与切片隔离。
- gofmt 格式化并复查：通过。
- go test -count=1 -v ./internal/domain/embedding：全部 9 个测试通过，无跳过。
- go test ./...：通过。
- go vet ./...：通过。
- 容器内 go test -race -count=1 ./internal/domain/embedding：通过，使用 golang:1.25（Go 1.25.14 / Linux GCC）；容器自动删除。组件仅使用标准库，无外部服务测试需要跳过或等待。
- git diff --check 及新增文件尾空白检查：通过。
- docker compose config --quiet：通过；此次未变更或启动 Compose 服务。
- OpenAPI YAML 解析和 operationId 唯一性检查：通过；接口行为未改变，无文档接口增量。
- 新增/修改函数、结构体的紧邻中文注释检查：通过。

结论：来源组件及全部测试已完整迁移，未发现缺失行为或无法验证的迁移项。共新增 5 个文件（3 个来源文件的对应实现、1 个补充测试文件、1 份本审计记录），未修改已有业务文件；验证完成后按用户要求创建独立迁移提交。
