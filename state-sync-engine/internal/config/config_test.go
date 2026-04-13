package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// validConfig returns a minimal valid Config for testing.
func validConfig() Config {
	return Config{
		LocalDatabase: DatabaseConfig{
			Host:            "localhost",
			Port:            5432,
			Database:        "myapp",
			User:            "replicator",
			Password:        "localpass",
			SSLMode:         "require",
			ReplicationSlot: "failsafe_slot",
		},
		CloudDatabase: DatabaseConfig{
			Host:     "10.0.0.3",
			Port:     5432,
			Database: "myapp",
			User:     "replicator",
			Password: "cloudpass",
			SSLMode:  "require",
		},
		Replication: ReplicationConfig{
			Tables:        []string{"public.users"},
			BatchSize:     100,
			FlushInterval: 1 * time.Second,
			MaxLagSeconds: 30,
		},
		DistributedLock: LockConfig{
			GCSBucket:     "failsafe-locks",
			LockKey:       "active-writer",
			TTL:           30 * time.Second,
			RenewInterval: 10 * time.Second,
		},
		Health: HealthConfig{
			StatusPort:    8080,
			MetricsPort:   9090,
			CheckInterval: 10 * time.Second,
		},
		Failover: FailoverConfig{
			Timeout:             60 * time.Second,
			ConsecutiveFailures: 3,
		},
	}
}

func TestValidate_ValidConfig(t *testing.T) {
	cfg := validConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid config, got error: %v", err)
	}
}

func TestValidate_MissingLocalDatabaseHost(t *testing.T) {
	cfg := validConfig()
	cfg.LocalDatabase.Host = ""
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for missing local_database.host")
	}
	assertContains(t, err.Error(), "local_database.host is required")
}

func TestValidate_MissingLocalDatabaseName(t *testing.T) {
	cfg := validConfig()
	cfg.LocalDatabase.Database = ""
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for missing local_database.database")
	}
	assertContains(t, err.Error(), "local_database.database is required")
}

func TestValidate_MissingLocalDatabaseUser(t *testing.T) {
	cfg := validConfig()
	cfg.LocalDatabase.User = ""
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for missing local_database.user")
	}
	assertContains(t, err.Error(), "local_database.user is required")
}

func TestValidate_MissingLocalDatabasePassword(t *testing.T) {
	cfg := validConfig()
	cfg.LocalDatabase.Password = ""
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for missing local_database.password")
	}
	assertContains(t, err.Error(), "local_database.password is required")
}

func TestValidate_InvalidLocalDatabasePort(t *testing.T) {
	cfg := validConfig()
	cfg.LocalDatabase.Port = 0
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for invalid local_database.port")
	}
	assertContains(t, err.Error(), "local_database.port must be greater than 0")
}

func TestValidate_MissingLocalDatabaseSSLMode(t *testing.T) {
	cfg := validConfig()
	cfg.LocalDatabase.SSLMode = ""
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for missing local_database.ssl_mode")
	}
	assertContains(t, err.Error(), "local_database.ssl_mode is required")
}

func TestValidate_MissingLocalDatabaseReplicationSlot(t *testing.T) {
	cfg := validConfig()
	cfg.LocalDatabase.ReplicationSlot = ""
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for missing local_database.replication_slot")
	}
	assertContains(t, err.Error(), "local_database.replication_slot is required")
}

func TestValidate_MissingCloudDatabaseHost(t *testing.T) {
	cfg := validConfig()
	cfg.CloudDatabase.Host = ""
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for missing cloud_database.host")
	}
	assertContains(t, err.Error(), "cloud_database.host is required")
}

func TestValidate_MissingCloudDatabasePassword(t *testing.T) {
	cfg := validConfig()
	cfg.CloudDatabase.Password = ""
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for missing cloud_database.password")
	}
	assertContains(t, err.Error(), "cloud_database.password is required")
}

func TestValidate_CloudDatabaseDoesNotRequireReplicationSlot(t *testing.T) {
	cfg := validConfig()
	cfg.CloudDatabase.ReplicationSlot = "" // should be fine
	if err := cfg.Validate(); err != nil {
		t.Fatalf("cloud_database should not require replication_slot, got: %v", err)
	}
}

func TestValidate_EmptyReplicationTables(t *testing.T) {
	cfg := validConfig()
	cfg.Replication.Tables = nil
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for empty replication.tables")
	}
	assertContains(t, err.Error(), "replication.tables must include at least one table")
}

func TestValidate_EmptyTableName(t *testing.T) {
	cfg := validConfig()
	cfg.Replication.Tables = []string{"public.users", ""}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for empty table name")
	}
	assertContains(t, err.Error(), "replication.tables[1] must not be empty")
}

func TestValidate_MultipleErrors(t *testing.T) {
	cfg := Config{} // everything missing/zero-valued
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation errors for empty config")
	}
	assertContains(t, err.Error(), "local_database.host is required")
	assertContains(t, err.Error(), "cloud_database.host is required")
	assertContains(t, err.Error(), "replication.tables must include at least one table")
	assertContains(t, err.Error(), "local_database.replication_slot is required")
}

