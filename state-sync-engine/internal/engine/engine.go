package engine

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/cloud-mirror/state-sync-engine/internal/config"
	"github.com/cloud-mirror/state-sync-engine/internal/state"
	"github.com/cloud-mirror/state-sync-engine/pkg/interfaces"
)

const (
	// shutdownTimeout is the maximum time allowed for graceful shutdown.
	shutdownTimeout = 30 * time.Second
)

// Engine is the main replication orchestrator. It subscribes to a local
// PostgreSQL database's logical replication stream, buffers change events
// in memory, and flushes batches to a cloud database via a publisher.
// After each successful flush it acknowledges the LSN and persists
// replication state to disk.
type Engine struct {
	cfg        *config.Config
	subscriber interfaces.ReplicationSubscriber
	publisher  interfaces.ReplicationPublisher
	stateStore *state.Store

	mu    sync.Mutex
	state *state.ReplicationState

	// buffer holds queued events waiting to be flushed.
	buffer []interfaces.ChangeEvent
}

// New creates an Engine with the provided dependencies.
func New(
	cfg *config.Config,
	subscriber interfaces.ReplicationSubscriber,
	publisher interfaces.ReplicationPublisher,
	stateStore *state.Store,
) *Engine {
	return &Engine{
		cfg:        cfg,
		subscriber: subscriber,
		publisher:  publisher,
		stateStore: stateStore,
	}
}

// Run starts the replication engine. It blocks until a termination signal
// is received or the context is cancelled. It performs:
//  1. Load persisted state (resume from last LSN).
//  2. Connect subscriber to local database.
//  3. Start consuming change events into an in-memory buffer.
//  4. Flush batches to the cloud on batch_size or flush_interval.
//  5. On shutdown: stop consuming, flush remaining events, persist state, close connections.
func (e *Engine) Run(ctx context.Context) error {
	// Step 1: Load persisted state.
	rs, err := e.stateStore.Load()
	if err != nil {
		return fmt.Errorf("loading replication state: %w", err)
	}
	e.mu.Lock()
	e.state = rs
	e.buffer = make([]interfaces.ChangeEvent, 0, e.cfg.Replication.BatchSize)
	e.mu.Unlock()

	log.Printf("replication state loaded: last_lsn=%d events_processed=%d",
		rs.LastLSN, rs.EventsProcessed)

	// Step 2: Connect subscriber.
	localDB := toDBConfig(e.cfg.LocalDatabase)
	if err := e.subscriber.Connect(ctx, localDB); err != nil {
		return fmt.Errorf("connecting subscriber: %w", err)
	}

	// Step 3: Subscribe to change events.
	eventCh, err := e.subscriber.Subscribe(
		ctx,
		e.cfg.LocalDatabase.ReplicationSlot,
		e.cfg.Replication.Tables,
	)
	if err != nil {
		return fmt.Errorf("starting subscription: %w", err)
	}

	// Set up signal-aware context for graceful shutdown.
	runCtx, stop := signal.NotifyContext(ctx, syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	// Step 4: Main replication loop.
	return e.loop(runCtx, eventCh)
}

// loop is the core event loop that consumes change events and flushes batches.
func (e *Engine) loop(ctx context.Context, eventCh <-chan interfaces.ChangeEvent) error {
	flushInterval := e.cfg.Replication.FlushInterval
	if flushInterval <= 0 {
		flushInterval = time.Second
	}
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Shutdown requested.
			return e.shutdown()

		case event, ok := <-eventCh:
			if !ok {
				// Channel closed (subscriber stopped). Flush and exit.
				log.Println("event channel closed, flushing remaining events")
				return e.shutdown()
			}

			e.mu.Lock()
			e.buffer = append(e.buffer, event)
			bufLen := len(e.buffer)
			e.mu.Unlock()

			// Flush if buffer reaches batch_size.
			if bufLen >= e.cfg.Replication.BatchSize {
				if err := e.flush(context.Background()); err != nil {
					log.Printf("error flushing batch: %v", err)
				}
			}

		case <-ticker.C:
			// Periodic flush based on flush_interval.
			e.mu.Lock()
			bufLen := len(e.buffer)
			e.mu.Unlock()

			if bufLen > 0 {
				if err := e.flush(context.Background()); err != nil {
					log.Printf("error flushing batch on interval: %v", err)
				}
			}
		}
	}
}

