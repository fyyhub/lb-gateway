# 轻量级 API 网关开发方案

## 1. 项目目标

开发一个轻量级 API 网关，支持外网环境部署，可通过可视化后台配置 API 代理、负载均衡、请求改写、响应结构映射，以及 Web 页面 302 跳转。

核心目标：

- 请求 `http://localhost/api` 时，根据配置转发到多个后端 API 服务。
- 支持轮询、加权轮询、随机等常见负载均衡策略。
- 支持修改请求信息，例如 Header、Path、Query、Body。
- 支持修改响应信息，尤其是将不同后端的 JSON 响应统一成一种结构。
- 请求 `http://localhost/web` 时，根据配置做 302 跳转到不同 Web 页面。
- 提供可视化管理后台，用于配置路由、上游服务、负载策略和字段映射。
- 适合轻量部署，同时具备外网运行所需的基础安全能力。

## 2. 技术选型

### 2.1 后端网关

推荐使用 Go。

原因：

- 语言轻量，产物可编译为单个二进制文件。
- 性能好，适合做网络代理。
- 标准库自带 HTTP 服务能力。
- 部署简单，适合外网服务器。

建议依赖：

- HTTP 服务：Go 标准库 `net/http`
- 反向代理：Go 标准库 `net/http/httputil`
- 路由：`chi` 或自研轻量路由匹配
- 数据库：SQLite
- SQLite 驱动：`modernc.org/sqlite` 或 `github.com/mattn/go-sqlite3`
- 日志：`zap` 或 `zerolog`
- 配置：JSON / YAML / SQLite

### 2.2 管理后台

推荐使用 React + Vite。

建议依赖：

- 前端框架：React
- 构建工具：Vite
- UI 组件：Ant Design 或 shadcn/ui
- 请求库：fetch 或 axios
- 表单校验：zod
- JSON 编辑器：Monaco Editor 或 JSONEditor

### 2.3 配置存储

第一版使用 SQLite。

原因：

- 轻量，无需单独部署数据库。
- 适合保存路由、上游、规则、日志。
- 后续如果需要多实例部署，可以迁移到 PostgreSQL 或 MySQL。

## 3. 总体架构

```text
Client
  |
  v
Gateway Runtime
  |
  +-- Route Matcher
  |
  +-- Request Rewriter
  |
  +-- Load Balancer
  |
  +-- Proxy / Redirect Handler
  |
  +-- Response Mapper
  |
  v
Backend API / Web Target

Admin Web UI
  |
  v
Admin API
  |
  v
SQLite Config Store
```

建议端口：

- 网关入口：`8080`
- 管理后台：`8081`
- 管理 API：可与管理后台同端口，也可使用 `8082`

## 4. 核心功能设计

### 4.1 路由管理

每条路由包含：

- 路由名称
- 是否启用
- 匹配 Host
- 匹配 Path
- 匹配 Method
- 路由类型：`proxy` 或 `redirect`
- 优先级
- 绑定上游组
- 请求改写规则
- 响应映射规则

示例：

```json
{
  "name": "api-route",
  "enabled": true,
  "priority": 100,
  "match": {
    "host": "localhost",
    "path": "/api/**",
    "methods": ["GET", "POST", "PUT", "DELETE"]
  },
  "type": "proxy",
  "upstreamGroupId": "api-services"
}
```

### 4.2 API 代理

请求示例：

```text
http://localhost:8080/api/user
```

网关处理流程：

1. 匹配 `/api/**` 路由。
2. 执行请求改写规则。
3. 根据负载均衡策略选择一个后端服务。
4. 转发请求到目标 API。
5. 接收响应。
6. 执行响应结构映射。
7. 返回统一结构给客户端。

### 4.3 Web 302 跳转

请求示例：

```text
http://localhost:8080/web
```

配置示例：

```json
{
  "name": "web-redirect",
  "enabled": true,
  "priority": 100,
  "match": {
    "path": "/web",
    "methods": ["GET"]
  },
  "type": "redirect",
  "redirect": {
    "statusCode": 302,
    "strategy": "weighted",
    "targets": [
      { "url": "https://site-a.example.com", "weight": 80 },
      { "url": "https://site-b.example.com", "weight": 20 }
    ]
  }
}
```