func TestLoad_FromYAMLFile(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.yaml")

	yaml := `
local_database:
  host: localhost
  port: 5432
  database: testdb
  user: testuser
  password: testpass
  ssl_mode: require
  replication_slot: test_slot

cloud_database:
  host: 10.0.0.3
  port: 5432
  database: testdb
  user: clouduser
  password: cloudpass
  ssl_mode: require

replication:
  tables:
    - public.users
    - public.orders
  batch_size: 50
  flush_interval: 2s
  max_lag_seconds: 60

distributed_lock:
  gcs_bucket: test-locks
  lock_key: active-writer
  ttl: 30s
  renew_interval: 10s

health:
  status_port: 8080
  metrics_port: 9090
  check_interval: 10s

failover:
  timeout: 60s
  consecutive_failures: 5
`
	if err := os.WriteFile(configFile, []byte(yaml), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := Load(configFile)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.LocalDatabase.Host != "localhost" {
		t.Errorf("expected host localhost, got %s", cfg.LocalDatabase.Host)
	}
	if cfg.LocalDatabase.Database != "testdb" {
		t.Errorf("expected database testdb, got %s", cfg.LocalDatabase.Database)
	}
	if cfg.LocalDatabase.ReplicationSlot != "test_slot" {
		t.Errorf("expected replication_slot test_slot, got %s", cfg.LocalDatabase.ReplicationSlot)
	}
	if cfg.CloudDatabase.Host != "10.0.0.3" {
		t.Errorf("expected cloud host 10.0.0.3, got %s", cfg.CloudDatabase.Host)
	}
	if len(cfg.Replication.Tables) != 2 {
		t.Errorf("expected 2 tables, got %d", len(cfg.Replication.Tables))
	}
	if cfg.Replication.BatchSize != 50 {
		t.Errorf("expected batch_size 50, got %d", cfg.Replication.BatchSize)
	}
	if cfg.Replication.FlushInterval != 2*time.Second {
		t.Errorf("expected flush_interval 2s, got %v", cfg.Replication.FlushInterval)
	}
	if cfg.DistributedLock.GCSBucket != "test-locks" {
		t.Errorf("expected gcs_bucket test-locks, got %s", cfg.DistributedLock.GCSBucket)
	}
	if cfg.Failover.ConsecutiveFailures != 5 {
		t.Errorf("expected consecutive_failures 5, got %d", cfg.Failover.ConsecutiveFailures)
	}
}

func TestLoad_EnvVarOverrides(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.yaml")

	yaml := `
local_database:
  host: localhost
  port: 5432
  database: myapp
  user: replicator
  password: filepass
  ssl_mode: require
  replication_slot: failsafe_slot

cloud_database:
  host: 10.0.0.3
  port: 5432
  database: myapp
  user: replicator
  password: cloudfilepass
  ssl_mode: require

replication:
  tables:
    - public.users
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
`
	if err := os.WriteFile(configFile, []byte(yaml), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	// Env prefix is STATE_SYNC per the current implementation.
	t.Setenv("STATE_SYNC_LOCAL_DATABASE_PASSWORD", "envpass")

	cfg, err := Load(configFile)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.LocalDatabase.Password != "envpass" {
		t.Errorf("expected password envpass from env var, got %s", cfg.LocalDatabase.Password)
	}
	if cfg.LocalDatabase.Host != "localhost" {
		t.Errorf("expected host localhost from file, got %s", cfg.LocalDatabase.Host)
	}
}

func TestLoad_InvalidYAMLFile(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.yaml")

	if err := os.WriteFile(configFile, []byte("invalid: [yaml: broken"), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_, err := Load(configFile)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestLoad_ValidationFailsOnMissingRequiredFields(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.yaml")

	yaml := `
local_database:
  host: localhost
`
	if err := os.WriteFile(configFile, []byte(yaml), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_, err := Load(configFile)
	if err == nil {
		t.Fatal("expected validation error for incomplete config")
	}
	assertContains(t, err.Error(), "configuration validation failed")
}

func TestLoad_FromJSONFile(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.json")

	jsonData := `{
  "local_database": {
    "host": "localhost",
    "port": 5432,
    "database": "jsondb",
    "user": "jsonuser",
    "password": "jsonpass",
    "ssl_mode": "require",
    "replication_slot": "json_slot"
  },
  "cloud_database": {
    "host": "10.0.0.5",
    "port": 5432,
    "database": "jsondb",
    "user": "clouduser",
    "password": "cloudpass",
    "ssl_mode": "require"
  },
  "replication": {
    "tables": ["public.items"],
    "batch_size": 200,
    "flush_interval": "3s",
    "max_lag_seconds": 45
  },
  "distributed_lock": {
    "gcs_bucket": "json-locks",
    "lock_key": "active-writer",
    "ttl": "30s",
    "renew_interval": "10s"
  },
  "health": {
    "status_port": 8080,
    "metrics_port": 9090,
    "check_interval": "10s"
  },
  "failover": {
    "timeout": "60s",
    "consecutive_failures": 3
  }
}`
	if err := os.WriteFile(configFile, []byte(jsonData), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := Load(configFile)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.LocalDatabase.Database != "jsondb" {
		t.Errorf("expected database jsondb, got %s", cfg.LocalDatabase.Database)
	}
	if cfg.Replication.BatchSize != 200 {
		t.Errorf("expected batch_size 200, got %d", cfg.Replication.BatchSize)
	}
}

func TestLoad_ConfigFileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/path/config.yaml")
	if err == nil {
		t.Fatal("expected error for missing config file")
	}
}

func assertContains(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Errorf("expected error to contain %q, got %q", want, got)
	}
}
