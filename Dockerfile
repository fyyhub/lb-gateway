# --- Stage 1: build the admin UI ---
FROM node:24-bookworm-slim AS web

WORKDIR /web
COPY web-admin/package.json web-admin/package-lock.json ./
RUN npm ci
COPY web-admin ./
RUN npm run build

# --- Stage 2: build the Go binaries (UI embedded into the server) ---
FROM golang:1.25-bookworm AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal
# Embed the built admin UI into the server binary.
COPY --from=web /web/dist ./internal/webui/dist

RUN CGO_ENABLED=0 GOOS=linux go build -o /out/server ./cmd/server
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/mock-api ./cmd/mock-api

# --- Stage 3: runtime ---
FROM debian:bookworm-slim AS runtime

WORKDIR /app

RUN apt-get update \
	&& apt-get install -y --no-install-recommends wget ca-certificates \
	&& rm -rf /var/lib/apt/lists/*

COPY --from=builder /out/server /app/bin/server
COPY --from=builder /out/mock-api /app/bin/mock-api
COPY configs /app/configs

RUN useradd --system --uid 10001 --home /app gateway \
	&& mkdir -p /app/data \
	&& chown -R gateway:gateway /app

USER gateway

# Single port: gateway data plane at /, admin UI at /admin, admin API at /admin/api.
EXPOSE 8080

CMD ["/app/bin/server", "-config", "/app/configs/config.docker.json", "-db", "/app/data/gateway.db"]
