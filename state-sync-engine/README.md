# State Sync Engine

The State Sync Engine is a standalone Go service that monitors the local PostgreSQL database using logical replication and asynchronously replicates changes to the cloud backup node.

## Project Structure

```
state-sync-engine/
├── cmd/                      # Application entry points
│   └── state-sync/          # Main application
│       └── main.go
├── internal/                # Private application code
│   ├── config/             # Configuration management
│   ├── replication/        # Replication subscriber/publisher
│   ├── lock/               # Distributed lock implementation
│   └── coordinator/        # State coordination
├── pkg/                     # Public interfaces
│   └── interfaces/         # Core interfaces
└── go.mod                  # Go module file
```

## Core Interfaces

The system defines four core interfaces:

1. **ReplicationSubscriber**: Manages logical replication from local PostgreSQL
2. **ReplicationPublisher**: Transmits changes to cloud database
3. **DistributedLock**: Coordinates active writer designation
4. **StateCoordinator**: Manages database read-only transitions

## Configuration

Configuration is loaded from `config.yaml` and can be overridden with environment variables. See `config.example.yaml` for a complete example.

## Building

```bash
go build -o state-sync ./cmd/state-sync
```

## Running

```bash
./state-sync -config config.yaml
```

## Dependencies

- `github.com/jackc/pgx/v5` - PostgreSQL driver with logical replication support
- `go.uber.org/zap` - Structured logging
- `github.com/spf13/viper` - Configuration management
- `github.com/prometheus/client_golang` - Metrics collection
- `cloud.google.com/go/storage` - GCS for distributed locks

## Credential management and security

### Environment variables for secrets

Do not hardcode credentials in versioned config files. Use environment variables for passwords and secrets:

```yaml
local_database:
  password: ${LOCAL_DB_PASSWORD}
cloud_database:
  password: ${CLOUD_DB_PASSWORD}
  ssl_mode: require
  ssl_root_cert_path: /etc/ssl/certs/cloudsql-ca.pem
```

Required cloud TLS settings:

- `cloud_database.ssl_mode` must be one of `require`, `verify-ca`, `verify-full`
- `cloud_database.ssl_root_cert_path` must point to a CA bundle used for certificate validation

### GCP Secret Manager example

Store a secret:

```bash
echo -n "super-secret-password" | gcloud secrets create cloud-db-password --data-file=-
```

Grant read access to runtime service account:

```bash
gcloud secrets add-iam-policy-binding cloud-db-password \
  --member="serviceAccount:<SERVICE_ACCOUNT_EMAIL>" \
  --role="roles/secretmanager.secretAccessor"
```

Inject secret into environment before startup (example):

```bash
export CLOUD_DB_PASSWORD="$(gcloud secrets versions access latest --secret=cloud-db-password)"
./state-sync -config config.yaml
```

### Cloudflare Tunnel authentication

Use Cloudflare Zero Trust tunnel credentials instead of static exposed endpoints:

1. Authenticate with `cloudflared tunnel login`
2. Create tunnel credentials via `cloudflared tunnel create <name>`
3. Store the generated JSON credentials file securely (outside source control)
4. Run tunnel with explicit credentials/config paths and least-privileged host access
