# 推荐模块

推荐流由三层组成：`internal/handler/recommendation` 负责两个内部 JSON 接口，`internal/application/recommendation` 负责候选召回、兴趣相似度、热度和新鲜度排序、游标分页及作者打散，`internal/infrastructure/persistence/recommendation` 负责 PostgreSQL 查询和事务写入。

## Feed 场景

`GET /api/feed-items?scene=recommend` 和 `POST /api/feed/query` 的 `scene=recommend` 要求 JWT。Feed 策略把 JWT 用户 ID、`cursor`、`limit` 和 `client_context.request_id` 传给推荐服务，再复用已有卡片、互动计数缓存和仓储组装响应。推荐页不写 Redis、不发布 RabbitMQ 消息；卡片或计数缓存失败时回源数据库。

## 排序和分页

候选池只包含已发布视频，排除用户近七天内曝光的视频，按互动热度、发布时间和视频 ID召回。推荐服务读取最近三十天最多二百条 `play`/`complete` 行为，按观看时长和完成标记加权生成兴趣向量。存在用户画像时使用 `0.70 * similarity + 0.20 * log1p(hot_score)/10 + 0.10 * freshness`，冷启动使用 `0.65 * log1p(hot_score)/10 + 0.35 * freshness`。页内游标基于评分、发布时间和视频 ID，在作者打散前生成；同作者相邻候选会延后。

## 曝光接口

`POST /internal/exposures` 接收用户、场景、请求标识和 `video_ids`。批内按视频去重并保留首次顺序，事务中追加 `exposed` 流水，再通过 PostgreSQL `ON CONFLICT` 原子累加 `exposures.exposure_count`。唯一键仍为 `(user_id, video_id)`；相同 `request_id` 跨请求不会去重。视频不存在、未发布或任意数据库错误都会回滚整个批次。

## 数据和依赖

实现复用 `videos`、`video_stat`、`video_embedding`、`video_view_events` 和 `exposures`，不新增配置、环境变量、Compose 服务、数据卷或 RabbitMQ 拓扑。PostgreSQL 表和唯一索引由现有 `router.Register` 负责迁移。
