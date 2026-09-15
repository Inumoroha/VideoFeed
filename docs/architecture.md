# 视频 Feed 系统架构图（MVP）

本文描述当前视频 Feed 系统的实现边界与演进方向。Go API 以单体方式承载用户、视频、Feed、互动与上传能力；Redis、消息队列和对象存储仍属于后续演进范围。

## 1. 系统上下文

这张图展示 GCFeed 与客户端、存储和演进型基础设施的边界。

```mermaid
---
config:
  theme: base
  layout: dagre
  themeVariables:
    background: transparent
    fontFamily: Inter, PingFang SC, Microsoft YaHei, Arial
    primaryTextColor: "#0F172A"
    lineColor: "#94A3B8"
    clusterBkg: "#F8FAFC"
    clusterBorder: "#CBD5E1"
---
flowchart LR
  %% GCFeed MVP system context
  classDef client fill:#EFF6FF,stroke:#60A5FA,color:#0F172A,stroke-width:1px;
  classDef system fill:#DCFCE7,stroke:#22C55E,color:#0F172A,stroke-width:1px;
  classDef store fill:#F1F5F9,stroke:#64748B,color:#0F172A,stroke-width:1px;
  classDef future fill:#FAE8FF,stroke:#C084FC,color:#0F172A,stroke-width:1px,stroke-dasharray:5 5;

  Web["Web App<br/>React + Vite"]
  Client["移动端 / API 调用方"]
  API["GCFeed API<br/>Go + Gin"]
  MySQL[("MySQL<br/>业务数据")]
  Uploads[("uploads<br/>视频 / 封面 / 头像")]
  Redis[("Redis<br/>缓存与计数")]
  MQ[("MQ<br/>异步事件")]
  ObjectStorage[("对象存储<br/>媒体文件")]

  Web -->|"调用管理与浏览接口"| API
  Client -->|"调用公开 API"| API
  API -->|"读写 users / videos / video_stat / interaction_*"| MySQL
  API -->|"保存和读取本地文件"| Uploads
  API -.->|"缓存 Feed 与计数"| Redis
  API -.->|"投递互动和审核事件"| MQ
  API -.->|"迁移媒体文件"| ObjectStorage

  class Web,Client client;
  class API system;
  class MySQL,Uploads store;
  class Redis,MQ,ObjectStorage future;
  linkStyle default stroke:#94A3B8,stroke-width:1.4px
```

## 2. API 内部分层

这张图展示 Go API 单体内部的主要代码层次和依赖方向。

```mermaid
---
config:
  theme: base
  layout: dagre
  themeVariables:
    background: transparent
    fontFamily: Inter, PingFang SC, Microsoft YaHei, Arial
    primaryTextColor: "#0F172A"
    lineColor: "#94A3B8"
    clusterBkg: "#F8FAFC"
    clusterBorder: "#CBD5E1"
---
flowchart LR
  %% Internal layering in apps/api
  classDef entry fill:#F8FAFC,stroke:#94A3B8,color:#0F172A,stroke-width:1px;
  classDef http fill:#DBEAFE,stroke:#3B82F6,color:#0F172A,stroke-width:1px;
  classDef service fill:#DCFCE7,stroke:#22C55E,color:#0F172A,stroke-width:1px;
  classDef domain fill:#CCFBF1,stroke:#14B8A6,color:#0F172A,stroke-width:1px;
  classDef infra fill:#FFEDD5,stroke:#F97316,color:#0F172A,stroke-width:1px;
  classDef store fill:#F1F5F9,stroke:#64748B,color:#0F172A,stroke-width:1px;

  Entry["cmd/server/main.go<br/>启动装配"]
  Router["Router<br/>路由注册"]
  Auth["JWT Middleware<br/>鉴权上下文"]
  Handlers["HTTP Handlers<br/>user / video / feed / interaction / upload"]
  Services["Application Services<br/>user / video / feed / interaction"]
  Domains["Domain Models<br/>user / video / feed / interaction"]
  Config["Config Loader<br/>configs/config.yaml"]
  JWT["JWT Manager<br/>签发访问令牌"]
  Repo["GORM Repository<br/>仓储实现"]
  SQL["database/sql<br/>MySQL 连接"]
  MySQL[("MySQL")]
  Uploads[("uploads 目录")]

  Entry -->|"加载配置"| Config
  Entry -->|"创建数据库连接"| SQL
  Entry -->|"注册 HTTP 路由"| Router
  Router -->|"校验受保护接口"| Auth
  Auth -->|"解析和签名 Token"| JWT
  Router -->|"分发业务请求"| Handlers
  Handlers -->|"调用用例服务"| Services
  Services -->|"执行领域规则"| Domains
  Services -->|"读写仓储"| Repo
  Repo -->|"复用连接池"| SQL
  SQL -->|"持久化数据"| MySQL
  Handlers -->|"保存上传文件"| Uploads
  Router -->|"暴露静态文件"| Uploads

  class Entry entry;
  class Router,Auth,Handlers http;
  class Services service;
  class Domains domain;
  class Config,JWT,Repo,SQL infra;
  class MySQL,Uploads store;
  linkStyle default stroke:#94A3B8,stroke-width:1.4px
```

