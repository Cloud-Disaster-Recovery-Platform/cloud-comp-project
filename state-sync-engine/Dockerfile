################################################################################
# Stage 1 — Build the Go binary
################################################################################
FROM golang:1.20-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o /state-sync-engine ./cmd/state-sync

################################################################################
# Stage 2 — Minimal runtime image
################################################################################
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

# Non-root user
RUN addgroup -S sse && adduser -S sse -G sse
USER sse

WORKDIR /app
COPY --from=builder /state-sync-engine /app/state-sync-engine
COPY config.example.yaml /app/config.yaml

EXPOSE 8080 9090

HEALTHCHECK --interval=15s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -qO- http://localhost:8080/status || exit 1

ENTRYPOINT ["/app/state-sync-engine"]
CMD ["-config", "/app/config.yaml"]
