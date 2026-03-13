# Cloud Mirror

A headless failsafe infrastructure that provides 100% uptime with zero cloud costs during idle states. This system integrates with existing local applications and PostgreSQL databases, providing automatic failover to a cloud backup node while maintaining data consistency through asynchronous replication.

## Overview

Cloud Mirror is designed to be language-agnostic and non-invasive—it works with any existing application and database without requiring code changes or schema modifications. The architecture consists of three primary components:

1. **State Sync Engine**: A standalone Go service that monitors the local PostgreSQL database using logical replication and asynchronously replicates changes to the cloud backup node
2. **Cloud Backup Node**: GCP infrastructure (Cloud Run + Cloud SQL) that mirrors the local application and database, scaling to zero during normal operation
3. **Health Monitoring & Failover**: DNS-based health checks and traffic routing that automatically redirects to the cloud backup when the local application fails

## Key Features

- **Zero Cloud Costs During Idle**: Cloud Backup Node scales to zero instances when not needed
- **Automatic Failover**: DNS-based health checks detect local failures and redirect traffic within ~60 seconds
- **Data Consistency**: Asynchronous logical replication ensures eventual consistency with configurable lag thresholds
- **Split-Brain Prevention**: Distributed lock mechanism prevents conflicting writes during failover
- **Non-Invasive**: Works with existing applications without code modifications
- **Observable**: Structured logging, Prometheus metrics, and status endpoints for monitoring
- **Resilient**: Exponential backoff, circuit breaker, and graceful degradation for error recovery

## Architecture

### Normal Operation (Local Active)
- Local application serves all traffic via Cloudflare Tunnel
- Local database accepts all writes
- State Sync Engine streams changes to Cloud SQL
- Cloud Backup Node scaled to zero (no cost)
- Health checks continuously validate local application

### Failover (Cloud Active)
- Health checks detect local application failure
- DNS redirects traffic to Cloud Backup Node
- Cloud Run scales from zero to active instances
- State Sync Engine acquires distributed lock
- Local database marked read-only (if accessible)
- Cloud Backup Node serves requests using mirrored data

### Recovery (Return to Local)
- Health checks detect local application recovery
- State Sync Engine synchronizes cloud-side changes to local
- DNS redirects traffic back to local application
- Cloud Backup Node scales back to zero

## Technology Stack

### State Sync Engine
- **Language**: Go 1.21+
- **PostgreSQL Driver**: pgx/v5 (native logical replication support)
- **Metrics**: Prometheus client library
- **Logging**: go.uber.org/zap (structured JSON logging)
- **Configuration**: github.com/spf13/viper (env vars + YAML/JSON)

### Cloud Infrastructure
- **Compute**: Google Cloud Run (serverless containers)
- **Database**: Google Cloud SQL (PostgreSQL 15)
- **Networking**: Google Cloud VPC with private IP
- **DNS**: Google Cloud DNS with health checks
- **Storage**: Google Cloud Storage (distributed locks)
- **Infrastructure as Code**: Terraform

### Edge & Tunneling
- **DNS Provider**: Cloudflare
- **Tunneling**: Cloudflare Tunnel (cloudflared)

## Getting Started

### Prerequisites
- Local PostgreSQL database (9.4+)
- GCP project with billing enabled
- Cloudflare account with domain
- Go 1.21+ (for building State Sync Engine)
- Terraform 1.0+ (for infrastructure provisioning)

### Quick Start

1. **Clone the repository**
   ```bash
   git clone <repository-url>
   cd cloud-comp-project
   ```

2. **Set up local environment**
   ```bash
   # Configure PostgreSQL replication user
   psql -U postgres -c "CREATE USER replicator WITH REPLICATION ENCRYPTED PASSWORD 'password';"
   psql -U postgres -c "GRANT CONNECT ON DATABASE myapp TO replicator;"
   ```

3. **Deploy cloud infrastructure**
   ```bash
   cd terraform
   terraform init
   terraform apply -var-file=environments/prod/terraform.tfvars
   ```

4. **Build and run State Sync Engine**
   ```bash
   cd state-sync-engine
   go build -o state-sync
   ./state-sync -config config.yaml
   ```

5. **Configure Cloudflare Tunnel**
   ```bash
   cloudflared tunnel create failsafe
   cloudflared tunnel route dns failsafe app.example.com
   cloudflared tunnel run failsafe
   ```

## Configuration

The State Sync Engine is configured via `config.yaml`:

```yaml
local_database:
  host: localhost
  port: 5432
  database: myapp
  user: replicator
  password: ${LOCAL_DB_PASSWORD}
  ssl_mode: require

cloud_database:
  host: 10.0.0.3
  port: 5432
  database: myapp
  user: replicator
  password: ${CLOUD_DB_PASSWORD}
  ssl_mode: require

replication:
  tables:
    - public.users
    - public.orders
  batch_size: 100
  flush_interval: 1s
  max_lag_seconds: 30

distributed_lock:
  gcs_bucket: failsafe-locks
  lock_key: active-writer
  ttl: 30s
  renew_interval: 10s

health:
  status_port: 8080
  metrics_port: 9090
  check_interval: 10s

failover:
  timeout: 60s
  consecutive_failures: 3
```

## Monitoring

### Metrics Endpoint
Access Prometheus metrics at `http://localhost:9090/metrics`:
- `state_sync_replication_lag_seconds` - Current replication lag
- `state_sync_events_processed_total` - Total change events processed
- `state_sync_events_failed_total` - Failed replication attempts
- `state_sync_active_node` - Current active node (0=local, 1=cloud)
- `state_sync_failover_events_total` - Number of failover events
- `state_sync_split_brain_events_total` - Split-brain condition detections

### Status Endpoint
Check system health at `http://localhost:8080/status`:
```json
{
  "active_node": "local",
  "replication_lag": 0.5,
  "lock_holder": "local-node-1",
  "last_failover_time": "2024-03-13T10:30:00Z",
  "events_processed": 15234
}
```