## 3. 核心请求链路

这张图展示注册、登录、上传、发布、刷 Feed 与互动的核心请求链路。上传、发布和互动请求都需要携带 Bearer JWT。

```mermaid
---
config:
  theme: base
  themeVariables:
    background: transparent
    fontFamily: Inter, PingFang SC, Microsoft YaHei, Arial
    primaryTextColor: "#0F172A"
    actorBkg: "#EFF6FF"
    actorBorder: "#60A5FA"
    actorTextColor: "#0F172A"
    activationBkgColor: "#DCFCE7"
    activationBorderColor: "#22C55E"
    sequenceNumberColor: "#475569"
    lineColor: "#94A3B8"
---
sequenceDiagram
  autonumber
  participant C as Client
  participant R as Gin Router
  participant H as Handler
  participant S as Service
  participant Repo as GORM Repository
  participant DB as MySQL
  participant FS as uploads
  participant Auth as JWT Middleware

  C->>R: POST /api/users
  R->>H: User.Register
  H->>S: 创建用户
  S->>Repo: Save user
  Repo->>DB: INSERT users
  DB-->>Repo: 返回 user id
  Repo-->>S: 返回用户实体
  S-->>H: 返回 profile
  H-->>C: 201 Created

  C->>R: POST /api/sessions
  R->>H: User.Login
  H->>S: 校验账号密码
  S->>Repo: FindByAccount
  Repo->>DB: SELECT users
  DB-->>Repo: 返回用户记录
  S-->>H: 返回 Bearer JWT
  H-->>C: 返回 access_token

  C->>R: POST /api/uploads (Bearer JWT)
  R->>Auth: 校验访问令牌
  Auth->>H: Upload.Create（用户上下文）
  H->>FS: 写入 video / cover / avatar
  FS-->>H: 返回文件路径
  H-->>C: 返回 /uploads/{kind}/{filename}

  C->>R: POST /api/videos (Bearer JWT)
  R->>Auth: 校验访问令牌
  Auth->>H: Video.Create（用户上下文）
  H->>S: CreatePublished
  S->>Repo: Save video and video_stat
  Repo->>DB: INSERT videos, video_stat
  DB-->>Repo: 返回 video id
  Repo-->>S: 返回视频实体
  S-->>H: 返回视频详情
  H-->>C: 201 Created

  C->>R: GET /api/feed-items?cursor=...&limit=...
  R->>H: Feed.ListFeedItems
  H->>S: GetFeed(scene=timeline)
  S->>Repo: ListFeed
  Repo->>DB: SELECT videos LEFT JOIN users LEFT JOIN video_stat
  DB-->>Repo: 返回 Feed 行
  Repo-->>S: 返回 FeedItem 列表
  S-->>H: 返回 items + next_cursor + has_more
  H-->>C: 返回 timeline feed items

  C->>R: POST /api/interactions/like (Bearer JWT)
  R->>Auth: 校验访问令牌
  Auth->>H: Interaction.ToggleLike（用户上下文）
  H->>S: 切换点赞状态
  S->>Repo: 写入互动动作并刷新计数
  Repo->>DB: UPDATE interaction_action, video_stat
  S-->>H: 返回点赞状态与统计
  H-->>C: 200 OK
```

## 4. 数据模型

这张图展示当前 GORM 自动迁移的数据表及业务关联。关联用于表达业务关系；当前持久化模型未声明数据库外键约束。

