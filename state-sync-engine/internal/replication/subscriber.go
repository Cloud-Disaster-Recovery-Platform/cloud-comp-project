package replication

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/cloud-mirror/state-sync-engine/pkg/interfaces"
	"github.com/jackc/pglogrepl"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgproto3"
)

const (
	defaultPublicationName = "state_sync_publication"
	replicationProtocolV1  = "proto_version '1'"
	duplicateObjectSQLState = "42710"
)

type relationInfo struct {
	table   string
	columns []string
}

// Subscriber implements interfaces.ReplicationSubscriber using PostgreSQL logical replication.
type Subscriber struct {
	mu sync.RWMutex

	dbConfig        interfaces.DBConfig
	replConn        *pgconn.PgConn
	relationByID    map[uint32]relationInfo
	lastAckedLSN    interfaces.LSN
	publicationName string

	streamCancel context.CancelFunc
	streamDone   chan struct{}
}

// NewSubscriber creates a logical replication subscriber.
func NewSubscriber() *Subscriber {
	return &Subscriber{
		relationByID:    make(map[uint32]relationInfo),
		publicationName: defaultPublicationName,
	}
}

// Connect establishes replication connection and creates slot if needed.
func (s *Subscriber) Connect(ctx context.Context, config interfaces.DBConfig) error {
	if strings.TrimSpace(config.ReplicationSlot) == "" {
		return errors.New("replication slot is required")
	}

	s.mu.Lock()
	if s.replConn != nil {
		s.mu.Unlock()
		return errors.New("replication subscriber is already connected")
	}
	s.mu.Unlock()

	standardConnString := buildConnString(config, false)
	standardConn, err := pgx.Connect(ctx, standardConnString)
	if err != nil {
		return fmt.Errorf("failed to connect to local database: %w", err)
	}
	defer standardConn.Close(ctx)

	_, err = standardConn.Exec(ctx, "SELECT pg_create_logical_replication_slot($1, 'pgoutput')", config.ReplicationSlot)
	if err != nil {
		var pgErr *pgconn.PgError
		if !(errors.As(err, &pgErr) && pgErr.Code == duplicateObjectSQLState) {
			return fmt.Errorf("failed to create replication slot %q: %w", config.ReplicationSlot, err)
		}
	}

	replConn, err := pgconn.Connect(ctx, buildConnString(config, true))
	if err != nil {
		return fmt.Errorf("failed to establish replication connection: %w", err)
	}

	s.mu.Lock()
	s.dbConfig = config
	s.replConn = replConn
	s.relationByID = make(map[uint32]relationInfo)
	s.lastAckedLSN = 0
	s.mu.Unlock()

	return nil
}

// Subscribe consumes change events from the replication slot and emits ChangeEvents.
func (s *Subscriber) Subscribe(ctx context.Context, slotName string, tables []string) (<-chan interfaces.ChangeEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.replConn == nil {
		return nil, errors.New("replication subscriber is not connected")
	}
	if s.streamCancel != nil {
		return nil, errors.New("replication stream is already active")
	}

	if strings.TrimSpace(slotName) == "" {
		slotName = s.dbConfig.ReplicationSlot
	}
	if strings.TrimSpace(slotName) == "" {
		return nil, errors.New("slot name is required")
	}

	if err := s.ensurePublication(ctx, tables); err != nil {
		return nil, err
	}

	startLSN := pglogrepl.LSN(s.lastAckedLSN)
	opts := pglogrepl.StartReplicationOptions{
		PluginArgs: []string{
			replicationProtocolV1,
			fmt.Sprintf("publication_names '%s'", s.publicationName),
		},
	}
	if err := pglogrepl.StartReplication(ctx, s.replConn, slotName, startLSN, opts); err != nil {
		return nil, fmt.Errorf("failed to start logical replication: %w", err)
	}

	out := make(chan interfaces.ChangeEvent, 256)
	streamCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})

	s.streamCancel = cancel
	s.streamDone = done

	go s.consumeStream(streamCtx, out, done)

	return out, nil
}

// Acknowledge confirms processing of WAL up to the provided LSN.
func (s *Subscriber) Acknowledge(ctx context.Context, lsn interfaces.LSN) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.replConn == nil {
		return errors.New("replication subscriber is not connected")
	}

	status := pglogrepl.StandbyStatusUpdate{
		WALWritePosition: pglogrepl.LSN(lsn),
		WALFlushPosition: pglogrepl.LSN(lsn),
		WALApplyPosition: pglogrepl.LSN(lsn),
		ClientTime:       time.Now(),
	}
	if err := pglogrepl.SendStandbyStatusUpdate(ctx, s.replConn, status); err != nil {
		return fmt.Errorf("failed to acknowledge LSN %d: %w", lsn, err)
	}

	if lsn > s.lastAckedLSN {
		s.lastAckedLSN = lsn
	}

	return nil
}

