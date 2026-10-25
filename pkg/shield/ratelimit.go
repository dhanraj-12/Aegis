package shield

import (
	"context"
	"net/http"
	"time"

	"github.com/dhanraj-12/Aegis/pkg/crdt"
)

// RateLimitGuard wraps the CRDT sliding window rate limiter.
type RateLimitGuard struct {
	limiter *crdt.Limiter
}

// NewRateLimitGuard creates a new RateLimitGuard.
func NewRateLimitGuard(limiter *crdt.Limiter) *RateLimitGuard {
	return &RateLimitGuard{
		limiter: limiter,
	}
}

// Acquire tests velocity rate limit using CRDT sliding window.
func (g *RateLimitGuard) Acquire(ctx context.Context, identifier string, limit uint64) (Decision, DoneFunc) {
	if !g.limiter.Allow(identifier, limit) {
		return Decision{
			Allowed:    false,
			StatusCode: http.StatusTooManyRequests,
			Reason:     "429 Too Many Requests: Rate limit exceeded",
		}, func(int, time.Duration) {}
	}

	// Rate limiting does not require post-request tracking.
	return Decision{Allowed: true}, func(int, time.Duration) {}
}

// Close is a no-op as limiter lifecycle is managed by caller.
func (g *RateLimitGuard) Close() error {
	return nil
}
