package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/cloud-mirror/state-sync-engine/pkg/interfaces"
)

const (
	// DefaultStatePath is the default filesystem path for persisting replication state.
	DefaultStatePath = "/var/lib/state-sync/replication.state"
)

// NodeType indicates which node is currently active.
type NodeType string

const (
	NodeLocal NodeType = "local"
	NodeCloud NodeType = "cloud"
)

// ReplicationState tracks current replication progress.
// It is persisted to disk as JSON after each successful batch flush
// and loaded on startup to resume from the last known position.
type ReplicationState struct {
	SlotName         string        `json:"slot_name"`
	LastLSN          interfaces.LSN `json:"last_lsn"`
	LastFlushTime    time.Time     `json:"last_flush_time"`
	EventsProcessed  int64         `json:"events_processed"`
	EventsFailed     int64         `json:"events_failed"`
	CurrentLag       time.Duration `json:"current_lag"`
	ActiveNode       NodeType      `json:"active_node"`
	LockHolder       string        `json:"lock_holder"`
	LastFailoverTime time.Time     `json:"last_failover_time,omitempty"`
}

// Store manages persistence of ReplicationState to and from disk.
type Store struct {
	path string
}

// NewStore creates a state store that reads/writes from the given file path.
// If path is empty, DefaultStatePath is used.
func NewStore(path string) *Store {
	if path == "" {
		path = DefaultStatePath
	}
	return &Store{path: path}
}

// Save marshals the ReplicationState to JSON and writes it atomically to disk.
// It creates the parent directory if it does not exist.
// Atomic write is achieved by writing to a temporary file then renaming.
func (s *Store) Save(rs *ReplicationState) error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating state directory %q: %w", dir, err)
	}

	data, err := json.MarshalIndent(rs, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling replication state: %w", err)
	}

	// Write to a temp file in the same directory, then rename for atomicity.
	tmpFile := s.path + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return fmt.Errorf("writing temp state file %q: %w", tmpFile, err)
	}

	if err := os.Rename(tmpFile, s.path); err != nil {
		// Clean up temp file on rename failure.
		_ = os.Remove(tmpFile)
		return fmt.Errorf("renaming temp state file to %q: %w", s.path, err)
	}

	return nil
}

// Load reads the persisted ReplicationState from disk.
// If the file does not exist, it returns a zero-valued state and no error,
// indicating a fresh start where replication should begin from the beginning.
func (s *Store) Load() (*ReplicationState, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// Fresh start — no prior state.
			return &ReplicationState{
				ActiveNode: NodeLocal,
			}, nil
		}
		return nil, fmt.Errorf("reading state file %q: %w", s.path, err)
	}

	var rs ReplicationState
	if err := json.Unmarshal(data, &rs); err != nil {
		return nil, fmt.Errorf("parsing state file %q: %w", s.path, err)
	}

	return &rs, nil
}

// Path returns the filesystem path used by this store.
func (s *Store) Path() string {
	return s.path
}