// Close gracefully shuts down replication stream and database connection.
func (s *Subscriber) Close() error {
	s.mu.Lock()
	cancel := s.streamCancel
	done := s.streamDone
	conn := s.replConn

	s.streamCancel = nil
	s.streamDone = nil
	s.replConn = nil
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	if done != nil {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
		}
	}

	if conn != nil {
		ctx, cancelClose := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelClose()
		if err := conn.Close(ctx); err != nil {
			return fmt.Errorf("failed to close replication connection: %w", err)
		}
	}

	return nil
}

func (s *Subscriber) ensurePublication(ctx context.Context, tables []string) error {
	standardConn, err := pgx.Connect(ctx, buildConnString(s.dbConfig, false))
	if err != nil {
		return fmt.Errorf("failed to connect for publication setup: %w", err)
	}
	defer standardConn.Close(ctx)

	var exists bool
	err = standardConn.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_publication WHERE pubname = $1)", s.publicationName).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check publication existence: %w", err)
	}
	if exists {
		return nil
	}

	pubIdentifier := quoteIdentifier(s.publicationName)
	statement := fmt.Sprintf("CREATE PUBLICATION %s FOR ALL TABLES", pubIdentifier)
	if len(tables) > 0 {
		quoted := make([]string, 0, len(tables))
		for _, table := range tables {
			qt, err := quoteQualifiedTable(table)
			if err != nil {
				return err
			}
			quoted = append(quoted, qt)
		}
		statement = fmt.Sprintf("CREATE PUBLICATION %s FOR TABLE %s", pubIdentifier, strings.Join(quoted, ", "))
	}

	if _, err := standardConn.Exec(ctx, statement); err != nil {
		var pgErr *pgconn.PgError
		if !(errors.As(err, &pgErr) && pgErr.Code == duplicateObjectSQLState) {
			return fmt.Errorf("failed to create publication %q: %w", s.publicationName, err)
		}
	}

	return nil
}

func (s *Subscriber) consumeStream(ctx context.Context, out chan<- interfaces.ChangeEvent, done chan<- struct{}) {
	defer close(done)
	defer close(out)

	var txLSN interfaces.LSN
	var txTime time.Time

	for {
		msg, err := s.replConn.ReceiveMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			return
		}

		copyData, ok := msg.(*pgproto3.CopyData)
		if !ok || len(copyData.Data) == 0 {
			continue
		}

		switch copyData.Data[0] {
		case pglogrepl.PrimaryKeepaliveMessageByteID:
			pkm, err := pglogrepl.ParsePrimaryKeepaliveMessage(copyData.Data[1:])
			if err != nil {
				continue
			}
			if pkm.ReplyRequested {
				_ = s.Acknowledge(context.Background(), s.currentAckedLSN())
			}
		case pglogrepl.XLogDataByteID:
			xld, err := pglogrepl.ParseXLogData(copyData.Data[1:])
			if err != nil {
				continue
			}

			logicalMessage, err := pglogrepl.Parse(xld.WALData)
			if err != nil {
				continue
			}

			switch m := logicalMessage.(type) {
			case *pglogrepl.BeginMessage:
				txLSN = interfaces.LSN(m.FinalLSN)
				txTime = m.CommitTime
			case *pglogrepl.RelationMessage:
				s.storeRelation(m)
			case *pglogrepl.InsertMessage:
				s.emitEvent(ctx, out, interfaces.ChangeEvent{
					LSN:       txLSN,
					Timestamp: txTime,
					Table:     s.relationName(m.RelationID),
					Operation: interfaces.OperationInsert,
					NewRow:    s.extractTuple(m, "Tuple", s.relationColumns(m.RelationID)),
				})
			case *pglogrepl.UpdateMessage:
				s.emitEvent(ctx, out, interfaces.ChangeEvent{
					LSN:       txLSN,
					Timestamp: txTime,
					Table:     s.relationName(m.RelationID),
					Operation: interfaces.OperationUpdate,
					OldRow:    s.extractTuple(m, "OldTuple", s.relationColumns(m.RelationID)),
					NewRow:    s.extractTuple(m, "NewTuple", s.relationColumns(m.RelationID)),
				})
			case *pglogrepl.DeleteMessage:
				s.emitEvent(ctx, out, interfaces.ChangeEvent{
					LSN:       txLSN,
					Timestamp: txTime,
					Table:     s.relationName(m.RelationID),
					Operation: interfaces.OperationDelete,
					OldRow:    s.extractTuple(m, "OldTuple", s.relationColumns(m.RelationID)),
				})
			case *pglogrepl.CommitMessage:
				txLSN = interfaces.LSN(m.CommitLSN)
				_ = s.Acknowledge(context.Background(), txLSN)
			}
		}
	}
}

func (s *Subscriber) emitEvent(ctx context.Context, out chan<- interfaces.ChangeEvent, event interfaces.ChangeEvent) {
	select {
	case out <- event:
	case <-ctx.Done():
	}
}

