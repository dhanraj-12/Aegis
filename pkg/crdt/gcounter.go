package crdt

import "sync"

// this GCounter is representing the CRDT grow-only counter.
type GCounter struct {
	mu			sync.RWMutex
	counts 		map[string]uint64
}

// NewGcounter initialize.
func NewGCounter() *GCounter {
	return &GCounter{
		counts: make(map[string]uint64),
	}
}


// Increment safely add 1 to local counter
func(g *GCounter) Increment(nodeID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.counts[nodeID]++
}


// Value return the total count of the GCounter
func(g * GCounter) Value() uint64 {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var total uint64
	for _, count := range g.counts {
		total += count
	}
	return total
}

// Merg mathematically merget two GCounter states. 
func (g *GCounter) Merge(remoteCounts map[string]uint64) {
	g.mu.Lock()
	defer g.mu.Unlock()

	for nodeID, remoteCounts := range remoteCounts {
		if localCount, exists := g.counts[nodeID]; !exists || remoteCounts > localCount {
			g.counts[nodeID] = remoteCounts
		}
 	}
}


// State return the current copy of State
func (g *GCounter) State() map[string]uint64 {
	g.mu.RLock()
	defer g.mu.RUnlock()

	stateCopy := make(map[string]uint64, len(g.counts))
	for nodeId, count := range g.counts {
		stateCopy[nodeId] = count
	}
	return stateCopy
}	






