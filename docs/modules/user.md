# 用户模块

本文档描述当前代码已经实现的用户功能，不记录尚未实现的接口或业务规则。

## 1. 模块职责

用户模块目前负责：

- 用户注册和密码登录；
- 签发、校验访问用 JWT；
- 获取和更新当前登录用户的资料；
- 匿名获取其他用户的公开资料；
- 处理退出登录请求。

当前没有刷新令牌接口、账号找回接口、管理员接口或用户列表接口。

## 2. 分层关系

用户请求的主要调用链如下：

```text
Gin 路由
  -> JWT 认证中间件（仅受保护接口）
  -> HTTP Handler
  -> application/user.Service
  -> domain/user.User
  -> domain/user.Repository
  -> persistence/user.Repository
  -> GORM / PostgreSQL
```

- `router` 负责初始化 JWT 管理器、用户仓储和应用服务，并注册路由；启动时还会执行 `AutoMigrate`。
- `middleware` 解析 `Authorization: Bearer <token>`，验证访问令牌，并把用户 ID 写入 Gin 上下文。
- `handler/user` 负责解析 JSON、读取认证上下文、调用应用服务和转换 HTTP 响应。
- `application/user` 编排注册、登录、查询资料和更新资料等用例，不直接依赖 GORM。
- `domain/user` 保存用户实体及核心规则，包括输入校验、密码哈希、密码验证和资料更新规则。
- `infrastructure/persistence/user` 实现领域层的 `Repository` 接口，负责领域对象和 GORM 模型之间的转换。

## 3. 已实现接口

| 方法 | 路径 | 鉴权 | 成功响应 | 可能的错误状态 |
| --- | --- | --- | --- | --- |
| `POST` | `/api/users` | 无 | `201`，返回用户资料 | `400`、`409`、`500` |
| `POST` | `/api/sessions` | 无 | `200`，返回访问令牌 | `400`、`401`、`500` |
| `DELETE` | `/api/sessions/current` | Bearer JWT | `204`，无响应体 | `401`、`500` |
| `GET` | `/api/users/me` | Bearer JWT | `200`，返回当前用户资料 | `401`、`404`、`500` |
| `PATCH` | `/api/users/me` | Bearer JWT | `200`，返回更新后的用户资料 | `400`、`401`、`404`、`500` |
| `GET` | `/api/users/{userId}` | 无 | `200`，返回公开用户资料 | `400`、`404`、`500` |

注册和资料更新当前均不支持幂等键。接口路径、字段和响应以 `api/openapi.yaml` 为准。

### 3.1 注册

请求体：

```json
{
  "account": "demo_user",
  "password": "example-password",
  "nickname": "Demo User"
}
```

`account`、`password` 和 `nickname` 均为必填字符串。`account` 和 `nickname` 会去除首尾空白，密码只检查去除空白后是否为空，但合法密码本身的首尾空白会保留并参与校验。

成功响应示例：

```json
{
  "id": 1,
  "account": "demo_user",
  "nickname": "Demo User",
  "role": "user",
  "bio": "",
  "status": 1,
  "avatar_url": "",
  "following_count": 0,
  "follower_count": 0,
  "work_count": 0
}
```

密码在领域层使用 `bcrypt` 哈希后保存，密码明文不会出现在响应中。已存在的账号返回 `409`，请求或领域校验失败返回 `400`。

### 3.2 密码登录

请求体：

```json
{
  "account": "demo_user",
  "password": "example-password"
}
```

成功响应示例：

```json
{
  "access_token": "example-access-token",
  "token_type": "Bearer",
  "expires_in_seconds": 900
}
```

`expires_in_seconds` 由 JWT 配置中的 `access_ttl` 换算得到。当前配置默认访问令牌有效期为 `15m`。账号不存在或密码错误统一返回 `401`，避免暴露账号是否存在。

### 3.3 退出登录

请求必须携带有效的访问令牌：

```http
Authorization: Bearer <access-token>
```

接口返回 `204 No Content`。当前 JWT 是无状态令牌，服务端不维护令牌黑名单；客户端需要删除本地访问令牌，令牌自然过期前不会被此接口单独撤销。

### 3.4 获取当前用户资料

请求必须携带有效的访问令牌。接口通过令牌中的用户 ID 查询用户，并返回：