func (s *Subscriber) storeRelation(msg *pglogrepl.RelationMessage) {
	value := reflect.ValueOf(msg)
	if value.Kind() == reflect.Pointer {
		value = value.Elem()
	}

	relationIDField := value.FieldByName("RelationID")
	namespaceField := value.FieldByName("Namespace")
	relationNameField := value.FieldByName("RelationName")
	columnsField := value.FieldByName("Columns")

	if !relationIDField.IsValid() || !namespaceField.IsValid() || !relationNameField.IsValid() || !columnsField.IsValid() {
		return
	}

	relationID := uint32(relationIDField.Uint())
	namespace := namespaceField.String()
	relationName := relationNameField.String()

	columns := make([]string, 0, columnsField.Len())
	for i := 0; i < columnsField.Len(); i++ {
		columnValue := columnsField.Index(i)
		if columnValue.Kind() == reflect.Pointer {
			if columnValue.IsNil() {
				columns = append(columns, "")
				continue
			}
			columnValue = columnValue.Elem()
		}
		nameField := columnValue.FieldByName("Name")
		if nameField.IsValid() && nameField.Kind() == reflect.String {
			columns = append(columns, nameField.String())
			continue
		}
		columns = append(columns, "")
	}

	s.mu.Lock()
	s.relationByID[relationID] = relationInfo{
		table:   namespace + "." + relationName,
		columns: columns,
	}
	s.mu.Unlock()
}

func (s *Subscriber) relationName(relationID uint32) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	relation, ok := s.relationByID[relationID]
	if !ok {
		return ""
	}
	return relation.table
}

func (s *Subscriber) relationColumns(relationID uint32) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	relation, ok := s.relationByID[relationID]
	if !ok {
		return nil
	}
	out := make([]string, len(relation.columns))
	copy(out, relation.columns)
	return out
}

func (s *Subscriber) extractTuple(message any, fieldName string, columns []string) map[string]any {
	value := reflect.ValueOf(message)
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil
		}
		value = value.Elem()
	}

	tupleField := value.FieldByName(fieldName)
	if !tupleField.IsValid() {
		return nil
	}
	if tupleField.Kind() == reflect.Pointer {
		if tupleField.IsNil() {
			return nil
		}
		tupleField = tupleField.Elem()
	}

	columnsField := tupleField.FieldByName("Columns")
	if !columnsField.IsValid() || columnsField.Kind() != reflect.Slice {
		return nil
	}

	row := make(map[string]any, columnsField.Len())
	for i := 0; i < columnsField.Len(); i++ {
		columnValue := columnsField.Index(i)
		if columnValue.Kind() == reflect.Pointer {
			if columnValue.IsNil() {
				continue
			}
			columnValue = columnValue.Elem()
		}

		columnName := fmt.Sprintf("col_%d", i+1)
		if i < len(columns) && strings.TrimSpace(columns[i]) != "" {
			columnName = columns[i]
		}

		dataType := byte(0)
		dataTypeField := columnValue.FieldByName("DataType")
		if dataTypeField.IsValid() && dataTypeField.CanUint() {
			dataType = byte(dataTypeField.Uint())
		}

		switch dataType {
		case 'n':
			row[columnName] = nil
			continue
		case 'u':
			row[columnName] = "unchanged_toast"
			continue
		}

		dataField := columnValue.FieldByName("Data")
		if !dataField.IsValid() || dataField.Kind() != reflect.Slice || dataField.Type().Elem().Kind() != reflect.Uint8 {
			row[columnName] = nil
			continue
		}
		row[columnName] = string(dataField.Bytes())
	}

	return row
}

func (s *Subscriber) currentAckedLSN() interfaces.LSN {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastAckedLSN
}

func buildConnString(config interfaces.DBConfig, replicationMode bool) string {
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
	if replicationMode {
		query.Set("replication", "database")
	}
	connURL.RawQuery = query.Encode()

	return connURL.String()
}

func quoteQualifiedTable(table string) (string, error) {
	parts := strings.Split(table, ".")
	if len(parts) == 1 {
		if !isSafeIdentifier(parts[0]) {
			return "", fmt.Errorf("invalid table name %q", table)
		}
		return quoteIdentifier(parts[0]), nil
	}
	if len(parts) == 2 {
		if !isSafeIdentifier(parts[0]) || !isSafeIdentifier(parts[1]) {
			return "", fmt.Errorf("invalid table name %q", table)
		}
		return quoteIdentifier(parts[0]) + "." + quoteIdentifier(parts[1]), nil
	}
	return "", fmt.Errorf("invalid table name %q", table)
}

func quoteIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

func isSafeIdentifier(identifier string) bool {
	if identifier == "" {
		return false
	}
	for _, r := range identifier {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		}
		return false
	}
	return true
}