// flush publishes all buffered events to the cloud database, acknowledges
// the last LSN, updates replication state metrics, and persists state to disk.
func (e *Engine) flush(ctx context.Context) error {
	e.mu.Lock()
	if len(e.buffer) == 0 {
		e.mu.Unlock()
		return nil
	}
	batch := make([]interfaces.ChangeEvent, len(e.buffer))
	copy(batch, e.buffer)
	e.buffer = e.buffer[:0]
	e.mu.Unlock()

	// Publish batch to cloud.
	if err := e.publisher.Publish(ctx, batch); err != nil {
		// On failure, put events back into the buffer for retry.
		e.mu.Lock()
		e.buffer = append(batch, e.buffer...)
		e.state.EventsFailed += int64(len(batch))
		e.mu.Unlock()
		return fmt.Errorf("publishing batch of %d events: %w", len(batch), err)
	}

	// Determine the highest LSN in the batch.
	var maxLSN interfaces.LSN
	var latestTimestamp time.Time
	for _, ev := range batch {
		if ev.LSN > maxLSN {
			maxLSN = ev.LSN
		}
		if ev.Timestamp.After(latestTimestamp) {
			latestTimestamp = ev.Timestamp
		}
	}

	// Acknowledge the LSN so PostgreSQL can advance the replication slot.
	if maxLSN > 0 {
		if err := e.subscriber.Acknowledge(ctx, maxLSN); err != nil {
			log.Printf("warning: failed to acknowledge LSN %d: %v", maxLSN, err)
			// Non-fatal: events were published; LSN will be re-acked next time.
		}
	}

	// Update in-memory state.
	now := time.Now()
	e.mu.Lock()
	e.state.LastLSN = maxLSN
	e.state.LastFlushTime = now
	e.state.EventsProcessed += int64(len(batch))
	e.state.SlotName = e.cfg.LocalDatabase.ReplicationSlot
	if !latestTimestamp.IsZero() {
		e.state.CurrentLag = now.Sub(latestTimestamp)
	}
	stateSnapshot := *e.state
	e.mu.Unlock()

	log.Printf("flushed %d events: lsn=%d lag=%s total_processed=%d",
		len(batch), maxLSN, stateSnapshot.CurrentLag.Round(time.Millisecond), stateSnapshot.EventsProcessed)

	// Persist state to disk.
	if err := e.stateStore.Save(&stateSnapshot); err != nil {
		log.Printf("warning: failed to persist replication state: %v", err)
		// Non-fatal: state will be retried on next flush.
	}

	return nil
}

// shutdown performs graceful shutdown:
//  1. Flush all queued events.
//  2. Persist final replication state.
//  3. Close subscriber connection.
//
// It enforces a 30-second timeout and logs a warning if exceeded.
func (e *Engine) shutdown() error {
	log.Println("shutting down replication engine...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- e.shutdownWork(shutdownCtx)
	}()

	select {
	case err := <-done:
		if err != nil {
			log.Printf("shutdown completed with errors: %v", err)
		} else {
			log.Println("shutdown complete")
		}
		return err
	case <-shutdownCtx.Done():
		log.Println("WARNING: graceful shutdown exceeded 30 second timeout")
		return fmt.Errorf("shutdown timed out after %s", shutdownTimeout)
	}
}

func (e *Engine) shutdownWork(ctx context.Context) error {
	var shutdownErr error

	// Step 1: Flush remaining buffered events.
	if err := e.flush(ctx); err != nil {
		log.Printf("error flushing during shutdown: %v", err)
		shutdownErr = err
	}

	// Step 2: Persist final state.
	e.mu.Lock()
	stateSnapshot := *e.state
	e.mu.Unlock()

	if err := e.stateStore.Save(&stateSnapshot); err != nil {
		log.Printf("error persisting state during shutdown: %v", err)
		if shutdownErr == nil {
			shutdownErr = err
		}
	}

	// Step 3: Close subscriber.
	if err := e.subscriber.Close(); err != nil {
		log.Printf("error closing subscriber during shutdown: %v", err)
		if shutdownErr == nil {
			shutdownErr = err
		}
	}

	return shutdownErr
}

// State returns a snapshot of the current replication state.
// Safe for concurrent use.
func (e *Engine) State() state.ReplicationState {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.state == nil {
		return state.ReplicationState{}
	}
	return *e.state
}

// toDBConfig converts a config.DatabaseConfig to an interfaces.DBConfig.
func toDBConfig(db config.DatabaseConfig) interfaces.DBConfig {
	return interfaces.DBConfig{
		Host:            db.Host,
		Port:            db.Port,
		Database:        db.Database,
		User:            db.User,
		Password:        db.Password,
		SSLMode:         db.SSLMode,
		ReplicationSlot: db.ReplicationSlot,
	}
}

// SignalAwareRun is a convenience function that creates the signal context
// at the top level. Useful for main.go.
func (e *Engine) SignalAwareRun() error {
	ctx := context.Background()
	sigCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	return e.Run(sigCtx)
}