```mermaid
---
config:
  theme: base
  themeVariables:
    background: transparent
    fontFamily: Inter, PingFang SC, Microsoft YaHei, Arial
    primaryTextColor: "#0F172A"
    lineColor: "#94A3B8"
---
erDiagram
  USERS ||--o{ VIDEOS : "发布视频"
  VIDEOS ||--|| VIDEO_STAT : "拥有统计"
  USERS ||--o{ INTERACTION_ACTION : "发起点赞或收藏"
  VIDEOS ||--o{ INTERACTION_ACTION : "接收点赞或收藏"
  USERS ||--o{ INTERACTION_COMMENT : "发表评论"
  VIDEOS ||--o{ INTERACTION_COMMENT : "接收评论"
  USERS ||--o{ INTERACTION_ACTION_IDEMPOTENCY : "保存请求结果"
  VIDEOS ||--o{ INTERACTION_ACTION_IDEMPOTENCY : "关联请求视频"

  USERS {
    bigint id PK
    string account UK
    string password
    string nickname
    string avatar_url
    string bio
    string role
    int status
  }

  VIDEOS {
    bigint id PK
    bigint author_id
    string title
    string description
    string media_url
    string cover_url
    int status
    datetime published_at
    string idempotency_key
  }

  VIDEO_STAT {
    bigint video_id PK
    int like_count
    int comment_count
    int favorite_count
  }

  INTERACTION_ACTION {
    bigint id PK
    bigint user_id
    bigint video_id
    string action_type
    int status
    string idempotency_key
  }

  INTERACTION_ACTION_IDEMPOTENCY {
    bigint id PK
    bigint user_id
    bigint video_id
    string action_type
    string idempotency_key
    boolean active
    int like_count
    int favorite_count
  }

  INTERACTION_COMMENT {
    bigint id PK
    bigint video_id
    bigint user_id
    string content
    int status
    string idempotency_key
    datetime created_at
  }
```

## 5. 演进能力地图

这张图展示已落地闭环和后续模块的连接方式，虚线代表规划中的能力边界。

```mermaid
---
config:
  theme: base
  layout: dagre
  themeVariables:
    background: transparent
    fontFamily: Inter, PingFang SC, Microsoft YaHei, Arial
    primaryTextColor: "#0F172A"
    lineColor: "#94A3B8"
    clusterBkg: "#F8FAFC"
    clusterBorder: "#CBD5E1"
---
flowchart TB
  %% Roadmap map from current MVP to planned modules
  classDef current fill:#DCFCE7,stroke:#22C55E,color:#0F172A,stroke-width:1px;
  classDef growth fill:#FAE8FF,stroke:#C084FC,color:#0F172A,stroke-width:1px,stroke-dasharray:5 5;
  classDef platform fill:#FFEDD5,stroke:#F97316,color:#0F172A,stroke-width:1px,stroke-dasharray:5 5;
  classDef data fill:#F1F5F9,stroke:#64748B,color:#0F172A,stroke-width:1px,stroke-dasharray:5 5;

  User["用户"]
  Video["视频"]
  Feed["Feed"]
  Upload["上传"]
  Recommendation["推荐"]
  Interaction["互动"]
  Review["审核"]
  Message["消息"]
  Admin["后台运营"]
  Governance["系统治理"]
  Observability["监控告警"]
  AsyncStore[("Redis / MQ / 对象存储")]

  User -->|"扩展关注关系"| Interaction
  Video -->|"进入内容审核"| Review
  Video -->|"承载点赞评论收藏"| Interaction
  Upload -->|"迁移媒体文件"| AsyncStore
  Feed -->|"接入召回排序"| Recommendation
  Recommendation -->|"返回候选内容"| Feed
  Interaction -->|"投递互动事件"| AsyncStore
  AsyncStore -->|"生成站内通知"| Message
  Admin -->|"处理审核任务"| Review
  Governance -->|"保护核心接口"| Feed
  Observability -->|"采集服务指标"| Governance

  class User,Video,Feed,Upload,Interaction current;
  class Recommendation,Review,Message,Admin growth;
  class Governance,Observability platform;
  class AsyncStore data;
  linkStyle default stroke:#94A3B8,stroke-width:1.4px
```

## 6. 说明

- 当前代码以 Go API 单体承载用户、视频、Feed、互动与上传能力，内部按接口层、应用层、领域层、基础设施层组织。
- 业务接口统一挂载在 `/api/*`；互动接口为 `/api/interactions/*` 与 `/api/comments`。静态文件通过 `/uploads/*` 访问，健康检查使用 `/health`。
- 数据持久化使用 MySQL，GORM 自动迁移 `users`、`videos`、`video_stat`、`interaction_action`、`interaction_action_idempotency` 与 `interaction_comment` 表。
- Feed 当前采用 Timeline 策略，按 `published_at DESC, id DESC` 排序，并通过 Base64 游标分页。
- 推荐、审核、消息、治理和监控模块作为演进边界保留；已落地的互动模块后续可接入异步事件和独立服务。

