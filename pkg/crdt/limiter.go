package crdt

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Limiter struct {
	mu         sync.RWMutex
	states     map[string]*GCounter
	localNode  string
	windowSize time.Duration
	stopCh     chan struct{}
	closeOnce  sync.Once
}

// NewLimiter creates a new Limiter engine and starts its background cleanup janitor.
func NewLimiter(localNode string, windowSize time.Duration) *Limiter {
	l := &Limiter{
		states:     make(map[string]*GCounter),
		localNode:  localNode,
		windowSize: windowSize,
		stopCh:     make(chan struct{}),
	}

	go l.janitorLoop()
	return l
}

func (l *Limiter) getOrCreate(key string) *GCounter {
	// Fast read path: if already exists, return with read-lock
	l.mu.RLock()
	counter, exist := l.states[key]
	l.mu.RUnlock()
	if exist {
		return counter
	}

	// Write path: acquire exclusive lock
	l.mu.Lock()
	defer l.mu.Unlock()

	// Double-check after acquiring write lock to avoid duplicate counter creation
	if counter, exist = l.states[key]; exist {
		return counter
	}

	newCounter := NewGCounter()
	l.states[key] = newCounter
	return newCounter
}

// isStaleKey checks if a time-bucketed key has a timestamp strictly older than cutoffUnix.
func isStaleKey(key string, cutoffUnix int64) bool {
	// Use LastIndexByte so IPv6 addresses (e.g., ip:::1:1729600000) don't break timestamp parsing.
	lastColon := strings.LastIndexByte(key, ':')
	if lastColon == -1 {
		return false
	}

	ts, err := strconv.ParseInt(key[lastColon+1:], 10, 64)
	if err != nil {
		return false
	}

	return ts < cutoffUnix
}

// EvictStale removes window buckets older than 2 * windowSize to prevent memory leaks.
func (l *Limiter) EvictStale() {
	cutoffUnix := time.Now().Truncate(l.windowSize).Add(-2 * l.windowSize).Unix()

	l.mu.Lock()
	defer l.mu.Unlock()

	for key := range l.states {
		if isStaleKey(key, cutoffUnix) {
			delete(l.states, key)
		}
	}
}

// janitorLoop periodically evicts expired time-buckets in the background.
func (l *Limiter) janitorLoop() {
	interval := l.windowSize
	if interval <= 0 {
		interval = time.Minute
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			l.EvictStale()
		case <-l.stopCh:
			return
		}
	}
}

// Close stops the background janitor goroutine cleanly during shutdown.
func (l *Limiter) Close() error {
	l.closeOnce.Do(func() {
		close(l.stopCh)
	})
	return nil
}

// Allow checks the sliding window and returns true if the request is allowed, or false if rejected.
func (l *Limiter) Allow(identifier string, limit uint64) bool {
	now := time.Now()

	// Calculate fixed windows
	currentWindow := now.Truncate(l.windowSize)
	previousWindow := currentWindow.Add(-l.windowSize)

	// Format keys to match protobuf schema
	currentKey := fmt.Sprintf("%s:%d", identifier, currentWindow.Unix())
	previousKey := fmt.Sprintf("%s:%d", identifier, previousWindow.Unix())

	// GCounters for current and previous windows
	currentCounter := l.getOrCreate(currentKey)
	previousCounter := l.getOrCreate(previousKey)

	// Calculate time into current window and overlap percentage
	timeIntoCurrent := now.Sub(currentWindow).Seconds()
	totalWindowSecond := l.windowSize.Seconds()
	overlapPercentage := (totalWindowSecond - timeIntoCurrent) / totalWindowSecond

	// Get actual values from previous and current windows
	previousCounts := float64(previousCounter.Value())
	currentCounts := float64(currentCounter.Value())

	// Estimated count in the current rolling window
	estimatedCount := (previousCounts * overlapPercentage) + currentCounts

	if uint64(estimatedCount) >= limit {
		return false
	}

	currentCounter.Increment(l.localNode)
	return true
}

// SnapShot returns a thread-safe deep copy of active state for UDP gossip payload.
func (l *Limiter) SnapShot() map[string]map[string]uint64 {
	cutoffUnix := time.Now().Truncate(l.windowSize).Add(-2 * l.windowSize).Unix()

	l.mu.RLock()
	defer l.mu.RUnlock()
	snapShot := make(map[string]map[string]uint64, len(l.states))

	for key, gCounter := range l.states {
		if isStaleKey(key, cutoffUnix) {
			continue
		}
		snapShot[key] = gCounter.State()
	}

	return snapShot
}

// MergeState merges remote ingested state from a UDP gossip payload.
func (l *Limiter) MergeState(remoteState map[string]map[string]uint64) {
	cutoffUnix := time.Now().Truncate(l.windowSize).Add(-2 * l.windowSize).Unix()

	for key, remoteGCounter := range remoteState {
		if isStaleKey(key, cutoffUnix) {
			continue
		}
		localCounter := l.getOrCreate(key)
		localCounter.Merge(remoteGCounter)
	}
}
