package coordinator

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/cloud-mirror/state-sync-engine/pkg/interfaces"
	"github.com/jackc/pgx/v5"
)

// PGStateCoordinator implements interfaces.StateCoordinator using PostgreSQL
// administrative SQL commands to transition databases between read-only and
// read-write modes during failover and recovery.
type PGStateCoordinator struct{}

var _ interfaces.StateCoordinator = (*PGStateCoordinator)(nil)

// NewPGStateCoordinator creates a new state coordinator.
func NewPGStateCoordinator() *PGStateCoordinator {
	return &PGStateCoordinator{}
}

// SetReadOnly marks the target database as read-only by executing:
//  1. ALTER DATABASE <db> SET default_transaction_read_only = on
//  2. Terminating active write transactions via pg_terminate_backend
//
// This corresponds to the read-only coordination described in the design doc.
// Requirements: 6.2
func (c *PGStateCoordinator) SetReadOnly(ctx context.Context, db interfaces.DBConfig) error {
	conn, err := connect(ctx, db)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)

	dbIdent := quoteIdentifier(db.Database)

	// Step 1: Set the database default to read-only for new transactions.
	alterSQL := fmt.Sprintf("ALTER DATABASE %s SET default_transaction_read_only = on", dbIdent)
	if _, err := conn.Exec(ctx, alterSQL); err != nil {
		return fmt.Errorf("failed to set database %q to read-only: %w", db.Database, err)
	}

	// Step 2: Terminate active sessions that may still be writing.
	// Exclude our own session and monitoring queries.
	terminateSQL := `
		SELECT pg_terminate_backend(pid)
		FROM pg_stat_activity
		WHERE datname = $1
		  AND pid != pg_backend_pid()
		  AND state = 'active'
		  AND query NOT ILIKE '%pg_stat_activity%'`
	if _, err := conn.Exec(ctx, terminateSQL, db.Database); err != nil {
		return fmt.Errorf("failed to terminate active transactions on %q: %w", db.Database, err)
	}

	// Step 3: Verify the transition completed.
	state, err := c.getStateWithConn(ctx, conn)
	if err != nil {
		return fmt.Errorf("failed to verify read-only state on %q: %w", db.Database, err)
	}
	if state != interfaces.DatabaseStateReadOnly {
		return fmt.Errorf("database %q is not in read-only mode after SET (got %s)", db.Database, state)
	}

	return nil
}

// SetReadWrite enables writes on the target database by executing:
//
//	ALTER DATABASE <db> SET default_transaction_read_only = off
//
// After executing, it verifies the transition completed successfully.
// Requirements: 6.3
func (c *PGStateCoordinator) SetReadWrite(ctx context.Context, db interfaces.DBConfig) error {
	conn, err := connect(ctx, db)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)

	dbIdent := quoteIdentifier(db.Database)

	alterSQL := fmt.Sprintf("ALTER DATABASE %s SET default_transaction_read_only = off", dbIdent)
	if _, err := conn.Exec(ctx, alterSQL); err != nil {
		return fmt.Errorf("failed to set database %q to read-write: %w", db.Database, err)
	}

	// Verify the transition completed.
	state, err := c.getStateWithConn(ctx, conn)
	if err != nil {
		return fmt.Errorf("failed to verify read-write state on %q: %w", db.Database, err)
	}
	if state != interfaces.DatabaseStateReadWrite {
		return fmt.Errorf("database %q is not in read-write mode after SET (got %s)", db.Database, state)
	}

	return nil
}

// GetState queries the current default_transaction_read_only setting and
// returns the corresponding DatabaseState enum.
// Requirements: 6.2, 6.3
func (c *PGStateCoordinator) GetState(ctx context.Context, db interfaces.DBConfig) (interfaces.DatabaseState, error) {
	conn, err := connect(ctx, db)
	if err != nil {
		return "", err
	}
	defer conn.Close(ctx)

	return c.getStateWithConn(ctx, conn)
}

// getStateWithConn queries the read-only state using an existing connection.
// It uses SHOW to query the session-level default inherited from the database
// setting, then resets the session to pick up database-level changes.
func (c *PGStateCoordinator) getStateWithConn(ctx context.Context, conn *pgx.Conn) (interfaces.DatabaseState, error) {
	// RESET ensures we see the database-level default, not any session override.
	if _, err := conn.Exec(ctx, "RESET default_transaction_read_only"); err != nil {
		return "", fmt.Errorf("failed to reset transaction read-only setting: %w", err)
	}

	var value string
	if err := conn.QueryRow(ctx, "SHOW default_transaction_read_only").Scan(&value); err != nil {
		return "", fmt.Errorf("failed to query default_transaction_read_only: %w", err)
	}

	switch strings.TrimSpace(strings.ToLower(value)) {
	case "on":
		return interfaces.DatabaseStateReadOnly, nil
	case "off":
		return interfaces.DatabaseStateReadWrite, nil
	default:
		return "", fmt.Errorf("unexpected default_transaction_read_only value: %q", value)
	}
}

// connect establishes a standard (non-replication) pgx connection.
func connect(ctx context.Context, db interfaces.DBConfig) (*pgx.Conn, error) {
	connStr := buildConnString(db)
	conn, err := pgx.Connect(ctx, connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database %q at %s:%d: %w",
			db.Database, db.Host, db.Port, err)
	}
	return conn, nil
}

// buildConnString constructs a PostgreSQL connection string from DBConfig.
func buildConnString(config interfaces.DBConfig) string {
	connURL := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(config.User, config.Password),
		Host:   fmt.Sprintf("%s:%d", config.Host, config.Port),
		Path:   config.Database,
	}

	query := connURL.Query()
	if strings.TrimSpace(config.SSLMode) != "" {
		query.Set("sslmode", config.SSLMode)
	}
	connURL.RawQuery = query.Encode()

	return connURL.String()
}

// quoteIdentifier safely quotes a PostgreSQL identifier to prevent SQL injection.
func quoteIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}
