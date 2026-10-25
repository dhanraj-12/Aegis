package shield

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// AdaptiveConcurrencyGuard implements TCP Vegas-based adaptive concurrency limiting.
type AdaptiveConcurrencyGuard struct {
	// inFlight tracks current active requests via atomic primitives.
	inFlight atomic.Int64

	// Concurrency limits and parameters
	mu           sync.RWMutex
	currentLimit float64
	minLimit     float64
	maxLimit     float64
	alpha        float64
	beta         float64

	// RTT tracking (stored as nanoseconds for fast atomic reads)
	minRTT       atomic.Int64
	rttWindow    time.Duration
	lastRTTReset time.Time
}

// NewAdaptiveConcurrencyGuard creates a new TCP Vegas adaptive concurrency limiter.
func NewAdaptiveConcurrencyGuard(initial, minLimit, maxLimit, alpha, beta float64, rttWindow time.Duration) *AdaptiveConcurrencyGuard {
	if rttWindow <= 0 {
		rttWindow = 10 * time.Second
	}
	return &AdaptiveConcurrencyGuard{
		currentLimit: initial,
		minLimit:     minLimit,
		maxLimit:     maxLimit,
		alpha:        alpha,
		beta:         beta,
		rttWindow:    rttWindow,
		lastRTTReset: time.Now(),
	}
}

// Acquire reserves an in-flight slot if current concurrency is within limits.
func (g *AdaptiveConcurrencyGuard) Acquire(ctx context.Context, identifier string, limit uint64) (Decision, DoneFunc) {
	current := g.inFlight.Add(1)

	g.mu.RLock()
	currentLimit := g.currentLimit
	g.mu.RUnlock()

	if float64(current) > currentLimit {
		// Roll back the in-flight increment on load-shedding
		g.inFlight.Add(-1)
		return Decision{
			Allowed:    false,
			StatusCode: http.StatusServiceUnavailable,
			Reason:     "503 Service Unavailable: Concurrency limit saturated",
		}, func(int, time.Duration) {}
	}

	return Decision{Allowed: true}, func(statusCode int, latency time.Duration) {
		// Always decrement in-flight count upon request completion
		g.inFlight.Add(-1)

		// Do not adjust Vegas limits based on upstream 5xx errors or timeouts
		if statusCode >= 500 {
			return
		}

		g.updateVegas(latency)
	}
}

// updateVegas adjusts the concurrency limit using the TCP Vegas queue-size algorithm.
func (g *AdaptiveConcurrencyGuard) updateVegas(latency time.Duration) {
	rttNanos := latency.Nanoseconds()
	if rttNanos <= 0 {
		rttNanos = 1000 // 1 microsecond floor to prevent division by zero
	}

	storedMin := g.minRTT.Load()
	if storedMin == 0 || rttNanos < storedMin {
		g.minRTT.Store(rttNanos)
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	// Cold start: skip adjustment until we have at least one prior reference
	if storedMin == 0 {
		return
	}

	currentMin := g.minRTT.Load()
	if currentMin <= 0 {
		currentMin = rttNanos
	}

	// Vegas queue size estimation: Queue = Limit * (1 - RTT_min / RTT_curr)
	gradient := 1.0 - (float64(currentMin) / float64(rttNanos))
	queueSize := g.currentLimit * gradient

	if queueSize > g.beta {
		g.currentLimit = max(g.minLimit, g.currentLimit-1.0)
	} else if queueSize < g.alpha {
		g.currentLimit = min(g.maxLimit, g.currentLimit+1.0)
	}

	// Periodic minRTT probing reset to prevent stale baseline poisoning
	if time.Since(g.lastRTTReset) > g.rttWindow {
		g.minRTT.Store(rttNanos)
		g.lastRTTReset = time.Now()
	}
}

// InFlight returns the current number of active in-flight requests.
func (g *AdaptiveConcurrencyGuard) InFlight() int64 {
	return g.inFlight.Load()
}

// CurrentLimit returns the current concurrency limit.
func (g *AdaptiveConcurrencyGuard) CurrentLimit() float64 {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.currentLimit
}

// Close is a no-op as the guard has no background goroutines.
func (g *AdaptiveConcurrencyGuard) Close() error {
	return nil
}
