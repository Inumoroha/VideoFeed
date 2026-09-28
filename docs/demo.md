# VideoFeed 本地演示

本仓库目前没有配置 Git remote。可在本机通过 Docker Compose 展示完整链路；对外展示仓库时，应先自行选择代码托管地址并检查本地配置、数据和演示账号，再添加 remote。以下步骤不需要预置账号或访问生产服务。

## 启动

在仓库根目录运行 `docker compose up --build -d`，随后运行 `docker compose ps`。八项服务应处于 running，`postgres`、`redis`、`rabbitmq`、`api` 应显示 healthy。首次构建需要拉取镜像；已有数据卷会保留测试数据。不要使用 `docker compose down -v` 清理演示现场。

打开 Web：<http://127.0.0.1:5173>。API 健康检查位于 <http://127.0.0.1:9000/health>，Prometheus 位于 <http://127.0.0.1:9090/targets>，Grafana 看板位于 <http://127.0.0.1:3000/d/videofeed-overview/videofeed-overview>。Grafana 的 Compose 示例账号是 `admin` / `admin`，仅供本地演示。

## 发布与消费链路

1. 在 Web 注册新账号并登录。准备一个短 H.264 MP4 和 PNG 封面；若本机装有 FFmpeg，可用以下命令生成演示素材：

   ```bash
   ffmpeg -f lavfi -i testsrc2=size=640x360:rate=24 -t 2 -c:v libx264 -pix_fmt yuv420p -movflags +faststart demo.mp4
   ffmpeg -f lavfi -i color=c=teal:s=640x360 -frames:v 1 demo.png
   ```

2. 在 Web 发布页选择两个文件，填写标题并发布。浏览器向 `/api/videos/upload` 发送一次 multipart 请求；成功后在作品详情和最新流中查看视频。可以在浏览器开发者工具中确认视频 Range 请求返回 `206`。服务端镜像内有 `ffmpeg/ffprobe`，会验证视频元数据并执行 MP4 faststart。
3. 用无效封面或空标题重试，确认页面显示校验失败，且不会创建作品。本次 Compose 联调还核对了上传卷文件数：两种失败前后均为视频/封面 `1/1`。异常进程退出或文件删除失败仍可能留下孤儿文件，旧的独立上传接口也不负责回收未引用文件。
4. 查看 <http://127.0.0.1:9000/metrics> 的 `videofeed_video_upload_total{kind="publish"}`，以及 Prometheus 的 API/worker 两个 target 和 Grafana `VideoFeed Overview` 看板。互动、发布、向量队列有消费者；曝光队列目前没有消费进程。

## 性能证据

原始 k6 摘要保存在 [timeline-compose-2026-09-28.json](performance-results/timeline-compose-2026-09-28.json)。当次在 Windows Docker Desktop 的 Compose 网络内，用 `grafana/k6:0.54.0` 对公开时间线 `GET /api/feed-items?scene=timeline&limit=10` 运行 20 VU、60 秒、每轮间隔 0.2 秒。数据库当时有 12 位用户和 15 个视频；完成 5,880 次请求，平均 97.96 RPS，P95 5.62 ms，HTTP 失败率 0%，k6 阈值全部通过。

这是小数据量和缓存命中的本机基线，不能推断最大吞吐或线上延迟。在仓库根目录可用 PowerShell 复跑相同负载：

```powershell
docker run --rm --network videofeed_default -v "${PWD}/scripts:/scripts:ro" -e BASE_URL=http://api:9000 -e SCENE=timeline -e VUS=20 -e DURATION=60s -e THINK_TIME=0.2 grafana/k6:0.54.0 run /scripts/feed-load.js
```

脚本见 [scripts/feed-load.js](../scripts/feed-load.js)，其他场景见 [性能测试指南](performance-testing.md)。复跑时记录当次数据规模和机器环境，不要把指南中的示例数字写入简历。
