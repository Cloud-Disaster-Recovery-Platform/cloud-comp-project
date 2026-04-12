package config

import (
"errors"
"fmt"
"strings"
"time"

"github.com/spf13/viper"
)

const (
defaultEnvPrefix  = "STATE_SYNC"
defaultConfigName = "config"
)

// Config represents the complete configuration for the State Sync Engine
// and matches the schema documented in docs/design.md.
type Config struct {
LocalDatabase   DatabaseConfig    `mapstructure:"local_database"`
CloudDatabase   DatabaseConfig    `mapstructure:"cloud_database"`
Replication     ReplicationConfig `mapstructure:"replication"`
DistributedLock LockConfig        `mapstructure:"distributed_lock"`
Health          HealthConfig      `mapstructure:"health"`
Failover        FailoverConfig    `mapstructure:"failover"`
}

// DatabaseConfig holds database connection parameters.
type DatabaseConfig struct {
Host            string `mapstructure:"host"`
Port            int    `mapstructure:"port"`
Database        string `mapstructure:"database"`
User            string `mapstructure:"user"`
Password        string `mapstructure:"password"`
SSLMode         string `mapstructure:"ssl_mode"`
ReplicationSlot string `mapstructure:"replication_slot"` // Only for local database.
}

// ReplicationConfig holds replication behavior parameters.
type ReplicationConfig struct {
Tables        []string      `mapstructure:"tables"`
BatchSize     int           `mapstructure:"batch_size"`
FlushInterval time.Duration `mapstructure:"flush_interval"`
MaxLagSeconds int           `mapstructure:"max_lag_seconds"`
}

// LockConfig holds distributed lock parameters.
type LockConfig struct {
GCSBucket     string        `mapstructure:"gcs_bucket"`
LockKey       string        `mapstructure:"lock_key"`
TTL           time.Duration `mapstructure:"ttl"`
RenewInterval time.Duration `mapstructure:"renew_interval"`
}

// HealthConfig holds health check and status endpoint parameters.
type HealthConfig struct {
StatusPort    int           `mapstructure:"status_port"`
MetricsPort   int           `mapstructure:"metrics_port"`
CheckInterval time.Duration `mapstructure:"check_interval"`
}

// FailoverConfig holds failover behavior parameters.
type FailoverConfig struct {
Timeout             time.Duration `mapstructure:"timeout"`
ConsecutiveFailures int           `mapstructure:"consecutive_failures"`
}

// Load reads config from a file (YAML/JSON/etc supported by Viper), applies
// environment variable overrides, unmarshals into Config, and validates it.
func Load(configPath string) (*Config, error) {
v := viper.New()
v.SetEnvPrefix(defaultEnvPrefix)
v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
v.AutomaticEnv()

if configPath != "" {
v.SetConfigFile(configPath)
} else {
v.SetConfigName(defaultConfigName)
v.AddConfigPath(".")
}

if err := v.ReadInConfig(); err != nil {
var notFound viper.ConfigFileNotFoundError
if errors.As(err, &notFound) {
if configPath == "" {
return nil, fmt.Errorf("configuration file not found: expected %s.(yaml|yml|json) in current directory", defaultConfigName)
}
return nil, fmt.Errorf("configuration file not found at %q", configPath)
}
return nil, fmt.Errorf("failed to read configuration: %w", err)
}

cfg := &Config{}
if err := v.Unmarshal(cfg); err != nil {
return nil, fmt.Errorf("failed to unmarshal configuration: %w", err)
}

if err := cfg.Validate(); err != nil {
return nil, err
}

return cfg, nil
}

// Validate checks required startup configuration and returns descriptive errors.
func (c *Config) Validate() error {
var errs []string

validateDatabase := func(prefix string, db DatabaseConfig, requireSlot bool) {
if strings.TrimSpace(db.Host) == "" {
errs = append(errs, fmt.Sprintf("%s.host is required", prefix))
}
if db.Port <= 0 {
errs = append(errs, fmt.Sprintf("%s.port must be greater than 0", prefix))
}
if strings.TrimSpace(db.Database) == "" {
errs = append(errs, fmt.Sprintf("%s.database is required", prefix))
}
if strings.TrimSpace(db.User) == "" {
errs = append(errs, fmt.Sprintf("%s.user is required", prefix))
}
if strings.TrimSpace(db.Password) == "" {
errs = append(errs, fmt.Sprintf("%s.password is required", prefix))
}
if strings.TrimSpace(db.SSLMode) == "" {
errs = append(errs, fmt.Sprintf("%s.ssl_mode is required", prefix))
}
if requireSlot && strings.TrimSpace(db.ReplicationSlot) == "" {
errs = append(errs, fmt.Sprintf("%s.replication_slot is required", prefix))
}
}

validateDatabase("local_database", c.LocalDatabase, true)
validateDatabase("cloud_database", c.CloudDatabase, false)

if len(c.Replication.Tables) == 0 {
errs = append(errs, "replication.tables must include at least one table")
} else {
for i, table := range c.Replication.Tables {
if strings.TrimSpace(table) == "" {
errs = append(errs, fmt.Sprintf("replication.tables[%d] must not be empty", i))
}
}
}

if len(errs) > 0 {
return fmt.Errorf("configuration validation failed: %s", strings.Join(errs, "; "))
}

return nil
}
