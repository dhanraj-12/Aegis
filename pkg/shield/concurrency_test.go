package shield

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"
)

func TestAcquire_BelowLimit(t *testing.T) {
	guard := NewAdaptiveConcurrencyGuard(10, 2, 20, 3.0, 6.0, 10*time.Second)

	decision, done := guard.Acquire(context.Background(), "user-1", 100)
	if !decision.Allowed {
		t.Fatalf("expected request below limit to be allowed")
	}
	if inFlight := guard.InFlight(); inFlight != 1 {
		t.Fatalf("expected inFlight = 1, got %d", inFlight)
	}

	done(http.StatusOK, 10*time.Millisecond)
	if inFlight := guard.InFlight(); inFlight != 0 {
		t.Fatalf("expected inFlight to decrement to 0, got %d", inFlight)
	}
}

func TestAcquire_AtLimit_Sheds(t *testing.T) {
	guard := NewAdaptiveConcurrencyGuard(2, 2, 10, 3.0, 6.0, 10*time.Second)

	// Fill the 2 slots
	d1, done1 := guard.Acquire(context.Background(), "user-1", 100)
	d2, done2 := guard.Acquire(context.Background(), "user-2", 100)
	if !d1.Allowed || !d2.Allowed {
		t.Fatalf("first two requests should be allowed")
	}

	// Third request exceeds limit of 2 -> should return 503
	d3, _ := guard.Acquire(context.Background(), "user-3", 100)
	if d3.Allowed {
		t.Fatalf("third request should be shed")
	}
	if d3.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d", d3.StatusCode)
	}
	if inFlight := guard.InFlight(); inFlight != 2 {
		t.Fatalf("inFlight should remain at 2 after shed rollback, got %d", inFlight)
	}

	done1(http.StatusOK, 10*time.Millisecond)
	done2(http.StatusOK, 10*time.Millisecond)
}

func TestVegas_LimitAdjustments(t *testing.T) {
	// min=2, max=20, initial=10, alpha=3, beta=6
	guard := NewAdaptiveConcurrencyGuard(10, 2, 20, 3.0, 6.0, 10*time.Second)

	// Seed baseline minRTT = 20ms
	_, d1 := guard.Acquire(context.Background(), "u1", 0)
	d1(http.StatusOK, 20*time.Millisecond)

	initialLimit := guard.CurrentLimit()

	// 1. Simulate fast responses (RTT = 20ms = minRTT) -> queue = 10 * (1 - 20/20) = 0 < alpha (3)
	// Vegas should increase currentLimit
	_, d2 := guard.Acquire(context.Background(), "u2", 0)
	d2(http.StatusOK, 20*time.Millisecond)

	if guard.CurrentLimit() <= initialLimit {
		t.Errorf("expected limit to increase when queue is low, got %f", guard.CurrentLimit())
	}

	// 2. Simulate slow responses (RTT = 200ms) -> gradient = 1 - 20/200 = 0.9 -> queue = Limit * 0.9 > beta (6)
	// Vegas should decrease currentLimit
	for i := 0; i < 5; i++ {
		_, done := guard.Acquire(context.Background(), "u3", 0)
		done(http.StatusOK, 200*time.Millisecond)
	}

	if guard.CurrentLimit() >= initialLimit {
		t.Errorf("expected limit to decrease under high latency, got %f", guard.CurrentLimit())
	}
}

func TestVegas_IgnoresServerErrors(t *testing.T) {
	guard := NewAdaptiveConcurrencyGuard(10, 2, 20, 3.0, 6.0, 10*time.Second)

	// Seed baseline
	_, d1 := guard.Acquire(context.Background(), "u1", 0)
	d1(http.StatusOK, 20*time.Millisecond)

	limitBefore := guard.CurrentLimit()

	// Return a 502 Bad Gateway with huge latency (5000ms)
	_, d2 := guard.Acquire(context.Background(), "u2", 0)
	d2(http.StatusBadGateway, 5000*time.Millisecond)

	if guard.CurrentLimit() != limitBefore {
		t.Errorf("5xx errors should not alter Vegas limits, got limit %f vs before %f", guard.CurrentLimit(), limitBefore)
	}
	if inFlight := guard.InFlight(); inFlight != 0 {
		t.Errorf("inFlight should still decrement to 0 on 5xx, got %d", inFlight)
	}
}

func TestVegas_MinRTTReset(t *testing.T) {
	shortWindow := 50 * time.Millisecond
	guard := NewAdaptiveConcurrencyGuard(10, 2, 20, 3.0, 6.0, shortWindow)

	// Baseline fast hit: 5ms
	_, d1 := guard.Acquire(context.Background(), "u1", 0)
	d1(http.StatusOK, 5*time.Millisecond)

	// Wait for rttWindow to expire
	time.Sleep(60 * time.Millisecond)

	// Next response has higher latency: 50ms
	_, d2 := guard.Acquire(context.Background(), "u2", 0)
	d2(http.StatusOK, 50*time.Millisecond)

	// minRTT should have reset to the new reading
	storedMin := guard.minRTT.Load()
	if storedMin < (50 * time.Millisecond).Nanoseconds() {
		t.Errorf("expected minRTT to be updated after window expiration")
	}
}

func TestVegas_ConcurrentAccess(t *testing.T) {
	guard := NewAdaptiveConcurrencyGuard(50, 10, 100, 3.0, 6.0, 5*time.Second)
	var wg sync.WaitGroup

	const routines = 100
	const iterations = 50

	for i := 0; i < routines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				dec, done := guard.Acquire(context.Background(), "user-concurrency", 0)
				if dec.Allowed {
					time.Sleep(time.Duration(id%5) * time.Millisecond)
					done(http.StatusOK, time.Duration(10+id%20)*time.Millisecond)
				}
			}
		}(i)
	}

	wg.Wait()

	if inFlight := guard.InFlight(); inFlight != 0 {
		t.Fatalf("expected all in-flight slots to be released, got %d", inFlight)
	}
}

func TestVegas_RespectsBounds(t *testing.T) {
	minLimit := 5.0
	maxLimit := 15.0
	guard := NewAdaptiveConcurrencyGuard(10.0, minLimit, maxLimit, 3.0, 6.0, 10*time.Second)

	// Force decrease repeatedly
	_, d := guard.Acquire(context.Background(), "u", 0)
	d(http.StatusOK, 10*time.Millisecond) // seed

	for i := 0; i < 30; i++ {
		_, done := guard.Acquire(context.Background(), "u", 0)
		done(http.StatusOK, 1000*time.Millisecond)
	}

	if guard.CurrentLimit() < minLimit {
		t.Fatalf("limit %f dropped below minLimit %f", guard.CurrentLimit(), minLimit)
	}

	// Force increase repeatedly
	for i := 0; i < 30; i++ {
		_, done := guard.Acquire(context.Background(), "u", 0)
		done(http.StatusOK, 10*time.Millisecond)
	}

	if guard.CurrentLimit() > maxLimit {
		t.Fatalf("limit %f exceeded maxLimit %f", guard.CurrentLimit(), maxLimit)
	}
}
