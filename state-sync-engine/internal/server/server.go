package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"

	"github.com/cloud-mirror/state-sync-engine/internal/state"
)

// StateProvider returns the current replication state.
type StateProvider interface {
	State() state.ReplicationState
}

// StatusResponse is the JSON body returned by the /status endpoint.
type StatusResponse struct {
	ActiveNode       string `json:"active_node"`
	ReplicationLag   string `json:"replication_lag"`
	LockHolder       string `json:"lock_holder"`
	LastFailoverTime string `json:"last_failover_time,omitempty"`
	EventsProcessed  int64  `json:"events_processed"`
	EventsFailed     int64  `json:"events_failed"`
	LastLSN          uint64 `json:"last_lsn"`
	LastFlushTime    string `json:"last_flush_time,omitempty"`
	SlotName         string `json:"slot_name"`
}

// MetricsServer serves the Prometheus /metrics endpoint.
type MetricsServer struct {
	server *http.Server
	logger *zap.Logger
}

// NewMetricsServer creates a metrics server on the given port.
func NewMetricsServer(port int, logger *zap.Logger) *MetricsServer {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	return &MetricsServer{
		server: &http.Server{
			Addr:    fmt.Sprintf(":%d", port),
			Handler: mux,
		},
		logger: logger,
	}
}

// Start begins listening. Returns immediately; call Shutdown to stop.
func (s *MetricsServer) Start() error {
	s.logger.Info("metrics server starting", zap.String("addr", s.server.Addr))
	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("metrics server error", zap.Error(err))
		}
	}()
	return nil
}

// Shutdown gracefully stops the metrics server.
func (s *MetricsServer) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

// StatusServer serves the /status health endpoint.
type StatusServer struct {
	server   *http.Server
	provider StateProvider
	logger   *zap.Logger
	maxLag   time.Duration
}

// NewStatusServer creates a status server on the given port.
func NewStatusServer(port int, provider StateProvider, maxLag time.Duration, logger *zap.Logger) *StatusServer {
	s := &StatusServer{
		provider: provider,
		logger:   logger,
		maxLag:   maxLag,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/status", s.handleStatus)

	s.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}
	return s
}

// Start begins listening. Returns immediately; call Shutdown to stop.
func (s *StatusServer) Start() error {
	s.logger.Info("status server starting", zap.String("addr", s.server.Addr))
	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("status server error", zap.Error(err))
		}
	}()
	return nil
}

// Shutdown gracefully stops the status server.
func (s *StatusServer) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

func (s *StatusServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	st := s.provider.State()

	resp := StatusResponse{
		ActiveNode:      string(st.ActiveNode),
		ReplicationLag:  st.CurrentLag.String(),
		LockHolder:      st.LockHolder,
		EventsProcessed: st.EventsProcessed,
		EventsFailed:    st.EventsFailed,
		LastLSN:         uint64(st.LastLSN),
		SlotName:        st.SlotName,
	}
	if !st.LastFailoverTime.IsZero() {
		resp.LastFailoverTime = st.LastFailoverTime.Format(time.RFC3339)
	}
	if !st.LastFlushTime.IsZero() {
		resp.LastFlushTime = st.LastFlushTime.Format(time.RFC3339)
	}

	// Determine health: unhealthy if lag exceeds maxLag.
	healthy := true
	if s.maxLag > 0 && st.CurrentLag > s.maxLag {
		healthy = false
	}

	w.Header().Set("Content-Type", "application/json")
	if healthy {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	if err := json.NewEncoder(w).Encode(resp); err != nil {
		s.logger.Error("failed to encode status response", zap.Error(err))
	}
}
