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
