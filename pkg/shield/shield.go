package shield

import (
	"context"
	"time"
)

// Decision defines the outcome of an acquisition attempt.
type Decision struct {
	Allowed    bool
	StatusCode int    // e.g., 429 for rate limit, 503 for concurrency shed
	Reason     string
}

// DoneFunc is executed upon HTTP response completion to record latency and release concurrency.
type DoneFunc func(statusCode int, latency time.Duration)

// Guard is the universal interface for protection mechanisms.
type Guard interface {
	// Acquire evaluates whether the request is allowed to proceed.
	// If Allowed is true, the caller MUST call the returned DoneFunc after the request completes.
	Acquire(ctx context.Context, identifier string, limit uint64) (Decision, DoneFunc)
	Close() error
}
