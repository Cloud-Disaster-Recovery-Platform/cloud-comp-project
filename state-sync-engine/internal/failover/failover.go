package failover

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/cloud-mirror/state-sync-engine/internal/config"
	"github.com/cloud-mirror/state-sync-engine/internal/metrics"
	"github.com/cloud-mirror/state-sync-engine/internal/state"
	"github.com/cloud-mirror/state-sync-engine/pkg/interfaces"
)

// Manager coordinates failover detection, recovery, and split-brain resolution.
type Manager struct {
	cfg         *config.Config
	lock        interfaces.DistributedLock
	coordinator interfaces.StateCoordinator
	publisher   interfaces.ReplicationPublisher
	logger      *zap.Logger
	metrics     *metrics.Metrics
	nodeID      string

	mu           sync.Mutex
	activeNode   state.NodeType
	lockHeld     bool
	lastFailover time.Time
}

// NewManager creates a new failover manager.
func NewManager(
	cfg *config.Config,
	lock interfaces.DistributedLock,
	coordinator interfaces.StateCoordinator,
	publisher interfaces.ReplicationPublisher,
	logger *zap.Logger,
	m *metrics.Metrics,
	nodeID string,
) *Manager {
	return &Manager{
		cfg:         cfg,
		lock:        lock,
		coordinator: coordinator,
		publisher:   publisher,
		logger:      logger,
		metrics:     m,
		nodeID:      nodeID,
		activeNode:  state.NodeLocal,
	}
}

// Run starts the periodic failover check loop. Blocks until ctx is cancelled.
func (fm *Manager) Run(ctx context.Context) error {
	interval := fm.cfg.Health.CheckInterval
	if interval <= 0 {
		interval = 10 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	fm.logger.Info("failover manager started",
		zap.String("node_id", fm.nodeID),
		zap.Duration("check_interval", interval),
	)

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			fm.check(ctx)
		}
	}
}

// check performs a single failover check cycle.
func (fm *Manager) check(ctx context.Context) {
	holder, err := fm.lock.GetHolder(ctx, fm.cfg.DistributedLock.LockKey)
	if err != nil {
		fm.logger.Warn("failed to check lock holder", zap.Error(err))
		return
	}

	fm.mu.Lock()
	previousNode := fm.activeNode
	fm.mu.Unlock()

	if holder != "" && holder != fm.nodeID {
		// Another node holds the lock — cloud is active.
		fm.handleCloudActive(ctx, holder, previousNode)
	} else if holder == fm.nodeID || holder == "" {
		// We hold the lock or no one does — local is active.
		fm.handleLocalActive(ctx, holder, previousNode)
	}

	// Split-brain detection: check if both nodes think they are active.
	fm.detectSplitBrain(ctx, holder)
}

// handleCloudActive coordinates the local side when cloud becomes the active writer.
func (fm *Manager) handleCloudActive(ctx context.Context, holder string, previousNode state.NodeType) {
	fm.mu.Lock()
	if fm.activeNode == state.NodeCloud {
		fm.mu.Unlock()
		return // Already in cloud-active state.
	}
	fm.activeNode = state.NodeCloud
	fm.lastFailover = time.Now()
	fm.mu.Unlock()

	fm.logger.Info("failover detected: cloud is now active",
		zap.String("lock_holder", holder),
		zap.String("previous_node", string(previousNode)),
	)

	if fm.metrics != nil {
		fm.metrics.ActiveNode.Set(1) // 1 = cloud
		fm.metrics.FailoverEvents.Inc()
	}

	// Set local database to read-only.
	localDB := toDBConfig(fm.cfg.LocalDatabase)
	if err := fm.coordinator.SetReadOnly(ctx, localDB); err != nil {
		fm.logger.Error("failed to set local database to read-only during failover",
			zap.Error(err),
		)
		// Continue even if we can't reach the local DB — it may be down.
	} else {
		fm.logger.Info("local database set to read-only")
	}
}