处理流程：

1. 匹配 `/web` 路由。
2. 根据轮询、加权或随机策略选择一个目标页面。
3. 返回 `302`。
4. 设置响应头 `Location` 为目标页面地址。

### 4.4 上游服务和负载均衡

上游组配置示例：

```json
{
  "id": "api-services",
  "name": "API 服务组",
  "strategy": "weighted-round-robin",
  "targets": [
    {
      "url": "http://127.0.0.1:9001",
      "weight": 70,
      "enabled": true
    },
    {
      "url": "http://127.0.0.1:9002",
      "weight": 30,
      "enabled": true
    }
  ]
}
```

第一版支持：

- `round-robin`：轮询
- `weighted-round-robin`：加权轮询
- `random`：随机

第二版扩展：

- 健康检查
- 失败重试
- 上游熔断
- 最少连接
- 灰度规则

### 4.5 请求改写

支持改写内容：

- Header
- Path
- Query
- JSON Body 字段
- Method

规则示例：

```json
{
  "requestRewrite": [
    {
      "type": "setHeader",
      "key": "X-Gateway",
      "value": "light-api-gateway"
    },
    {
      "type": "rewritePath",
      "from": "/api",
      "to": "/v1"
    },
    {
      "type": "setQuery",
      "key": "source",
      "value": "gateway"
    }
  ]
}
```

### 4.6 响应结构映射

目标是把不同后端返回结构统一成固定响应结构。

后端 A 返回：

```json
{
  "code": 0,
  "data": {
    "name": "Tom",
    "id": 1
  }
}
```

后端 B 返回：

```json
{
  "success": true,
  "result": {
    "username": "Tom",
    "userId": 1
  }
}
```

统一输出：

```json
{
  "success": true,
  "data": {
    "name": "Tom",
    "id": 1
  }
}
```

映射规则示例：

```json
{
  "responseMapping": [
    {
      "from": "$.result.username",
      "to": "$.data.name"
    },
    {
      "from": "$.result.userId",
      "to": "$.data.id"
    },
    {
      "value": true,
      "to": "$.success"
    }
  ]
}
```

可视化后台建议：

- 左侧展示原始响应 JSON。
- 右侧展示目标响应 JSON。
- 用户选择源字段和目标字段。
- 支持常量值。
- 支持预览转换结果。
- 支持保存映射规则。

第一版不建议开放任意 JavaScript 脚本执行，避免外网环境下产生安全风险。后续如需脚本能力，应使用沙箱、超时、内存限制和权限控制。

## 5. 管理后台设计

### 5.1 登录页

功能：

- 用户名密码登录。
- JWT 或 Session 鉴权。
- 登录失败限制。

### 5.2 仪表盘

展示：

- 总请求量
- 成功率
- 错误率
- 平均耗时
- 最近请求日志
- 上游服务状态

### 5.3 路由管理

功能：

- 新增路由
- 编辑路由
- 启用 / 禁用路由
- 删除路由
- 调整优先级
- 绑定上游组
- 配置代理或 302 跳转

### 5.4 上游服务管理

功能：

- 新增上游组
- 配置负载策略
- 新增目标服务
- 配置权重
- 启用 / 禁用目标
- 测试连接

### 5.5 改写规则管理

功能：

- Header 改写
- Path 改写
- Query 改写
- Body 字段改写
- 响应 Header 改写
- 响应状态码改写

### 5.6 响应映射设计器

功能：

- 粘贴或抓取原始响应 JSON。
- 编辑目标响应 JSON 模板。
- 通过点选完成字段映射。
- 支持常量值。
- 支持预览输出。
- 支持保存为路由响应规则。

### 5.7 调试控制台

功能：

- 输入请求 URL、Method、Header、Body。
- 模拟请求网关。
- 显示命中的路由。
- 显示选中的上游。
- 显示请求改写结果。
- 显示原始响应。
- 显示映射后的响应。

### 5.8 请求日志

记录：

- 请求时间
- 请求 Method
- 请求 Path
- 命中路由
- 目标上游
- 响应状态码
- 耗时
- 错误信息
- 客户端 IP

敏感字段需要脱敏，例如：

