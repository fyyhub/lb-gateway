# Light API Gateway

Light API Gateway is a lightweight Go gateway MVP for API proxying, 302 web redirects, and SQLite-backed route management.

Implemented so far:

- JSON-based route configuration.
- SQLite-backed route and upstream configuration.
- `/api/**` reverse proxy.
- `/web` 302 redirect.
- Round-robin, weighted round-robin, and random target picking.
- Request rewrites for Header, Query, Path, and JSON Body fields.
- JSON response mapping with simple JSONPath-style `from` and `to` paths.
- Mapping preview API for admin/debug tooling.
- Loopback-only debug request API for local gateway verification.
- Request logging to console and SQLite.
- Automatic upstream health checks in SQLite mode.
- Load balancing skips upstream targets marked `unhealthy`.
- Admin API login with Bearer token auth.
- Admin API CRUD for routes, upstream groups, and upstream targets.
- React admin UI for routes, upstreams, mapping previews, and debug requests.
- Gateway hot reload from SQLite.
- Local mock API services for verification.

## Run Locally

Start two mock API services:

```powershell
go run ./cmd/mock-api -listen :9001 -name mock-api-a -shape a
go run ./cmd/mock-api -listen :9002 -name mock-api-b -shape b
```

Start the gateway:

```powershell
go run ./cmd/gateway -config configs/config.example.json
```

Verify API proxying:

```powershell
Invoke-RestMethod http://localhost:8080/api/users
```

The example route maps different upstream response shapes into a shared response:

```json
{
  "success": true,
  "data": {
    "name": "Tom",
    "id": 1
  }
}
```

Verify 302 redirect:

```powershell
Invoke-WebRequest http://localhost:8080/web -MaximumRedirection 0
```

## Run With SQLite And Admin API

Start the admin API. It creates `data/gateway.db`, seeds routes from `configs/config.example.json` when the routes table is empty, and creates a bootstrap admin user when no admin users exist.

```powershell
$env:GATEWAY_ADMIN_PASSWORD="change-this-password"
$env:GATEWAY_ADMIN_SECRET="change-this-token-secret"
go run ./cmd/admin -db data/gateway.db -seed-config configs/config.example.json
```

Start the gateway from SQLite:

```powershell
go run ./cmd/gateway -config configs/config.example.json -db data/gateway.db
```

When `-db` is set, the gateway probes enabled upstream targets every 10 seconds by default, writes `healthy` or `unhealthy` back to SQLite, and skips `unhealthy` targets during load balancing. Use `-health-check-interval 0` to disable checks, or tune `-health-check-timeout 2s`.

Start the React admin UI:

```powershell
cd web-admin
npm install
npm run dev
```

Open `http://127.0.0.1:8081`, use `http://localhost:8082` as the Admin API URL, and sign in with the bootstrap admin credentials.

Login to the admin API:

```powershell
$login = Invoke-RestMethod `
  -Method Post `
  -Uri http://localhost:8082/admin/api/auth/login `
  -ContentType application/json `
  -Body '{"username":"admin","password":"change-this-password"}'

$headers = @{ Authorization = "Bearer $($login.token)" }
```

List routes:

```powershell
Invoke-RestMethod http://localhost:8082/admin/api/routes -Headers $headers
```

List recent request logs:

```powershell
Invoke-RestMethod http://localhost:8082/admin/api/request-logs?limit=100 -Headers $headers
```

List recent audit logs:

```powershell
Invoke-RestMethod http://localhost:8082/admin/api/audit-logs?limit=100 -Headers $headers
```

Preview a response mapping:

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

Send a loopback debug request through the admin API:

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

The debug request API only allows `localhost` or loopback targets.

Add a JSON Body rewrite rule to a proxy route:

```json
{
  "type": "setJsonBody",
  "key": "$.meta.source",
  "value": "gateway"
}
```

Create a 302 redirect route:

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

The gateway polls SQLite and applies changes without restart.

## Run With Docker Compose

Build and start the gateway, admin API, React admin UI, and two mock upstream APIs:

```powershell
docker compose up --build
```

Open:

- Gateway: `http://localhost:8080`
- Admin UI: `http://localhost:8081`
- Admin API: `http://localhost:8082`

Default admin credentials:

```text
admin / admin123456
```

The compose stack stores SQLite data in the `gateway-data` Docker volume. To start from a fresh seeded database:

```powershell
docker compose down -v
docker compose up --build
```

You can override bootstrap values before startup:

```powershell
$env:GATEWAY_ADMIN_USERNAME="admin"
$env:GATEWAY_ADMIN_PASSWORD="change-this-password"
$env:GATEWAY_ADMIN_SECRET="change-this-token-secret"
docker compose up --build
```

The Docker seed config uses container service names such as `http://mock-api-a:9001`, so it is separate from the local `configs/config.example.json`.

Run tests:

```powershell
go test ./...
cd web-admin
npm run build
```
