FROM golang:1.25-bookworm AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=0 GOOS=linux go build -o /out/gateway ./cmd/gateway
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/admin ./cmd/admin
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/mock-api ./cmd/mock-api

FROM debian:bookworm-slim AS runtime

WORKDIR /app

RUN apt-get update \
	&& apt-get install -y --no-install-recommends wget ca-certificates \
	&& rm -rf /var/lib/apt/lists/*

COPY --from=builder /out/gateway /app/bin/gateway
COPY --from=builder /out/admin /app/bin/admin
COPY --from=builder /out/mock-api /app/bin/mock-api
COPY configs /app/configs
COPY scripts /app/scripts

RUN useradd --system --uid 10001 --home /app gateway \
	&& mkdir -p /app/data \
	&& chmod +x /app/scripts/wait-for-db.sh \
	&& chown -R gateway:gateway /app

USER gateway

EXPOSE 8080 8082 9001 9002

CMD ["/app/bin/gateway", "-config", "/app/configs/config.docker-runtime.json", "-db", "/app/data/gateway.db"]
