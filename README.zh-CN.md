# Light API Gateway

语言：[English](README.md) | [简体中文](README.zh-CN.md)

Light API Gateway 是一个轻量级 Go API 网关 MVP，支持 API 反向代理、302 Web 跳转、SQLite 持久化路由管理，以及内置 React 管理后台。

## 功能特性

- 基于 JSON 的路由配置。
- SQLite 持久化路由、上游分组和上游目标。
- `/api/**` 反向代理示例路由。
- `/web` 302 跳转示例路由。
- 支持轮询、加权轮询、随机负载均衡。
- 请求改写：Header、Query、Path、JSON Body。
- 响应映射：支持简单 JSONPath 风格的 `from` / `to` 路径。
- 管理后台可预览响应映射结果。
- 仅允许 loopback 目标的调试请求 API，便于本地验证网关行为。
- 请求日志输出到控制台并写入 SQLite。
- SQLite 模式下自动进行上游健康检查。
- 负载均衡会跳过标记为 `unhealthy` 的上游目标。
- 管理 API 登录与 Bearer Token 鉴权。
- 管理 API 支持路由、上游分组、上游目标 CRUD。
- React 管理后台支持路由、上游、响应映射、调试请求、请求日志、审计日志和账户设置。
- 网关可从 SQLite 热加载配置，无需重启即可应用路由变更。
- 提供本地 mock API 服务用于验证。
- 支持单端口一体化模式：网关、管理 API、管理 UI 共用一个端口。

## 快速开始：单端口一体化运行

`server` 二进制会在一个端口上同时提供：

- 网关数据面：`/`，包括你配置的路由，例如 `/api/**`、`/web`
- 管理后台：`/admin`
- 管理 API：`/admin/api`

先构建管理后台并嵌入 Go 二进制。构建产物会复制到 `internal/webui/dist`，随后由 `go build` 嵌入：

```powershell
cd web-admin
npm install
npm run build
cd ..
Copy-Item web-admin/dist/* internal/webui/dist/ -Recurse -Force
```

启动一体化服务。首次启动会创建 `data/gateway.db`；当路由表为空时，会从 `-config` 指定的配置文件初始化路由；当管理员表为空时，会创建启动管理员账号：

```powershell
$env:GATEWAY_ADMIN_PASSWORD="change-this-password"
$env:GATEWAY_ADMIN_SECRET="change-this-token-secret"
go run ./cmd/server -config configs/config.example.json -db data/gateway.db
```

打开 `http://localhost:8080/admin`，使用启动管理员账号登录。管理后台默认同源调用管理 API，因此「管理 API」输入框可以留空。

管理后台开发时，可以使用 Vite 热更新。Vite 会把 `/admin/api` 代理到 `:8080` 上的一体化服务：

```powershell
cd web-admin
npm run dev
```

然后打开 `http://127.0.0.1:8081/admin/`。

## 本地运行基础网关

启动两个 mock API 服务：

```powershell
go run ./cmd/mock-api -listen :9001 -name mock-api-a -shape a
go run ./cmd/mock-api -listen :9002 -name mock-api-b -shape b
```

启动网关：

```powershell
go run ./cmd/gateway -config configs/config.example.json
```

验证 API 代理：

```powershell
Invoke-RestMethod http://localhost:8080/api/users
```

示例路由会把不同上游返回结构映射成统一响应：

```json
{
  "success": true,
  "data": {
    "name": "Tom",
    "id": 1
  }
}
```

验证 302 跳转：

```powershell
Invoke-WebRequest http://localhost:8080/web -MaximumRedirection 0
```

## 分离模式运行

分离模式会把网关、管理 API、管理 UI 作为独立进程运行，适合需要分别扩展或部署数据面与管理面的场景。

启动管理 API。首次启动会创建 `data/gateway.db`；当路由表为空时，会从 `configs/config.example.json` 初始化路由；当管理员表为空时，会创建启动管理员账号：

```powershell
$env:GATEWAY_ADMIN_PASSWORD="change-this-password"
$env:GATEWAY_ADMIN_SECRET="change-this-token-secret"
go run ./cmd/admin -db data/gateway.db -seed-config configs/config.example.json
```

从 SQLite 启动网关：

```powershell
go run ./cmd/gateway -config configs/config.example.json -db data/gateway.db
```

设置 `-db` 后，网关默认每 10 秒探测一次启用的上游目标，把 `healthy` 或 `unhealthy` 写回 SQLite，并在负载均衡时跳过 `unhealthy` 目标。可以用 `-health-check-interval 0` 关闭健康检查，或用 `-health-check-timeout 2s` 调整超时时间。

启动 React 管理后台：

```powershell
cd web-admin
npm install
npm run dev
```

