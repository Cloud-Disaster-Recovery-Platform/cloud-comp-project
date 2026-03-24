package config

import (
	"time"
)

// Config represents the complete configuration for the State Sync Engine
type Config struct {
	LocalDatabase    DatabaseConfig    `mapstructure:"local_database"`
	CloudDatabase    DatabaseConfig    `mapstructure:"cloud_database"`
	Replication      ReplicationConfig `mapstructure:"replication"`
	DistributedLock  LockConfig        `mapstructure:"distributed_lock"`
	Health           HealthConfig      `mapstructure:"health"`
	Failover         FailoverConfig    `mapstructure:"failover"`
}

// DatabaseConfig holds database connection parameters
type DatabaseConfig struct {
	Host            string `mapstructure:"host"`
	Port            int    `mapstructure:"port"`
	Database        string `mapstructure:"database"`
	User            string `mapstructure:"user"`
	Password        string `mapstructure:"password"`
	SSLMode         string `mapstructure:"ssl_mode"`
	ReplicationSlot string `mapstructure:"replication_slot"` // Only for local database
}

// ReplicationConfig holds replication behavior parameters
type ReplicationConfig struct {
	Tables        []string      `mapstructure:"tables"`
	BatchSize     int           `mapstructure:"batch_size"`
	FlushInterval time.Duration `mapstructure:"flush_interval"`
	MaxLagSeconds int           `mapstructure:"max_lag_seconds"`
}

// LockConfig holds distributed lock parameters
type LockConfig struct {
	GCSBucket     string        `mapstructure:"gcs_bucket"`
	LockKey       string        `mapstructure:"lock_key"`
	TTL           time.Duration `mapstructure:"ttl"`
	RenewInterval time.Duration `mapstructure:"renew_interval"`
}

// HealthConfig holds health check and status endpoint parameters
type HealthConfig struct {
	StatusPort    int           `mapstructure:"status_port"`
	MetricsPort   int           `mapstructure:"metrics_port"`
	CheckInterval time.Duration `mapstructure:"check_interval"`
}

// FailoverConfig holds failover behavior parameters
type FailoverConfig struct {
	Timeout             time.Duration `mapstructure:"timeout"`
	ConsecutiveFailures int           `mapstructure:"consecutive_failures"`
}
