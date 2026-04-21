# Container Packaging Guide

This guide provides templates and best practices for containerizing applications to be deployed on the Cloud Mirror failsafe infrastructure (specifically for Google Cloud Run).

## General Requirements

To ensure compatibility with Cloud Run and the Cloud Mirror failover logic, your containers should:
1. **Listen on PORT**: Use the `PORT` environment variable (default 8080).
2. **Stateless**: Ensure the application does not rely on local disk persistence.
3. **Health Check**: Provide a `/health` endpoint for DNS-based health checks.
4. **Configuration**: Use environment variables for all secrets and connection strings.

## Dockerfile Templates

### Node.js (Express)

Based on the `demo-local-node` implementation.

```dockerfile
FROM node:20-slim

WORKDIR /app

COPY package*.json ./
RUN npm ci --only=production

COPY . .

# Run as non-root user for security
USER node

EXPOSE 3000
CMD [ "npm", "start" ]
```

### Go

Based on the `state-sync-engine` implementation.

```dockerfile
# Stage 1 — Build
FROM golang:1.21-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /app-binary ./cmd/main.go

# Stage 2 — Runtime
FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
RUN addgroup -S appgroup && adduser -S appuser -G appgroup
USER appuser
WORKDIR /app
COPY --from=builder /app-binary /app/app-binary
EXPOSE 8080
ENTRYPOINT ["/app/app-binary"]
```

### Python (FastAPI/Flask)

```dockerfile
FROM python:3.11-slim

WORKDIR /app

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY . .

# Use gunicorn or uvicorn for production
CMD ["uvicorn", "main:app", "--host", "0.0.0.0", "--port", "8080"]
```

## Environment Variables for Cloud Run

When deploying to Cloud Run via Terraform, the following variables are typically injected:

| Variable | Description |
| --- | --- |
| `DATABASE_URL` | Connection string for Cloud SQL (Private IP) |
| `NODE_ENV` / `APP_ENV` | Set to `production` |
| `LOG_LEVEL` | Set to `info` or `warn` |

## Connecting to Cloud SQL

Cloud Run connects to Cloud SQL using the **Cloud SQL Auth Proxy** or via **Direct VPC Egress**. The Cloud Mirror infrastructure uses Direct VPC Egress to the private IP of the Cloud SQL instance.

Ensure your database connection logic handles:
- **Connection Retries**: Cloud SQL may have brief interruptions.
- **TLS**: Use `sslmode=require` for secure connections.