- `Authorization`
- `Cookie`
- `Password`
- `Token`

## 6. 数据库设计

### 6.1 routes

保存路由配置。

字段：

- `id`
- `name`
- `enabled`
- `priority`
- `match_host`
- `match_path`
- `match_methods`
- `type`
- `upstream_group_id`
- `request_rewrite_json`
- `response_mapping_json`
- `redirect_config_json`
- `created_at`
- `updated_at`

### 6.2 upstream_groups

保存上游组。

字段：

- `id`
- `name`
- `strategy`
- `created_at`
- `updated_at`

### 6.3 upstream_targets

保存上游目标服务。

字段：

- `id`
- `group_id`
- `url`
- `weight`
- `enabled`
- `health_status`
- `created_at`
- `updated_at`

### 6.4 request_logs

保存请求日志。

字段：

- `id`
- `request_id`
- `method`
- `path`
- `route_id`
- `upstream_url`
- `status_code`
- `duration_ms`
- `client_ip`
- `error`
- `created_at`

### 6.5 admin_users

保存后台用户。

字段：

- `id`
- `username`
- `password_hash`
- `role`
- `enabled`
- `created_at`
- `updated_at`

### 6.6 audit_logs

保存后台操作审计日志。

字段：

- `id`
- `admin_user_id`
- `action`
- `resource_type`
- `resource_id`
- `detail_json`
- `created_at`

## 7. 后端模块划分

建议目录：

```text
light-api-gateway/
  cmd/
    gateway/
      main.go
    admin/
      main.go
  internal/
    adminapi/
    auth/
    config/
    gateway/
    loadbalance/
    logging/
    mapping/
    proxy/
    redirect/
    rewrite/
    router/
    store/
    upstream/
  web-admin/
  configs/
  scripts/
  docs/
```

模块职责：

- `gateway`：网关启动、请求入口、主流程编排。
- `router`：路由匹配和优先级处理。
- `upstream`：上游组和目标服务管理。
- `loadbalance`：轮询、加权轮询、随机策略。
- `proxy`：API 反向代理。
- `redirect`：302 跳转处理。
- `rewrite`：请求和响应改写。
- `mapping`：JSON 字段映射执行。
- `store`：SQLite 数据访问。
- `adminapi`：后台管理 API。
- `auth`：后台登录、鉴权和权限。
- `logging`：请求日志和审计日志。
- `config`：配置加载、热更新。

## 8. API 设计

### 8.1 管理 API

路由管理：

- `GET /admin/api/routes`
- `POST /admin/api/routes`
- `GET /admin/api/routes/:id`
- `PUT /admin/api/routes/:id`
- `DELETE /admin/api/routes/:id`
- `PATCH /admin/api/routes/:id/enabled`

上游管理：

- `GET /admin/api/upstream-groups`
- `POST /admin/api/upstream-groups`
- `GET /admin/api/upstream-groups/:id`
- `PUT /admin/api/upstream-groups/:id`
- `DELETE /admin/api/upstream-groups/:id`
- `POST /admin/api/upstream-groups/:id/targets`
- `PUT /admin/api/upstream-targets/:id`
- `DELETE /admin/api/upstream-targets/:id`

调试：

- `POST /admin/api/debug/request`
- `POST /admin/api/debug/mapping`

日志：

- `GET /admin/api/request-logs`
- `GET /admin/api/audit-logs`

认证：

- `POST /admin/api/auth/login`
- `POST /admin/api/auth/logout`
- `GET /admin/api/auth/me`

### 8.2 网关入口

网关入口不固定写死具体接口，而是匹配用户配置的路由。

示例：

- `/api/**`
- `/web`
- `/custom/**`

## 9. 外网部署安全要求

由于项目用于外网环境，第一版必须包含以下安全能力：

- 管理后台登录认证。
- 管理 API 鉴权。
- 管理后台强密码策略。
- 请求体大小限制。
- 请求超时。
- 上游地址白名单，避免 SSRF 风险。
- 禁止代理到内网敏感地址，除非显式允许。
- 敏感 Header 脱敏。
- 后台操作审计日志。
- CORS 可配置。
- 网关和管理后台端口分离。
- 建议使用 HTTPS，可通过 Caddy、Nginx 或云负载均衡接入证书。