```json
{
  "id": 1,
  "account": "demo_user",
  "nickname": "Demo User",
  "role": "user",
  "bio": "",
  "status": 1,
  "avatar_url": "",
  "following_count": 0,
  "follower_count": 0,
  "work_count": 3
}
```

`work_count` 是当前用户已发布作品数。关注关系模块尚未实现，因此 `following_count` 和 `follower_count` 当前固定返回 `0`。

### 3.5 获取公开用户资料

该接口无需鉴权，通过路径参数 `userId` 查询其他用户的公开资料：

```http
GET /api/users/42
```

成功响应只包含昵称、头像、简介和统计字段，不返回账号、角色或状态：

```json
{
  "id": 42,
  "nickname": "Demo User",
  "bio": "Video creator",
  "avatar_url": "https://example.com/avatar.png",
  "following_count": 12,
  "follower_count": 34,
  "work_count": 8
}
```

用户不存在返回 `404`，非正整数或无法解析的 `userId` 返回 `400`。

### 3.6 更新当前用户资料

请求体字段均为可选，但至少要提供一个非 `null` 字段：

```json
{
  "nickname": "Updated User",
  "avatar_url": "https://example.com/avatar.png",
  "bio": "Updated profile"
}
```

- `nickname` 不能是空字符串或只包含空白字符；更新时会去除首尾空白。
- `avatar_url` 和 `bio` 不做格式校验，更新时会去除首尾空白；传入空字符串可以清空字段。
- 空对象或所有字段均为 `null` 返回 `400`。
- 该接口不能修改账号、密码、角色或状态。

成功时返回更新后的用户资料，格式与获取资料接口相同。

## 4. 持久化模型

GORM 模型 `UserModel` 映射到 PostgreSQL 的 `users` 表。字段和约束以模型标签为准：

| 数据库列 | Go 类型 | GORM 约束 | 说明 |
| --- | --- | --- | --- |
| `id` | `int64` | 主键，自增 | 用户 ID |
| `account` | `string` | `size:64`、非空、唯一索引 | 登录账号；代码不限制其为邮箱或手机号 |
| `password` | `string` | `size:255`、非空 | bcrypt 密码哈希；领域层字段名为 `Password`，持久化模型字段名为 `PasswordHash` |
| `nickname` | `string` | `size:128`、非空 | 用户昵称 |
| `role` | `string` | `size:32`、非空 | 注册用户默认值为 `user` |
| `avatar_url` | `string` | `size:255` | 头像地址，可为空 |
| `bio` | `string` | `size:255` | 个人简介，可为空 |
| `status` | `int` | 默认值为 `1` | 当前正常状态为 `1` |
| `created_at` | `time.Time` | `autoCreateTime` | 创建时间 |
| `updated_at` | `time.Time` | `autoUpdateTime` | 更新时间 |

项目使用 PostgreSQL 和 GORM，因此本文不使用 MySQL 专用的 `TINYINT`、`DATETIME` 等类型描述。用户注册时由数据库唯一索引保证账号唯一，仓储层会将重复账号错误转换为领域错误。

## 5. 错误响应

接口错误使用统一结构：

```json
{
  "error": "invalid request"
}
```

当前主要错误消息如下：

| 状态码 | `error` 示例 | 含义 |
| --- | --- | --- |
| `400` | `invalid request` | JSON、必填字段或资料更新内容无效 |
| `401` | `invalid credentials` | 登录凭据错误 |
| `401` | `invalid access token` | 访问令牌无效或上下文中没有有效用户 ID |
| `404` | `user not found` | 令牌对应的用户不存在 |
| `409` | `account already exists` | 账号已被注册 |
| `500` | `internal server error` | 服务内部错误 |

认证中间件在缺少或错误的 `Authorization` 请求头时，也会返回 `401`，具体错误消息可能是 `authorization header is required`、`authorization format must be Bearer <token>` 或 `authorization scheme must be Bearer`。

## 6. 相关代码和验证

- 路由注册：`internal/router/register.go`
- HTTP 处理器：`internal/handler/user/`
- 应用服务：`internal/application/user/`
- 领域实体：`internal/domain/user/`
- 持久化实现：`internal/infrastructure/persistence/user/`
- OpenAPI：`api/openapi.yaml`

提交前可运行：

```bash
go test ./...
go vet ./...
git diff --check
```
