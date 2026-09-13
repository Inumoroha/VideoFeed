# Day 7：Feed 链路与缓存能力完善

## 今日目标

完成视频 Feed MVP 主链路的持久化、应用服务、HTTP 处理器和 Redis 缓存适配，确保时间线与热榜能够稳定分页。

## 完成内容

- 统一 Feed Repository 的时间线和热榜分页接口。
- 增加 `video`、`video_stat` 的批量卡片与统计查询。
- 修复时间线和热榜游标分页，保证排序稳定。
- 实现 Redis Feed 页、视频卡片和统计缓存。
- 增加 Redis 分钟热度桶和最近一小时热榜窗口查询。
- 适配 Handler 与应用层新的 Repository、Cache 接口。
- 更新架构、Feed 模块和优化方案文档。

## 关键设计

时间线按 `published_at DESC, id DESC` 排序；热榜按 `hot_score DESC, published_at DESC, id DESC` 排序。热度分计算为：

```text
like_count * 3 + comment_count * 5 + favorite_count * 4
```

页缓存只保存视频 ID 和排序字段，卡片和统计通过 Redis MGET 批量读取，缺失数据再批量回源数据库。热榜互动通过分钟粒度 ZSET 累加，并合并最近 60 个分钟桶。

## 验证情况

已通过 Feed 持久化、缓存和 Handler 包测试。应用层仍有并发取消隔离测试需要后续完善：singleflight 共享查询的等待取消行为尚未完全满足测试预期。

## 后续计划

- 完善 singleflight 请求级取消处理。
- 为 Redis FeedCache 增加集成测试。
- 补充热榜窗口分页和互动热度写入测试。
