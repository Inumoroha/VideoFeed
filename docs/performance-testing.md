# 性能测试指南

本文档说明如何用 k6、Prometheus 和 Grafana 测 VideoFeed 的接口性能、QPS、P95 延迟、错误率和缓存效果。

## 前置准备

安装 k6：

```bash
# macOS
brew install k6
# Windows
winget install k6
# Debian/Ubuntu
sudo gpg -k
sudo gpg --no-default-keyring --keyring /usr/share/keyrings/k6-archive-keyring.gpg --keyserver hkp://keyserver.ubuntu.com:80 --recv-keys C5AD17C747E3415A3642D57D77C6C491D6AC1D69
echo "deb [signed-by=/usr/share/keyrings/k6-archive-keyring.gpg] https://dl.k6.io/deb stable main" | sudo tee /etc/apt/sources.list.d/k6.list
sudo apt-get update
sudo apt-get install k6
```

启动项目（在仓库根目录执行）：

```bash
docker compose up -d --build
```

确认 API 正常：

```bash
curl http://127.0.0.1:9000/health
```

确认监控正常：

```bash
curl http://127.0.0.1:9000/metrics
curl http://127.0.0.1:9090/-/ready
curl http://127.0.0.1:3000/api/health
```

Grafana 面板：

```text
http://127.0.0.1:3000/d/videofeed-overview/videofeed-overview
```

默认账号密码：

```text
admin / admin
```

`admin / admin` 是 Compose 中的本地示例值，部署前必须通过环境变量替换；压测环境不要长期保留默认口令。

## 仓库内的压测脚本

仓库提供 `scripts/feed-load.js`，默认压测推荐流，带登录、阈值和自定义指标：

```bash
k6 run -e ACCOUNT="你的账号" -e PASSWORD="你的密码" scripts/feed-load.js
k6 run -e SCENE=timeline -e VUS=20 -e DURATION=60s scripts/feed-load.js
k6 run -e SCENE=hot -e VUS=20 -e DURATION=60s scripts/feed-load.js
```

可用环境变量：`BASE_URL`（默认 `http://127.0.0.1:9000`）、`SCENE`、`TOKEN`、`ACCOUNT`、`PASSWORD`、`LIMIT`、`THINK_TIME`、`VUS`、`DURATION`。脚本优先使用 `TOKEN`；只传 `ACCOUNT` 和 `PASSWORD` 时会先调用 `/api/sessions` 获取登录态。

下面各节提供不落地文件的内联脚本，便于临时调整场景或参数。

## 测最新视频流

目标：测公开视频 Feed 的 QPS、成功率、平均延迟和 P95 延迟。

```bash
SCENE=timeline VUS=20 DURATION=60s THINK_TIME=1 k6 run - <<'EOF'
import http from "k6/http";
import { check, sleep } from "k6";
import { Rate } from "k6/metrics";

const BASE_URL = (__ENV.BASE_URL || "http://127.0.0.1:9000").replace(/\/$/, "");
const SCENE = __ENV.SCENE || "timeline";
const LIMIT = Number(__ENV.LIMIT || 10);
const THINK_TIME = Number(__ENV.THINK_TIME || 1);
const successRate = new Rate("feed_success_rate");

export const options = {
  vus: Number(__ENV.VUS || 20),
  duration: __ENV.DURATION || "60s",
  thresholds: {
    http_req_failed: ["rate<0.01"],
    http_req_duration: ["p(95)<500"],
    feed_success_rate: ["rate>0.99"],
  },
};

export default function () {
  const url = `${BASE_URL}/api/feed-items?scene=${encodeURIComponent(SCENE)}&limit=${LIMIT}`;
  const res = http.get(url);
  const ok = check(res, {
    "status is 200": (r) => r.status === 200,
    "has feed items array": (r) => Array.isArray(r.json().items),
  });
  successRate.add(ok);
  sleep(THINK_TIME);
}
EOF
```

重点看：

- `http_reqs` 后面的 `/s`：QPS
- `http_req_duration avg`：平均延迟
- `http_req_duration p(95)`：P95 延迟
- `http_req_failed`：失败率
- `feed_success_rate`：业务成功率

## 测热门榜单

目标：测热榜读取链路，包括 Redis 热榜窗口和 Feed 卡片组装。

```bash
SCENE=hot VUS=20 DURATION=60s THINK_TIME=1 k6 run - <<'EOF'
import http from "k6/http";
import { check, sleep } from "k6";
import { Rate } from "k6/metrics";

const BASE_URL = (__ENV.BASE_URL || "http://127.0.0.1:9000").replace(/\/$/, "");
const SCENE = __ENV.SCENE || "hot";
const LIMIT = Number(__ENV.LIMIT || 10);
const THINK_TIME = Number(__ENV.THINK_TIME || 1);
const successRate = new Rate("feed_success_rate");

export const options = {
  vus: Number(__ENV.VUS || 20),
  duration: __ENV.DURATION || "60s",
  thresholds: {
    http_req_failed: ["rate<0.01"],
    http_req_duration: ["p(95)<500"],
    feed_success_rate: ["rate>0.99"],
  },
};

export default function () {
  const url = `${BASE_URL}/api/feed-items?scene=${encodeURIComponent(SCENE)}&limit=${LIMIT}`;
  const res = http.get(url);
  const ok = check(res, {
    "status is 200": (r) => r.status === 200,
    "has feed items array": (r) => Array.isArray(r.json().items),
  });
  successRate.add(ok);
  sleep(THINK_TIME);
}
EOF
```