建议生产部署：

```text
Internet
  |
  v
Caddy / Nginx / Cloud Load Balancer
  |
  +-- HTTPS -> Gateway :8080
  |
  +-- HTTPS -> Admin UI :8081
```

## 10. 开发阶段

### 第一阶段：MVP 网关核心

目标：先跑通最小可用网关。

任务：

- [x] 初始化 Go 项目。
- [x] 实现路由配置加载。
- [x] 实现 `/api/**` API 代理。
- [x] 实现 `/web` 302 跳转。
- [x] 实现轮询负载均衡。
- [x] 实现加权轮询负载均衡。
- [x] 实现基础请求日志。
- [x] 使用 JSON 文件保存临时配置。

验收标准：

- [x] 请求 `/api/test` 能转发到配置的 API 服务。
- [x] 多个 API 服务之间能按轮询或权重分发。
- [x] 请求 `/web` 能返回 302 并跳转到目标页面。
- [x] 日志能记录命中路由、目标上游和耗时。

完成记录：

- 2026-06-07：已完成 Go MVP 网关核心。新增 `cmd/gateway`、`cmd/mock-api`、`internal/config`、`internal/router`、`internal/loadbalance`、`internal/rewrite`、`internal/gateway` 和 `configs/config.example.json`。已通过 `go test ./...`、二进制构建，以及本地端到端验证。

### 第二阶段：SQLite 和管理 API

目标：让配置可持久化，可通过 API 管理。

任务：

- [x] 设计并创建 SQLite 表。
- [x] 实现路由 CRUD API。
- [x] 实现上游组 CRUD API。
- [x] 实现上游目标 CRUD API。
- [x] 实现配置热加载。
- [x] 实现后台登录认证。

验收标准：

- [x] 可以通过管理 API 新增、修改、删除路由。
- [x] 修改配置后网关无需重启即可生效。
- [x] 未登录用户不能访问管理 API。

完成记录：

- 2026-06-07：已完成 SQLite 配置持久化和管理 API。新增 `cmd/admin`、`internal/store`、`internal/adminapi`、`internal/auth`。管理 API 支持登录、路由 CRUD、上游组 CRUD、上游目标 CRUD；网关支持 `-db` 从 SQLite 加载配置并按 `-reload-interval` 热刷新。已通过 `go test ./...`、二进制构建，以及本地联调验证新增路由后无需重启即可 302 跳转。

### 第三阶段：请求改写和响应映射

目标：实现网关核心高级能力。

任务：

- [x] 实现 Header 改写。
- [x] 实现 Path 改写。
- [x] 实现 Query 改写。
- [x] 实现 JSON Body 字段改写。
- [x] 实现 JSON 响应字段映射。
- [x] 实现映射预览 API。

验收标准：

- [x] 请求转发前可以按规则修改 Header、Path、Query。
- [x] 不同后端响应可以统一成同一 JSON 结构。
- [x] 管理 API 可以预览映射结果。

完成记录：

- 2026-06-07：已完成请求改写和响应映射阶段。新增 `internal/mapping`，支持简单 JSONPath 风格字段读取和写入；请求规则新增 `setJsonBody`；代理响应新增 `responseMapping` 执行逻辑；管理 API 新增 `POST /admin/api/debug/mapping` 预览接口。已通过 `go test ./...`、二进制构建，以及本地端到端验证两个不同 mock 响应被统一为同一 JSON 结构。

### 第四阶段：React 可视化后台

目标：提供完整可视化配置能力。

任务：

- 初始化 React + Vite 项目。
- 实现登录页。
- 实现仪表盘。
- 实现路由管理页面。
- 实现上游服务管理页面。
- 实现请求改写规则页面。
- 实现响应映射设计器。
- 实现调试控制台。
- 实现请求日志页面。

验收标准：

- 用户可以通过页面完成路由和上游配置。
- 用户可以可视化配置响应字段映射。
- 用户可以在调试控制台测试配置效果。

### 第五阶段：外网安全和稳定性

目标：达到外网基础可用标准。

任务：

