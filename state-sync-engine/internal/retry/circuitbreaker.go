package retry

import (
	"errors"
	"sync"
	"time"
)

// CircuitState represents the state of a circuit breaker.
type CircuitState int

const (
	CircuitClosed   CircuitState = iota // Normal operation, requests pass through.
	CircuitOpen                         // Failures exceeded threshold, requests are blocked.
	CircuitHalfOpen                     // Testing if the service has recovered.
)

func (s CircuitState) String() string {
	switch s {
	case CircuitClosed:
		return "closed"
	case CircuitOpen:
		return "open"
	case CircuitHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// ErrCircuitOpen is returned when the circuit breaker is open.
var ErrCircuitOpen = errors.New("circuit breaker is open")

// CircuitBreaker tracks consecutive failures and opens the circuit
// after a configured threshold. When open, it blocks requests and
// periodically allows a single probe to test recovery.
type CircuitBreaker struct {
	mu                  sync.Mutex
	state               CircuitState
	consecutiveFailures int
	failureThreshold    int
	retryInterval       time.Duration
	lastFailureTime     time.Time
	now                 func() time.Time // injectable clock for testing
}

// CircuitBreakerConfig configures the circuit breaker.
type CircuitBreakerConfig struct {
	FailureThreshold int           // Number of consecutive failures to open circuit (default 5).
	RetryInterval    time.Duration // How long to wait before retrying when open (default 5 min).
}

// NewCircuitBreaker creates a circuit breaker with the given config.
func NewCircuitBreaker(cfg CircuitBreakerConfig) *CircuitBreaker {
	if cfg.FailureThreshold <= 0 {
		cfg.FailureThreshold = 5
	}
	if cfg.RetryInterval <= 0 {
		cfg.RetryInterval = 5 * time.Minute
	}
	return &CircuitBreaker{
		state:            CircuitClosed,
		failureThreshold: cfg.FailureThreshold,
		retryInterval:    cfg.RetryInterval,
		now:              time.Now,
	}
}

// Allow checks whether a request should be allowed through.
// Returns true if the circuit is closed or half-open (probe allowed).
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitClosed:
		return true
	case CircuitOpen:
		if cb.now().Sub(cb.lastFailureTime) >= cb.retryInterval {
			cb.state = CircuitHalfOpen
			return true
		}
		return false
	case CircuitHalfOpen:
		// Only one probe at a time; block additional requests.
		return false
	default:
		return false
	}
}

// RecordSuccess records a successful operation. Resets the circuit to closed.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.consecutiveFailures = 0
	cb.state = CircuitClosed
}

// RecordFailure records a failed operation. May open the circuit.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.consecutiveFailures++
	cb.lastFailureTime = cb.now()

	if cb.consecutiveFailures >= cb.failureThreshold {
		cb.state = CircuitOpen
	}
}

// State returns the current circuit state.
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}

// ConsecutiveFailures returns the current consecutive failure count.
func (cb *CircuitBreaker) ConsecutiveFailures() int {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.consecutiveFailures
}
