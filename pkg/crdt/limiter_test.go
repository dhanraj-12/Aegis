package crdt

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestLimiter_GetOrCreate_Concurrent(t *testing.T) {
	l := NewLimiter("node-1", 1*time.Minute)
	defer l.Close()

	const goroutines = 50
	const key = "test-key:1700000000"
	var wg sync.WaitGroup

	counters := make([]*GCounter, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			counters[idx] = l.getOrCreate(key)
			counters[idx].Increment(fmt.Sprintf("node-%d", idx%3))
		}(i)
	}

	wg.Wait()

	// All returned pointers must point to the exact same GCounter instance.
	first := counters[0]
	for i := 1; i < goroutines; i++ {
		if counters[i] != first {
			t.Fatalf("goroutine %d received a different GCounter instance pointer", i)
		}
	}

	if val := first.Value(); val != goroutines {
		t.Fatalf("expected total count %d on single instance, got %d", goroutines, val)
	}
}

func TestLimiter_EvictStale(t *testing.T) {
	windowSize := 10 * time.Second
	l := NewLimiter("node-1", windowSize)
	defer l.Close()

	now := time.Now().Truncate(windowSize)
	currentKey := fmt.Sprintf("user-1:%d", now.Unix())
	prevKey := fmt.Sprintf("user-1:%d", now.Add(-windowSize).Unix())
	staleKey1 := fmt.Sprintf("user-1:%d", now.Add(-3*windowSize).Unix())
	staleIPv6 := fmt.Sprintf("ip:::1:%d", now.Add(-4*windowSize).Unix())

	// Create keys
	l.getOrCreate(currentKey).Increment("node-1")
	l.getOrCreate(prevKey).Increment("node-1")
	l.getOrCreate(staleKey1).Increment("node-1")
	l.getOrCreate(staleIPv6).Increment("node-1")

	l.mu.RLock()
	countBefore := len(l.states)
	l.mu.RUnlock()
	if countBefore != 4 {
		t.Fatalf("expected 4 states before eviction, got %d", countBefore)
	}

	// Trigger eviction
	l.EvictStale()

	l.mu.RLock()
	defer l.mu.RUnlock()

	if _, exists := l.states[currentKey]; !exists {
		t.Errorf("currentKey %s should not have been evicted", currentKey)
	}
	if _, exists := l.states[prevKey]; !exists {
		t.Errorf("prevKey %s should not have been evicted", prevKey)
	}
	if _, exists := l.states[staleKey1]; exists {
		t.Errorf("staleKey1 %s should have been evicted", staleKey1)
	}
	if _, exists := l.states[staleIPv6]; exists {
		t.Errorf("staleIPv6 %s should have been evicted", staleIPv6)
	}
}

func TestLimiter_SnapShot_And_MergeState_FiltersStale(t *testing.T) {
	windowSize := 10 * time.Second
	l := NewLimiter("node-1", windowSize)
	defer l.Close()

	now := time.Now().Truncate(windowSize)
	currentKey := fmt.Sprintf("user-1:%d", now.Unix())
	staleKey := fmt.Sprintf("user-1:%d", now.Add(-3*windowSize).Unix())

	l.getOrCreate(currentKey).Increment("node-1")
	l.getOrCreate(staleKey).Increment("node-1")

	// Snapshot should only contain active window, omitting staleKey
	snap := l.SnapShot()
	if _, exists := snap[currentKey]; !exists {
		t.Errorf("expected currentKey in snapshot")
	}
	if _, exists := snap[staleKey]; exists {
		t.Errorf("expected staleKey to be omitted from snapshot")
	}

	// MergeState with stale incoming key should not resurrect it in states
	remoteState := map[string]map[string]uint64{
		staleKey: {"node-remote": 5},
	}
	l.EvictStale() // clean local staleKey first
	l.MergeState(remoteState)

	l.mu.RLock()
	_, exists := l.states[staleKey]
	l.mu.RUnlock()
	if exists {
		t.Errorf("MergeState should ignore incoming stale keys")
	}
}

func TestLimiter_JanitorLoop_And_Close(t *testing.T) {
	windowSize := 1 * time.Second
	l := NewLimiter("node-1", windowSize)

	now := time.Now().Truncate(windowSize)
	staleKey := fmt.Sprintf("user-old:%d", now.Add(-10*time.Second).Unix())
	l.getOrCreate(staleKey).Increment("node-1")

	// Wait up to 2s for janitor ticker to run eviction
	deadline := time.Now().Add(2 * time.Second)
	evicted := false
	for time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
		l.mu.RLock()
		_, exists := l.states[staleKey]
		l.mu.RUnlock()
		if !exists {
			evicted = true
			break
		}
	}

	if !evicted {
		t.Errorf("expected staleKey to be evicted by background janitor loop")
	}

	// Ensure Close does not panic or leak
	if err := l.Close(); err != nil {
		t.Fatalf("unexpected error on Close: %v", err)
	}

	// Double Close should be idempotent
	if err := l.Close(); err != nil {
		t.Fatalf("second Close call failed: %v", err)
	}
}

func TestLimiter_Allow(t *testing.T) {
	l := NewLimiter("node-1", 1*time.Minute)
	defer l.Close()

	identifier := "client-abc"
	limit := uint64(5)

	for i := uint64(0); i < limit; i++ {
		if !l.Allow(identifier, limit) {
			t.Fatalf("expected request %d to be allowed", i+1)
		}
	}

	// Limit reached, next should be rejected
	if l.Allow(identifier, limit) {
		t.Fatalf("expected request beyond limit %d to be blocked", limit)
	}
}
