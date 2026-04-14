package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all Prometheus metrics for the State Sync Engine.
type Metrics struct {
	// Replication metrics
	ReplicationLag    prometheus.Gauge
	EventsProcessed   prometheus.Counter
	EventsFailed      prometheus.Counter
	BatchSize         prometheus.Histogram
	FlushDuration     prometheus.Histogram

	// Connection metrics
	DBConnectionsActive prometheus.Gauge
	DBConnectionErrors  prometheus.Counter

	// Failover metrics
	ActiveNode         prometheus.Gauge
	FailoverEvents     prometheus.Counter
	SplitBrainEvents   prometheus.Counter

	// Lock metrics
	LockHeld            prometheus.Gauge
	LockAcquireDuration prometheus.Histogram
}

// New registers and returns all Prometheus metrics.
func New() *Metrics {
	return &Metrics{
		ReplicationLag: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "state_sync_replication_lag_seconds",
			Help: "Current replication lag in seconds.",
		}),
		EventsProcessed: promauto.NewCounter(prometheus.CounterOpts{
			Name: "state_sync_events_processed_total",
			Help: "Total change events processed.",
		}),
		EventsFailed: promauto.NewCounter(prometheus.CounterOpts{
			Name: "state_sync_events_failed_total",
			Help: "Failed replication attempts.",
		}),
		BatchSize: promauto.NewHistogram(prometheus.HistogramOpts{
			Name:    "state_sync_batch_size",
			Help:    "Number of events per batch.",
			Buckets: []float64{1, 5, 10, 25, 50, 100, 250, 500},
		}),
		FlushDuration: promauto.NewHistogram(prometheus.HistogramOpts{
			Name:    "state_sync_flush_duration_seconds",
			Help:    "Time to flush batch to cloud.",
			Buckets: prometheus.DefBuckets,
		}),
		DBConnectionsActive: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "state_sync_db_connections_active",
			Help: "Active database connections.",
		}),
		DBConnectionErrors: promauto.NewCounter(prometheus.CounterOpts{
			Name: "state_sync_db_connection_errors_total",
			Help: "Connection failure count.",
		}),
		ActiveNode: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "state_sync_active_node",
			Help: "Current active node (0=local, 1=cloud).",
		}),
		FailoverEvents: promauto.NewCounter(prometheus.CounterOpts{
			Name: "state_sync_failover_events_total",
			Help: "Number of failover events.",
		}),
		SplitBrainEvents: promauto.NewCounter(prometheus.CounterOpts{
			Name: "state_sync_split_brain_events_total",
			Help: "Split-brain condition detections.",
		}),
		LockHeld: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "state_sync_lock_held",
			Help: "Whether this instance holds the lock (0/1).",
		}),
		LockAcquireDuration: promauto.NewHistogram(prometheus.HistogramOpts{
			Name:    "state_sync_lock_acquire_duration_seconds",
			Help:    "Time to acquire lock.",
			Buckets: prometheus.DefBuckets,
		}),
	}
}
