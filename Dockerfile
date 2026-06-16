# Stage 1: Builder
FROM golang:1.26-alpine AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-w -s" \
    -o wanpey \
    ./cmd/api

# Stage 2: Runtime
FROM alpine:3.20

# ca-certificates required for TLS to payment providers; tzdata for correct timestamps
RUN apk --no-cache add ca-certificates tzdata

# Run as a non-root user — reduces blast radius if the process is compromised
RUN addgroup -S wanpey && adduser -S wanpey -G wanpey

WORKDIR /app

COPY --from=builder --chown=wanpey:wanpey /build/wanpey .

USER wanpey

# Config must be mounted at runtime via volume or CONFIG_PATH env var.
# Never bake .config.toml into the image — it contains credentials.
EXPOSE 8080

ENTRYPOINT ["./wanpey", "serve"]
