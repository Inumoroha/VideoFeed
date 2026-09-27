# 性能测试指南迁移审计

来源：`C:/Users/HP/Desktop/GCFeed`，提交 `7aceb3f49c2c63bbaa76cc29eb489b4649848e7a`，标题为 `docs(performance): 改为 Markdown 压测指南`，作者与提交时间均为 2026-06-06 21:47:57 +0800。目标为当前 VideoFeed，迁移前工作区干净，基线为提交 `cba55e5`。

## 提交完整变更

已确认该提交只存在于来源仓库，并执行 `git show --stat`、`git show --name-only`、`git show --format=fuller` 以及逐文件完整 diff。

| 项 | 结果 |
| --- | --- |
| 变更文件 | 3 个：`README.md`、新增 `docs/performance-testing.md`、删除 `scripts/feed-load.js` |
| 变更规模 | `296 insertions(+), 139 deletions(-)` |
| 变更类型 | 纯文档重构：把压测说明从 README 与脚本内联代码改写成独立指南，并删除脚本文件 |

## 逐文件迁移表

| 原文件 | 当前文件 | 新建 | 声明与适配 | 测试和文档 |
| --- | --- | --- | --- | --- |
| `docs/performance-testing.md` | `docs/performance-testing.md` | 是 | 保留全部章节：前置准备、四类场景压测、极限 QPS、结果解读、Grafana 指标、数据准备；替换全部仓库名、端口、路由与指标前缀；补充 k6 的 Windows 与 Linux 安装方式 | 本记录 |
| `README.md` | `README.md` | 否 | README 的压测章节改为指向新指南并保留重点观察指标，文档地图新增该指南条目 | 同上 |
| `scripts/feed-load.js` | `scripts/feed-load.js` | 否（保留） | 来源删除该脚本并把场景改为指南中的内联脚本；VideoFeed 保留脚本并在 README 与指南中同时保留「仓库脚本」与「内联脚本」两条路径，删改理由见下节 | 语法检查 |

## 与来源的差异

- **仓库名与路径**：正文项目名改为 VideoFeed；删除来源中写死的个人绝对路径 `cd /Users/sealos/repositories/GCFeed/apps`，改为「在仓库根目录执行」，与 VideoFeed 的 Compose 文件位置一致。
- **端口**：API 相关地址由 `8080` 全部改为 `9000`，与 `configs/config.yaml`、Compose 和 README 一致。
- **路由**：推荐流压测路径由来源的 `POST /api/feed-queries` 适配为 VideoFeed 的 `POST /api/feed/query`；最新流与热榜沿用 `/api/feed-items`。
- **指标前缀**：PromQL 中的 `gcfeed_` 全部改为 `videofeed_`，与 `internal/infrastructure/metrics` 的命名空间一致。
- **看板地址**：由 `gcfeed-overview` 改为 `videofeed-overview`，与 `monitoring/grafana/dashboards/videofeed-overview.json` 的 uid 一致。
- **k6 安装**：来源只给出 macOS 的 `brew install k6`；VideoFeed 开发环境包含 Windows，补充 Windows 与 Debian/Ubuntu 安装命令。
- **示例数字**：来源把示例压测结果直接写成可用于简历的表述。VideoFeed 的指南明确标注该组数字只是输出格式示例、不是本项目实测结果，要求在记录到简历或报告前重新压测并保留原始输出，避免把未验证数据当作事实。
- **保留压测脚本**：来源在同一提交中删除 `scripts/feed-load.js`，把场景改为指南里的内联脚本。VideoFeed 在上一提交（`2b9eaa5` 迁移）刚引入该脚本，README 也引用了它，且脚本内的推荐场景路径与 `BASE_URL` 已按 VideoFeed 适配（`/api/feed/query`、`9000`）。为了不引入回归，本次保留脚本，并在指南中把它记为「仓库内的压测脚本」，同时保留来源的内联脚本写法供临时调整参数使用。这是对来源行为的偏离，特此记录。

## 逐项行为核对

- **章节完整性**：来源 8 个二级章节全部保留，内联 k6 脚本 4 段（最新流、热榜、推荐流、极限 QPS）逐段迁移，阈值与自定义指标 `feed_success_rate` 未改动。
- **结果解读口径**：`http_reqs /s`、`http_req_duration avg`、`p(95)`、`http_req_failed`、`feed_success_rate` 五个观察项的解释与来源一致。
- **Grafana 面板清单**：7 个面板名称与看板 JSON 一致。
- **PromQL**：4 条查询语句与看板中的写法保持同一口径，仅替换命名空间前缀。
- **数据准备建议**：公开视频数量、多用户、互动数据、先访问一次推荐流四条建议原样保留。
- **文档链接**：新增指向 `modules/monitoring.md` 与 `deployment.md` 的相对链接，均已核对可解析。

## 验证结果

- 重新枚举来源 3 个文件，逐一对应迁移表；删除的文件按「保留并在指南中同时提供内联脚本」处理，理由已记录。
- 新指南共 287 行（来源 293 行），差异来自删除个人绝对路径、补充 k6 安装方式与示例数字声明。
- 对指南执行来源残留检查：`GCFeed`、`gcfeed`、`8080`、`feed-queries`、`sealos`、`apps` 均无命中，说明仓库名、端口、路由与指标前缀已全部适配。
- README 压测章节已无内联 k6 代码块，改为指向新指南，并保留四个重点观察指标。
- Markdown 相对链接检查：README、新指南、迁移索引的站内链接全部可解析。
- 本次为纯 Markdown 变更，没有 Go 代码、测试、配置、依赖、数据库、缓存、消息队列或接口变更；因此 `gofmt`、`go test`、`go vet`、OpenAPI 与 Compose 校验不受影响。上一提交已完成这些校验，本次未重复执行，也未改动相关文件。
- `scripts/feed-load.js` 未改动，其 Node ESM 语法检查在上一提交已通过。

## 已知限制与未验证内容

- 未实际执行 k6 压测：环境中没有安装 k6，指南中的命令、内联脚本与阈值未在真实负载下运行。
- 指南未包含 VideoFeed 的实测性能数据；示例输出仅用于说明指标含义。
- 未验证 Grafana 看板在压测期间的实际出图效果，面板名称与 PromQL 为静态核对。

## 最终审计

来源提交 3 个文件重新枚举：新增指南已按 VideoFeed 的仓库名、端口、路由、指标前缀与看板地址完整迁移；README 的压测章节与文档地图同步更新；被删除的脚本按保留处理并在本记录中说明偏离原因与验证依据。

结论：来源提交的压测指南内容已完整迁移并完成适配，未发现尚未迁移的章节或脚本片段；唯一的主动偏离是保留压测脚本，已记录理由；实际压测与看板出图未执行，已在限制中列出。