## 测推荐流

推荐流需要登录态。先准备一个账号密码，再运行：

```bash
ACCOUNT="你的账号" PASSWORD="你的密码" VUS=20 DURATION=60s THINK_TIME=1 k6 run - <<'EOF'
import http from "k6/http";
import { check, sleep } from "k6";
import { Rate } from "k6/metrics";

const BASE_URL = (__ENV.BASE_URL || "http://127.0.0.1:9000").replace(/\/$/, "");
const ACCOUNT = __ENV.ACCOUNT || "";
const PASSWORD = __ENV.PASSWORD || "";
const LIMIT = Number(__ENV.LIMIT || 10);
const THINK_TIME = Number(__ENV.THINK_TIME || 1);
const successRate = new Rate("feed_success_rate");

export const options = {
  vus: Number(__ENV.VUS || 20),
  duration: __ENV.DURATION || "60s",
  thresholds: {
    http_req_failed: ["rate<0.01"],
    http_req_duration: ["p(95)<500"],
    feed_success_rate: ["rate>0.99"],
  },
};

export function setup() {
  const res = http.post(
    `${BASE_URL}/api/sessions`,
    JSON.stringify({ account: ACCOUNT, password: PASSWORD }),
    { headers: { "Content-Type": "application/json" } }
  );
  if (res.status !== 200) {
    throw new Error(`login failed: status=${res.status} body=${res.body}`);
  }
  return { token: res.json().access_token };
}

export default function (data) {
  const res = http.post(
    `${BASE_URL}/api/feed/query`,
    JSON.stringify({
      scene: "recommend",
      limit: LIMIT,
      context: { request_id: `k6-recommend-${Date.now()}-${__VU}-${__ITER}` },
    }),
    {
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${data.token}`,
      },
    }
  );
  const ok = check(res, {
    "status is 200": (r) => r.status === 200,
    "has feed items array": (r) => Array.isArray(r.json().items),
  });
  successRate.add(ok);
  sleep(THINK_TIME);
}
EOF
```

## 测极限 QPS

普通压测会模拟用户停顿，`THINK_TIME=1` 时 20 VU 的理论 QPS 接近 20。测服务极限吞吐时，把等待时间设成 0：

```bash
SCENE=timeline VUS=50 DURATION=60s THINK_TIME=0 k6 run - <<'EOF'
import http from "k6/http";
import { check, sleep } from "k6";

const BASE_URL = (__ENV.BASE_URL || "http://127.0.0.1:9000").replace(/\/$/, "");
const SCENE = __ENV.SCENE || "timeline";
const THINK_TIME = Number(__ENV.THINK_TIME || 0);

export const options = {
  vus: Number(__ENV.VUS || 50),
  duration: __ENV.DURATION || "60s",
};

export default function () {
  const res = http.get(`${BASE_URL}/api/feed-items?scene=${SCENE}&limit=10`);
  check(res, { "status is 200": (r) => r.status === 200 });
  sleep(THINK_TIME);
}
EOF
```

逐步增加 `VUS`：

```bash
VUS=50
VUS=100
VUS=200
```

当 P95 明显升高、失败率上升或 CPU/数据库压力明显升高时，就接近当前本地环境上限。

## 如何解读结果

示例：

```text
http_reqs......................: 1200    19.85/s
http_req_duration..............: avg=5.35ms p(95)=17.96ms
http_req_failed................: 0.00%
feed_success_rate..............: 100.00%
```

含义：

- `http_reqs 19.85/s`：QPS 约 19.85
- `avg=5.35ms`：平均响应时间 5.35ms
- `p(95)=17.96ms`：95% 请求在 17.96ms 内完成
- `http_req_failed=0.00%`：HTTP 失败率为 0
- `feed_success_rate=100%`：业务检查全部通过

上面这组数字是示例格式，不是 VideoFeed 在某个真实环境下的实测结果；记录到简历或报告前必须在本机或目标环境重新压测并保留原始输出。

## 结合 Grafana 看指标

压测时打开：

```text
http://127.0.0.1:3000/d/videofeed-overview/videofeed-overview
```

重点观察：

- API QPS
- API 5xx Error Rate
- API P95 Latency
- Feed P95 Latency
- Feed Cache Hit Rate
- Upload and Video Processing P95
- Worker Success Rate

Prometheus 也可以直接查询：

```promql
sum(rate(videofeed_http_requests_total[5m])) by (route)
histogram_quantile(0.95, sum(rate(videofeed_http_request_duration_seconds_bucket[5m])) by (le, route))
histogram_quantile(0.95, sum(rate(videofeed_feed_request_duration_seconds_bucket[5m])) by (le, scene))
sum(rate(videofeed_feed_cache_requests_total{result="hit"}[5m])) by (area)
```

指标名称、标签和采集点见 [监控模块](modules/monitoring.md)；服务地址与限制见 [部署说明](deployment.md)。

## 测试前的数据准备

为了让结果更接近真实场景，建议先准备：

- 20 到 50 个公开视频
- 多个用户
- 一些点赞、收藏、评论
- 至少一次推荐流访问，让推荐候选链路产生数据

当 Feed 返回空数组时，延迟指标仍然有效，业务场景说服力会弱一些。
