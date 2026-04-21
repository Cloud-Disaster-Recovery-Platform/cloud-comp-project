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

## Prerequisites

- Go 1.21+
- PostgreSQL 14+ (Local & Cloud)
- GCS bucket (for distributed locks)
- `cloudflared` (optional, for secure tunneling)

## Installation

1. Clone the repository
2. Install dependencies:
   ```bash
   go mod download
   ```
3. Build the binary:
   ```bash
   make build
   ```

## Local PostgreSQL Setup

### 1. Enable Logical Replication

The local PostgreSQL instance must have logical replication enabled. Edit your `postgresql.conf`:

```conf
wal_level = logical
max_replication_slots = 5
max_wal_senders = 5
```

Restart PostgreSQL after making these changes.

### 2. User Permissions

The replication user requires specific permissions to manage slots and publications.

```sql
-- Create a dedicated replication user
CREATE USER replicator WITH REPLICATION PASSWORD 'your_password';

-- Grant access to the application database
GRANT CONNECT ON DATABASE myapp TO replicator;

-- Grant permissions on tables to be replicated
GRANT USAGE ON SCHEMA public TO replicator;
GRANT SELECT ON ALL TABLES IN SCHEMA public TO replicator;

-- (Optional) If you want the engine to create publications automatically:
ALTER USER replicator SET default_transaction_read_only = off;
-- Grant OWNERSHIP or enough privilege to CREATE PUBLICATION
```

### 3. Manual Replication Slot (Optional)

The engine will attempt to create the slot automatically if it doesn't exist, but you can create it manually:

```sql
SELECT pg_create_logical_replication_slot('failsafe_slot', 'pgoutput');
```

## Configuration

Configuration is loaded from `config.yaml` and can be overridden with environment variables. 

1. Copy the example config:
   ```bash
   cp config.example.yaml config.yaml
   ```
2. Update the values in `config.yaml` to match your environment.
3. Use environment variables for secrets (prefixed with `STATE_SYNC_`):
   ```bash
   export STATE_SYNC_LOCAL_DATABASE_PASSWORD=your_local_pass
   export STATE_SYNC_CLOUD_DATABASE_PASSWORD=your_cloud_pass
   ```

## Running

```bash
./bin/state-sync-engine -config config.yaml
```

## Docker setup

A multi-stage `Dockerfile` is provided for containerized deployments.

```bash
make docker-build
docker run -v $(pwd)/config.yaml:/app/config.yaml cloud-mirror/state-sync-engine
```

## Core Interfaces

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
