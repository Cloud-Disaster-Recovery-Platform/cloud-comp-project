package interfaces

import (
	"context"
	"time"
)

// ReplicationSubscriber manages logical replication from local PostgreSQL
type ReplicationSubscriber interface {
	// Connect establishes replication connection and creates slot if needed
	Connect(ctx context.Context, config DBConfig) error

	// Subscribe starts consuming change events from the replication slot
	Subscribe(ctx context.Context, slotName string, tables []string) (<-chan ChangeEvent, error)

	// Acknowledge confirms processing of changes up to LSN
	Acknowledge(ctx context.Context, lsn LSN) error

	// Close gracefully shuts down replication connection
	Close() error
}

// ReplicationPublisher transmits changes to cloud database
type ReplicationPublisher interface {
	// Publish sends a batch of changes to cloud database
	Publish(ctx context.Context, events []ChangeEvent) error

	// HealthCheck verifies cloud database connectivity
	HealthCheck(ctx context.Context) error
}

// DistributedLock coordinates active writer designation
type DistributedLock interface {
	// Acquire attempts to obtain the lock with TTL
	Acquire(ctx context.Context, lockKey string, ttl time.Duration) (bool, error)

	// Renew extends the lock lease
	Renew(ctx context.Context, lockKey string, ttl time.Duration) error

	// Release gives up the lock
	Release(ctx context.Context, lockKey string) error

	// GetHolder returns current lock holder identifier
	GetHolder(ctx context.Context, lockKey string) (string, error)
}

// StateCoordinator manages database read-only transitions
type StateCoordinator interface {
	// SetReadOnly marks a database as read-only
	SetReadOnly(ctx context.Context, db DBConfig) error

	// SetReadWrite enables writes on a database
	SetReadWrite(ctx context.Context, db DBConfig) error

	// GetState returns current read/write state
	GetState(ctx context.Context, db DBConfig) (DatabaseState, error)
}

// ChangeEvent represents a single database modification
type ChangeEvent struct {
	LSN       LSN            // Log Sequence Number for ordering
	Timestamp time.Time      // When change occurred
	Table     string         // Fully qualified table name
	Operation Operation      // INSERT, UPDATE, DELETE
	OldRow    map[string]any // Previous values (UPDATE/DELETE)
	NewRow    map[string]any // New values (INSERT/UPDATE)
}

// LSN represents a PostgreSQL Log Sequence Number
type LSN uint64

// Operation represents a database operation type
type Operation string

const (
	OperationInsert Operation = "INSERT"
	OperationUpdate Operation = "UPDATE"
	OperationDelete Operation = "DELETE"
)

// DatabaseState represents the read/write state of a database
type DatabaseState string

const (
	DatabaseStateReadOnly  DatabaseState = "read_only"
	DatabaseStateReadWrite DatabaseState = "read_write"
)

// DBConfig holds database connection configuration
type DBConfig struct {
	Host            string
	Port            int
	Database        string
	User            string
	Password        string
	SSLMode         string
	SSLRootCertPath string // Optional CA bundle path for TLS certificate verification
	ReplicationSlot string // Optional, only used for local database
}