- 请求超时控制。
- 请求体大小限制。
- IP 黑白名单。
- 上游地址白名单。
- 敏感信息脱敏。
- 审计日志。
- 健康检查。
- Docker 构建。
- 配置备份和恢复。

验收标准：

- 管理后台和管理 API 有完整鉴权。
- 常见异常请求不会拖垮网关。
- 上游异常时有明确错误和日志。
- 可以通过 Docker 部署。

## 11. 测试方案

### 11.1 单元测试

重点测试：

- 路由匹配
- 路由优先级
- 轮询负载均衡
- 加权负载均衡
- 请求改写
- 响应映射
- 302 跳转目标选择

### 11.2 集成测试

准备两个模拟 API 服务：

- `mock-api-a`
- `mock-api-b`

测试：

- `/api/**` 能正确转发。
- 权重配置符合预期分布。
- 后端响应结构能被统一转换。
- 上游不可用时能记录错误。

### 11.3 前端测试

测试：

- 路由表单校验。
- 上游权重配置。
- 响应映射预览。
- 调试控制台。
- 登录鉴权。

## 12. 部署方案

### 12.1 本地开发

```text
Gateway: http://localhost:8080
Admin UI: http://localhost:8081
Admin API: http://localhost:8082
SQLite: ./data/gateway.db
```

### 12.2 外网部署

推荐：

- 使用 Docker 部署 Go 网关和管理后台。
- 使用 Caddy 或 Nginx 做 HTTPS。
- 管理后台绑定独立域名或独立路径。
- 管理后台限制 IP 访问。

示例：

```text
https://gateway.example.com      -> Gateway :8080
https://gateway-admin.example.com -> Admin UI / Admin API
```

## 13. MVP 配置示例

```json
{
  "routes": [
    {
      "name": "api-route",
      "enabled": true,
      "priority": 100,
      "type": "proxy",
      "match": {
        "path": "/api/**",
        "methods": ["GET", "POST"]
      },
      "upstreamGroup": {
        "strategy": "weighted-round-robin",
        "targets": [
          {
            "url": "http://127.0.0.1:9001",
            "weight": 70
          },
          {
            "url": "http://127.0.0.1:9002",
            "weight": 30
          }
        ]
      },
      "requestRewrite": [
        {
          "type": "rewritePath",
          "from": "/api",
          "to": "/v1"
        }
      ],
      "responseMapping": [
        {
          "from": "$.result.username",
          "to": "$.data.name"
        },
        {
          "from": "$.result.userId",
          "to": "$.data.id"
        },
        {
          "value": true,
          "to": "$.success"
        }
      ]
    },
    {
      "name": "web-redirect",
      "enabled": true,
      "priority": 90,
      "type": "redirect",
      "match": {
        "path": "/web",
        "methods": ["GET"]
      },
      "redirect": {
        "statusCode": 302,
        "strategy": "weighted",
        "targets": [
          {
            "url": "https://site-a.example.com",
            "weight": 80
          },
          {
            "url": "https://site-b.example.com",
            "weight": 20
          }
        ]
      }
    }
  ]
}
```

## 14. 风险和注意事项

- 响应映射需要限制只处理 JSON 响应，非 JSON 响应默认不改写。
- 大响应体改写会占用内存，需要设置最大响应体大小。
- 请求 Body 改写需要读取完整 Body，也需要大小限制。
- 外网环境下不能随意开放脚本执行能力。
- 代理目标必须做白名单或限制，避免 SSRF。
- 管理后台必须有认证和审计。
- 配置热更新需要保证并发安全，避免请求处理中读到半更新配置。

## 15. 推荐第一版范围

第一版建议只做以下功能：

- Go 网关服务。
- React 管理后台。
- SQLite 配置库。
- `/api/**` 反向代理。
- `/web` 302 跳转。
- 轮询、加权轮询、随机。
- Header / Path / Query 改写。
- JSON 响应字段映射。
- 路由和上游可视化管理。
- 映射规则可视化配置和预览。
- 请求日志。
- 管理后台登录。
- Docker 部署。

暂不做：

- 多实例配置同步。
- 插件系统。
- 任意脚本执行。
- 复杂鉴权网关。
- 全量链路追踪。
- 服务注册发现。

这些能力可以在第二个大版本中扩展。