// handleLocalActive coordinates recovery when the local node becomes active again.
func (fm *Manager) handleLocalActive(ctx context.Context, holder string, previousNode state.NodeType) {
	fm.mu.Lock()
	if fm.activeNode == state.NodeLocal {
		fm.mu.Unlock()
		return // Already in local-active state.
	}
	fm.activeNode = state.NodeLocal
	fm.mu.Unlock()

	fm.logger.Info("recovery detected: local is now active",
		zap.String("lock_holder", holder),
		zap.String("previous_node", string(previousNode)),
	)

	if fm.metrics != nil {
		fm.metrics.ActiveNode.Set(0) // 0 = local
	}

	// Recovery coordination: sync cloud changes back and set local to read-write.
	fm.recoverLocal(ctx)
}

// recoverLocal synchronizes cloud-side changes to local and enables writes.
func (fm *Manager) recoverLocal(ctx context.Context) {
	// Verify cloud database is reachable to sync any changes.
	if err := fm.publisher.HealthCheck(ctx); err != nil {
		fm.logger.Warn("cloud database unreachable during recovery; proceeding with local restore",
			zap.Error(err),
		)
	} else {
		fm.logger.Info("cloud database reachable, changes will sync via normal replication")
	}

	// Set local database back to read-write.
	localDB := toDBConfig(fm.cfg.LocalDatabase)
	if err := fm.coordinator.SetReadWrite(ctx, localDB); err != nil {
		fm.logger.Error("failed to set local database to read-write during recovery",
			zap.Error(err),
		)
		return
	}
	fm.logger.Info("local database restored to read-write")

	// Release the distributed lock if we hold it.
	if err := fm.lock.Release(ctx, fm.cfg.DistributedLock.LockKey); err != nil {
		fm.logger.Warn("failed to release lock during recovery", zap.Error(err))
	} else {
		fm.mu.Lock()
		fm.lockHeld = false
		fm.mu.Unlock()
		if fm.metrics != nil {
			fm.metrics.LockHeld.Set(0)
		}
		fm.logger.Info("distributed lock released after recovery")
	}
}

// detectSplitBrain checks if both nodes think they are the active writer.
func (fm *Manager) detectSplitBrain(ctx context.Context, lockHolder string) {
	if lockHolder == "" {
		return // No one holds the lock, no split-brain possible.
	}

	// Check both database states to detect dual-active.
	localDB := toDBConfig(fm.cfg.LocalDatabase)
	localState, err := fm.coordinator.GetState(ctx, localDB)
	if err != nil {
		// Can't determine local state, skip detection.
		return
	}

	// Split-brain: lock is held by cloud but local is still in read-write mode.
	if lockHolder != fm.nodeID && localState == interfaces.DatabaseStateReadWrite {
		fm.logger.Error("SPLIT-BRAIN DETECTED: both nodes appear active",
			zap.String("lock_holder", lockHolder),
			zap.String("local_state", string(localState)),
			zap.String("resolution", "cloud is authoritative"),
			zap.Time("detected_at", time.Now()),
		)

		if fm.metrics != nil {
			fm.metrics.SplitBrainEvents.Inc()
		}

		// Resolution: cloud is authoritative — force local to read-only.
		if err := fm.coordinator.SetReadOnly(ctx, localDB); err != nil {
			fm.logger.Error("failed to resolve split-brain: could not set local to read-only",
				zap.Error(err),
			)
		} else {
			fm.logger.Info("split-brain resolved: local set to read-only, cloud is authoritative")
		}
	}
}

// ActiveNode returns the current active node type.
func (fm *Manager) ActiveNode() state.NodeType {
	fm.mu.Lock()
	defer fm.mu.Unlock()
	return fm.activeNode
}

// LastFailoverTime returns the time of the last failover event.
func (fm *Manager) LastFailoverTime() time.Time {
	fm.mu.Lock()
	defer fm.mu.Unlock()
	return fm.lastFailover
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
