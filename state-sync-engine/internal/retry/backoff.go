package retry

import (
	"context"
	"fmt"
	"math"
	"time"
)

const (
	// DefaultInitialDelay is the starting backoff delay.
	DefaultInitialDelay = 1 * time.Second
	// DefaultMaxDelay caps the backoff at 60 seconds.
	DefaultMaxDelay = 60 * time.Second
	// DefaultMultiplier doubles the delay on each retry.
	DefaultMultiplier = 2.0
)

// BackoffConfig controls exponential backoff behavior.
type BackoffConfig struct {
	InitialDelay time.Duration
	MaxDelay     time.Duration
	Multiplier   float64
}

// DefaultBackoff returns a BackoffConfig with standard settings (1s, 2s, 4s, 8s, max 60s).
func DefaultBackoff() BackoffConfig {
	return BackoffConfig{
		InitialDelay: DefaultInitialDelay,
		MaxDelay:     DefaultMaxDelay,
		Multiplier:   DefaultMultiplier,
	}
}

// Delay returns the backoff delay for the given attempt number (0-indexed).
func (b BackoffConfig) Delay(attempt int) time.Duration {
	if attempt <= 0 {
		return b.InitialDelay
	}
	delay := float64(b.InitialDelay) * math.Pow(b.Multiplier, float64(attempt))
	if delay > float64(b.MaxDelay) {
		return b.MaxDelay
	}
	return time.Duration(delay)
}

// RetryFunc is a function that can be retried. It returns an error if it fails.
type RetryFunc func(ctx context.Context) error

// LogFunc is called on each retry attempt with the attempt number, error, and next delay.
type LogFunc func(attempt int, err error, nextDelay time.Duration)

// Do executes fn with exponential backoff until it succeeds, the context is
// cancelled, or maxAttempts is reached (0 = unlimited).
func Do(ctx context.Context, cfg BackoffConfig, maxAttempts int, logFn LogFunc, fn RetryFunc) error {
	for attempt := 0; ; attempt++ {
		err := fn(ctx)
		if err == nil {
			return nil
		}

		if maxAttempts > 0 && attempt+1 >= maxAttempts {
			return fmt.Errorf("failed after %d attempts: %w", attempt+1, err)
		}

		delay := cfg.Delay(attempt)

		if logFn != nil {
			logFn(attempt, err, delay)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
}
