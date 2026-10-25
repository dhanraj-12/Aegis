package shield

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/dhanraj-12/Aegis/pkg/config"
	"github.com/dhanraj-12/Aegis/pkg/crdt"
)

func TestChain_RateLimitRejects_ConcurrencyNeverCalled(t *testing.T) {
	limiter := crdt.NewLimiter("node-1", 1*time.Minute)
	defer limiter.Close()

	rateGuard := NewRateLimitGuard(limiter)
	concGuard := NewAdaptiveConcurrencyGuard(10, 2, 20, 3.0, 6.0, 10*time.Second)

	chain := NewChainGuard(rateGuard, concGuard)

	// Consume rate limit (limit = 1)
	dec1, done1 := chain.Acquire(context.Background(), "user-x", 1)
	if !dec1.Allowed {
		t.Fatalf("first request should be allowed")
	}
	done1(http.StatusOK, 10*time.Millisecond)

	// Second request should be blocked by rate limit
	dec2, _ := chain.Acquire(context.Background(), "user-x", 1)
	if dec2.Allowed {
		t.Fatalf("second request should be blocked by rate limit")
	}
	if dec2.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", dec2.StatusCode)
	}

	// Concurrency guard inFlight should not have been incremented
	if inFlight := concGuard.InFlight(); inFlight != 0 {
		t.Fatalf("concurrency guard should have 0 inFlight, got %d", inFlight)
	}
}

func TestChain_RateLimitPasses_ConcurrencyRejects(t *testing.T) {
	limiter := crdt.NewLimiter("node-1", 1*time.Minute)
	defer limiter.Close()

	rateGuard := NewRateLimitGuard(limiter)
	concGuard := NewAdaptiveConcurrencyGuard(1, 1, 5, 3.0, 6.0, 10*time.Second)

	chain := NewChainGuard(rateGuard, concGuard)

	// Fill the 1 concurrency slot
	dec1, done1 := chain.Acquire(context.Background(), "user-1", 100)
	if !dec1.Allowed {
		t.Fatalf("first request should be allowed")
	}

	// Second request: rate limit allows (limit=100), but concurrency limit (1) rejects
	dec2, _ := chain.Acquire(context.Background(), "user-2", 100)
	if dec2.Allowed {
		t.Fatalf("second request should be rejected by concurrency guard")
	}
	if dec2.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 from concurrency shedding, got %d", dec2.StatusCode)
	}

	done1(http.StatusOK, 10*time.Millisecond)
}

func TestBuildGuard_Modes(t *testing.T) {
	limiter := crdt.NewLimiter("node-1", 1*time.Minute)
	defer limiter.Close()

	cfg := &config.Config{
		Protection: config.ProtectionConfig{
			Mode: "RateLimitOnly",
		},
	}
	g1 := BuildGuard(cfg, limiter)
	if _, ok := g1.(*RateLimitGuard); !ok {
		t.Fatalf("expected RateLimitGuard for RateLimitOnly")
	}

	cfg.Protection.Mode = "ConcurrencyOnly"
	cfg.Protection.Concurrency = config.ConcurrencyConfig{
		InitialLimit: 10,
		MinInFlight:  2,
		MaxInFlight:  20,
		Alpha:        3.0,
		Beta:         6.0,
		WindowSize:   "10s",
	}
	g2 := BuildGuard(cfg, limiter)
	if _, ok := g2.(*AdaptiveConcurrencyGuard); !ok {
		t.Fatalf("expected AdaptiveConcurrencyGuard for ConcurrencyOnly")
	}

	cfg.Protection.Mode = "Strict"
	g3 := BuildGuard(cfg, limiter)
	if _, ok := g3.(*ChainGuard); !ok {
		t.Fatalf("expected ChainGuard for Strict mode")
	}
}
