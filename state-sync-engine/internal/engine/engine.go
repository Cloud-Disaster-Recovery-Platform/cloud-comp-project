package engine

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/cloud-mirror/state-sync-engine/internal/config"
	"github.com/cloud-mirror/state-sync-engine/internal/metrics"
	"github.com/cloud-mirror/state-sync-engine/internal/retry"
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
	logger     *zap.Logger
	metrics    *metrics.Metrics

	mu    sync.Mutex
	state *state.ReplicationState

	// buffer holds queued events waiting to be flushed.
	buffer []interfaces.ChangeEvent

	// cloudBreaker tracks consecutive publish failures to the cloud database.
	cloudBreaker *retry.CircuitBreaker
}

// New creates an Engine with the provided dependencies.
func New(
	cfg *config.Config,
	subscriber interfaces.ReplicationSubscriber,
	publisher interfaces.ReplicationPublisher,
	stateStore *state.Store,
	logger *zap.Logger,
	m *metrics.Metrics,
) *Engine {
	return &Engine{
		cfg:        cfg,
		subscriber: subscriber,
		publisher:  publisher,
		stateStore: stateStore,
		logger:     logger,
		metrics:    m,
		cloudBreaker: retry.NewCircuitBreaker(retry.CircuitBreakerConfig{
			FailureThreshold: 5,
			RetryInterval:    5 * time.Minute,
		}),
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

	e.logger.Info("replication state loaded",
		zap.Uint64("last_lsn", uint64(rs.LastLSN)),
		zap.Int64("events_processed", rs.EventsProcessed),
	)

	// Step 2: Connect subscriber with exponential backoff.
	localDB := toDBConfig(e.cfg.LocalDatabase)
	backoff := retry.DefaultBackoff()
	err = retry.Do(ctx, backoff, 0, func(attempt int, retryErr error, nextDelay time.Duration) {
		e.logger.Warn("subscriber connection failed, retrying",
			zap.Int("attempt", attempt+1),
			zap.Error(retryErr),
			zap.Duration("next_delay", nextDelay),
		)
	}, func(retryCtx context.Context) error {
		return e.subscriber.Connect(retryCtx, localDB)
	})
	if err != nil {
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
				e.logger.Info("event channel closed, flushing remaining events")
				return e.shutdown()
			}

			e.mu.Lock()
			e.buffer = append(e.buffer, event)
			bufLen := len(e.buffer)
			e.mu.Unlock()

			// Flush if buffer reaches batch_size.
			if bufLen >= e.cfg.Replication.BatchSize {
				if err := e.flush(context.Background()); err != nil {
					e.logger.Error("error flushing batch", zap.Error(err))
				}
			}

		case <-ticker.C:
			// Periodic flush based on flush_interval.
			e.mu.Lock()
			bufLen := len(e.buffer)
			e.mu.Unlock()

			if bufLen > 0 {
				if err := e.flush(context.Background()); err != nil {
					e.logger.Error("error flushing batch on interval", zap.Error(err))
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

	flushStart := time.Now()

	// Check circuit breaker before attempting to publish.
	if !e.cloudBreaker.Allow() {
		// Circuit is open — queue events back for later retry.
		e.mu.Lock()
		e.buffer = append(batch, e.buffer...)
		e.mu.Unlock()
		e.logger.Warn("circuit breaker open: queuing events for later retry",
			zap.Int("event_count", len(batch)),
			zap.String("circuit_state", e.cloudBreaker.State().String()),
		)
		return fmt.Errorf("circuit breaker open: cloud database unavailable")
	}

	// Publish batch to cloud with retry and backoff.
	publishBackoff := retry.BackoffConfig{
		InitialDelay: 1 * time.Second,
		MaxDelay:     8 * time.Second,
		Multiplier:   2.0,
	}
	publishErr := retry.Do(ctx, publishBackoff, 3, func(attempt int, retryErr error, nextDelay time.Duration) {
		e.logger.Warn("publish attempt failed, retrying",
			zap.Int("attempt", attempt+1),
			zap.Error(retryErr),
			zap.Duration("next_delay", nextDelay),
		)
	}, func(retryCtx context.Context) error {
		return e.publisher.Publish(retryCtx, batch)
	})
	if publishErr != nil {
		e.cloudBreaker.RecordFailure()
		// On failure, put events back into the buffer for retry.
		e.mu.Lock()
		e.buffer = append(batch, e.buffer...)
		e.state.EventsFailed += int64(len(batch))
		e.mu.Unlock()
		if e.metrics != nil {
			e.metrics.EventsFailed.Add(float64(len(batch)))
			e.metrics.DBConnectionErrors.Inc()
		}
		e.logger.Error("publishing batch failed",
			zap.Int("event_count", len(batch)),
			zap.Int("consecutive_failures", e.cloudBreaker.ConsecutiveFailures()),
			zap.Error(publishErr),
		)
		return fmt.Errorf("publishing batch of %d events: %w", len(batch), publishErr)
	}
	e.cloudBreaker.RecordSuccess()

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
			e.logger.Warn("failed to acknowledge LSN",
				zap.Uint64("lsn", uint64(maxLSN)),
				zap.Error(err),
			)
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

	// Record Prometheus metrics.
	if e.metrics != nil {
		e.metrics.EventsProcessed.Add(float64(len(batch)))
		e.metrics.BatchSize.Observe(float64(len(batch)))
		e.metrics.FlushDuration.Observe(time.Since(flushStart).Seconds())
		e.metrics.ReplicationLag.Set(stateSnapshot.CurrentLag.Seconds())
	}

	e.logger.Info("flushed events",
		zap.Int("batch_size", len(batch)),
		zap.Uint64("lsn", uint64(maxLSN)),
		zap.Duration("lag", stateSnapshot.CurrentLag.Round(time.Millisecond)),
		zap.Int64("total_processed", stateSnapshot.EventsProcessed),
	)

	// Persist state to disk.
	if err := e.stateStore.Save(&stateSnapshot); err != nil {
		e.logger.Warn("failed to persist replication state", zap.Error(err))
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
	e.logger.Info("shutting down replication engine")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- e.shutdownWork(shutdownCtx)
	}()

	select {
	case err := <-done:
		if err != nil {
			e.logger.Error("shutdown completed with errors", zap.Error(err))
		} else {
			e.logger.Info("shutdown complete")
		}
		return err
	case <-shutdownCtx.Done():
		e.logger.Warn("graceful shutdown exceeded timeout", zap.Duration("timeout", shutdownTimeout))
		return fmt.Errorf("shutdown timed out after %s", shutdownTimeout)
	}
}

func (e *Engine) shutdownWork(ctx context.Context) error {
	var shutdownErr error

	// Step 1: Flush remaining buffered events.
	if err := e.flush(ctx); err != nil {
		e.logger.Error("error flushing during shutdown", zap.Error(err))
		shutdownErr = err
	}

	// Step 2: Persist final state.
	e.mu.Lock()
	stateSnapshot := *e.state
	e.mu.Unlock()

	if err := e.stateStore.Save(&stateSnapshot); err != nil {
		e.logger.Error("error persisting state during shutdown", zap.Error(err))
		if shutdownErr == nil {
			shutdownErr = err
		}
	}

	// Step 3: Close subscriber.
	if err := e.subscriber.Close(); err != nil {
		e.logger.Error("error closing subscriber during shutdown", zap.Error(err))
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
		SSLRootCertPath: db.SSLRootCertPath,
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