打开 `http://127.0.0.1:8081`，管理 API 地址填写 `http://localhost:8082`，然后使用启动管理员账号登录。

登录管理 API：

```powershell
$login = Invoke-RestMethod `
  -Method Post `
  -Uri http://localhost:8082/admin/api/auth/login `
  -ContentType application/json `
  -Body '{"username":"admin","password":"change-this-password"}'

$headers = @{ Authorization = "Bearer $($login.token)" }
```

查看路由：

```powershell
Invoke-RestMethod http://localhost:8082/admin/api/routes -Headers $headers
```

查看最近请求日志：

```powershell
Invoke-RestMethod http://localhost:8082/admin/api/request-logs?limit=100 -Headers $headers
```

查看最近审计日志：

```powershell
Invoke-RestMethod http://localhost:8082/admin/api/audit-logs?limit=100 -Headers $headers
```

预览响应映射：

```powershell
$previewBody = @{
  source = @{ result = @{ username = "Alice"; userId = 99 } }
  rules = @(
    @{ from = '$.result.username'; to = '$.data.name' }
    @{ from = '$.result.userId'; to = '$.data.id' }
    @{ value = $true; to = '$.success' }
  )
} | ConvertTo-Json -Depth 8

Invoke-RestMethod `
  -Method Post `
  -Uri http://localhost:8082/admin/api/debug/mapping `
  -Headers $headers `
  -ContentType application/json `
  -Body $previewBody
```

通过管理 API 发送 loopback 调试请求：

```powershell
$debugBody = @{
  url = "http://localhost:8080/api/users"
  method = "GET"
  headers = @{ "X-Debug-Source" = "admin-ui" }
} | ConvertTo-Json -Depth 6

Invoke-RestMethod `
  -Method Post `
  -Uri http://localhost:8082/admin/api/debug/request `
  -Headers $headers `
  -ContentType application/json `
  -Body $debugBody
```

调试请求 API 只允许访问 `localhost` 或 loopback 目标。

新增 JSON Body 改写规则示例：

```json
{
  "type": "setJsonBody",
  "key": "$.meta.source",
  "value": "gateway"
}
```

创建 302 跳转路由：

```powershell
$body = @{
  name = "admin-test-redirect"
  enabled = $true
  priority = 200
  type = "redirect"
  match = @{ path = "/admin-test"; methods = @("GET") }
  redirect = @{
    statusCode = 302
    strategy = "round-robin"
    targets = @(@{ url = "https://example.org"; weight = 1; enabled = $true })
  }
} | ConvertTo-Json -Depth 8

Invoke-RestMethod `
  -Method Post `
  -Uri http://localhost:8082/admin/api/routes `
  -Headers $headers `
  -ContentType application/json `
  -Body $body
```

网关会轮询 SQLite 并自动应用变更，无需重启。

## 使用 Docker Compose 运行

构建并启动一体化服务和两个 mock 上游 API：

```powershell
docker compose up --build
```

启动后所有能力都在一个端口上：

- 网关：`http://localhost:8080`
- 管理后台：`http://localhost:8080/admin`
- 管理 API：`http://localhost:8080/admin/api`

默认管理员账号：

```text
admin / admin123456
```

Compose 栈会把 SQLite 数据保存到 `gateway-data` Docker volume。需要重新初始化数据库时执行：

```powershell
docker compose down -v
docker compose up --build
```

也可以在启动前覆盖管理员初始化配置：

```powershell
$env:GATEWAY_ADMIN_USERNAME="admin"
$env:GATEWAY_ADMIN_PASSWORD="change-this-password"
$env:GATEWAY_ADMIN_SECRET="change-this-token-secret"
docker compose up --build
```

Docker seed 配置使用容器服务名，例如 `http://mock-api-a:9001`，因此它和本地运行使用的 `configs/config.example.json` 是分开的。

## 构建与测试

运行 Go 测试：

```powershell
go test ./...
```

构建管理后台：

```powershell
cd web-admin
npm install
npm run build
```

## 目录结构

```text
cmd/
  admin/       管理 API 入口
  gateway/     网关入口
  mock-api/    本地 mock 上游服务
  server/      单端口一体化入口
configs/       示例配置
internal/      网关、管理 API、存储、路由、改写、映射等内部模块
web-admin/     React 管理后台
data/          本地 SQLite 数据目录
```

## 安全提示

- 生产环境请务必修改 `GATEWAY_ADMIN_PASSWORD` 和 `GATEWAY_ADMIN_SECRET`。
- 管理 API 建议只暴露在可信网络内，或放在额外的认证与访问控制之后。
- 调试请求 API 已限制为 loopback 目标，但仍建议只在受信环境使用。
