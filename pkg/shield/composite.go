package shield

import (
	"context"
	"time"

	"github.com/dhanraj-12/Aegis/pkg/config"
	"github.com/dhanraj-12/Aegis/pkg/crdt"
)

// ChainGuard runs multiple protection guards sequentially.
type ChainGuard struct {
	guards []Guard
}

// NewChainGuard creates a ChainGuard combining multiple guards.
func NewChainGuard(guards ...Guard) *ChainGuard {
	return &ChainGuard{guards: guards}
}

// Acquire evaluates each guard in order. If any guard rejects, previous acquisitions are rolled back.
func (c *ChainGuard) Acquire(ctx context.Context, identifier string, limit uint64) (Decision, DoneFunc) {
	var doneFuncs []DoneFunc

	for _, g := range c.guards {
		decision, done := g.Acquire(ctx, identifier, limit)
		if !decision.Allowed {
			// Roll back any previously acquired guards
			for _, fn := range doneFuncs {
				fn(decision.StatusCode, 0)
			}
			return decision, func(int, time.Duration) {}
		}
		doneFuncs = append(doneFuncs, done)
	}

	return Decision{Allowed: true}, func(statusCode int, latency time.Duration) {
		for _, fn := range doneFuncs {
			fn(statusCode, latency)
		}
	}
}

// Close closes all underlying guards.
func (c *ChainGuard) Close() error {
	for _, g := range c.guards {
		_ = g.Close()
	}
	return nil
}

// BuildGuard constructs the active Guard based on the configured Protection.Mode.
func BuildGuard(cfg *config.Config, rateLimiter *crdt.Limiter) Guard {
	switch cfg.Protection.Mode {
	case "ConcurrencyOnly":
		cc := cfg.Protection.Concurrency
		window, _ := time.ParseDuration(cc.WindowSize)
		return NewAdaptiveConcurrencyGuard(
			float64(cc.InitialLimit),
			float64(cc.MinInFlight),
			float64(cc.MaxInFlight),
			cc.Alpha,
			cc.Beta,
			window,
		)

	case "Strict":
		cc := cfg.Protection.Concurrency
		window, _ := time.ParseDuration(cc.WindowSize)
		concurrencyGuard := NewAdaptiveConcurrencyGuard(
			float64(cc.InitialLimit),
			float64(cc.MinInFlight),
			float64(cc.MaxInFlight),
			cc.Alpha,
			cc.Beta,
			window,
		)
		rateLimitGuard := NewRateLimitGuard(rateLimiter)
		// Run RateLimitGuard first, then ConcurrencyGuard
		return NewChainGuard(rateLimitGuard, concurrencyGuard)

	case "RateLimitOnly":
		fallthrough
	default:
		return NewRateLimitGuard(rateLimiter)
	}
}
