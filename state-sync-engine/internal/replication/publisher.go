package replication

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/cloud-mirror/state-sync-engine/pkg/interfaces"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Publisher implements interfaces.ReplicationPublisher for Cloud SQL.
type Publisher struct {
	pool      *pgxpool.Pool
	batchSize int
}

var _ interfaces.ReplicationPublisher = (*Publisher)(nil)

// NewPublisher creates a replication publisher backed by a pgx connection pool.
func NewPublisher(ctx context.Context, config interfaces.DBConfig, batchSize int) (*Publisher, error) {
	connString := buildConnString(config, false)
	pool, err := pgxpool.New(ctx, connString)
	if err != nil {
		return nil, fmt.Errorf("failed to create cloud database connection pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to connect to cloud database: %w", err)
	}

	if batchSize <= 0 {
		batchSize = 100
	}

	return &Publisher{
		pool:      pool,
		batchSize: batchSize,
	}, nil
}

// Publish writes change events to the cloud database in transactional batches.
func (p *Publisher) Publish(ctx context.Context, events []interfaces.ChangeEvent) error {
	if p.pool == nil {
		return errors.New("replication publisher is not initialized")
	}
	if len(events) == 0 {
		return nil
	}

	for start := 0; start < len(events); start += p.batchSize {
		end := start + p.batchSize
		if end > len(events) {
			end = len(events)
		}

		tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return fmt.Errorf("failed to begin replication batch transaction: %w", err)
		}

		committed := false
		for _, event := range events[start:end] {
			query, args, err := buildDML(event)
			if err != nil {
				_ = tx.Rollback(ctx)
				return err
			}

			if _, err := tx.Exec(ctx, query, args...); err != nil {
				_ = tx.Rollback(ctx)
				return fmt.Errorf("failed to apply %s event on table %s: %w", event.Operation, event.Table, err)
			}
		}

		if err := tx.Commit(ctx); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("failed to commit replication batch: %w", err)
		}
		committed = true

		if !committed {
			_ = tx.Rollback(ctx)
		}
	}

	return nil
}

// HealthCheck verifies cloud connectivity.
func (p *Publisher) HealthCheck(ctx context.Context) error {
	if p.pool == nil {
		return errors.New("replication publisher is not initialized")
	}

	var one int
	if err := p.pool.QueryRow(ctx, "SELECT 1").Scan(&one); err != nil {
		return fmt.Errorf("cloud database health check failed: %w", err)
	}
	if one != 1 {
		return fmt.Errorf("cloud database health check returned unexpected value: %d", one)
	}
	return nil
}

// Close releases pool resources.
func (p *Publisher) Close() {
	if p.pool != nil {
		p.pool.Close()
	}
}

func buildDML(event interfaces.ChangeEvent) (string, []any, error) {
	table, err := quoteQualifiedTable(event.Table)
	if err != nil {
		return "", nil, fmt.Errorf("invalid table for replication event: %w", err)
	}

	switch event.Operation {
	case interfaces.OperationInsert:
		return buildInsertSQL(table, event.NewRow)
	case interfaces.OperationUpdate:
		return buildUpdateSQL(table, event.NewRow, choosePredicateRow(event.OldRow, event.NewRow))
	case interfaces.OperationDelete:
		return buildDeleteSQL(table, choosePredicateRow(event.OldRow, event.NewRow))
	default:
		return "", nil, fmt.Errorf("unsupported replication operation %q", event.Operation)
	}
}

func choosePredicateRow(oldRow map[string]any, fallback map[string]any) map[string]any {
	if len(oldRow) > 0 {
		return oldRow
	}
	return fallback
}

func buildInsertSQL(table string, row map[string]any) (string, []any, error) {
	if len(row) == 0 {
		return "", nil, errors.New("insert event missing new row data")
	}

	keys := sortedKeys(row)
	columns := make([]string, 0, len(keys))
	placeholders := make([]string, 0, len(keys))
	args := make([]any, 0, len(keys))

	for idx, key := range keys {
		if !isSafeIdentifier(key) {
			return "", nil, fmt.Errorf("invalid column name %q", key)
		}
		columns = append(columns, quoteIdentifier(key))
		placeholders = append(placeholders, fmt.Sprintf("$%d", idx+1))
		args = append(args, row[key])
	}

	sql := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s)",
		table,
		strings.Join(columns, ", "),
		strings.Join(placeholders, ", "),
	)
	return sql, args, nil
}

func buildUpdateSQL(table string, newRow map[string]any, predicate map[string]any) (string, []any, error) {
	if len(newRow) == 0 {
		return "", nil, errors.New("update event missing new row data")
	}
	if len(predicate) == 0 {
		return "", nil, errors.New("update event missing predicate row data")
	}

	setKeys := sortedKeys(newRow)
	setClauses := make([]string, 0, len(setKeys))
	args := make([]any, 0, len(setKeys)+len(predicate))
	nextParam := 1

	for _, key := range setKeys {
		if !isSafeIdentifier(key) {
			return "", nil, fmt.Errorf("invalid column name %q", key)
		}
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", quoteIdentifier(key), nextParam))
		args = append(args, newRow[key])
		nextParam++
	}

	whereClause, whereArgs, err := buildWhereClause(predicate, nextParam)
	if err != nil {
		return "", nil, err
	}
	args = append(args, whereArgs...)

	sql := fmt.Sprintf("UPDATE %s SET %s WHERE %s", table, strings.Join(setClauses, ", "), whereClause)
	return sql, args, nil
}

func buildDeleteSQL(table string, predicate map[string]any) (string, []any, error) {
	if len(predicate) == 0 {
		return "", nil, errors.New("delete event missing predicate row data")
	}

	whereClause, args, err := buildWhereClause(predicate, 1)
	if err != nil {
		return "", nil, err
	}

	sql := fmt.Sprintf("DELETE FROM %s WHERE %s", table, whereClause)
	return sql, args, nil
}

func buildWhereClause(predicate map[string]any, startParam int) (string, []any, error) {
	keys := sortedKeys(predicate)
	parts := make([]string, 0, len(keys))
	args := make([]any, 0, len(keys))
	next := startParam

	for _, key := range keys {
		if !isSafeIdentifier(key) {
			return "", nil, fmt.Errorf("invalid column name %q", key)
		}
		value := predicate[key]
		if value == nil {
			parts = append(parts, fmt.Sprintf("%s IS NULL", quoteIdentifier(key)))
			continue
		}
		parts = append(parts, fmt.Sprintf("%s = $%d", quoteIdentifier(key), next))
		args = append(args, value)
		next++
	}

	return strings.Join(parts, " AND "), args, nil
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
