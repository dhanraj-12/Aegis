package crdt

import (
	"fmt"
	"sync"
	"time"
)


type Limiter struct {
	mu 	  		sync.RWMutex
	states 		map[string]*GCounter
	localNode 	string
	windowSize 	time.Duration
}

// NewLimiter creates a new Limiter engine.
func NewLimiter(localNode string, windowSize time.Duration) *Limiter {
		return &Limiter{
			states:  make(map[string]*GCounter),
			localNode: localNode,
			windowSize: windowSize,
		}
}


func (l *Limiter) getOrCreate(key string) *GCounter {
	
	// if already exist then return that 
	l.mu.RLock()
	counter,exist := l.states[key] 
	l.mu.RUnlock()
	if exist {
		return counter
	}


	// if not exist then create and reaturn 
	l.mu.Lock()
	defer l.mu.Unlock()
	newCounter := NewGCounter()
	l.states[key] = newCounter 
	return newCounter
}



// Allow check for the rolling window and return true if request is allowed and false is rejected for 429 too many request
func (l *Limiter) Allow(identifier string, limit uint64) bool {
	now := time.Now()

	
	// calculating the fixed window
	currentWindow := now.Truncate(l.windowSize)
	previousWindow := currentWindow.Add(-l.windowSize)

	// formate the keys to match protobuf schema
	currentKey := fmt.Sprintf("%s:%d", identifier, currentWindow.Unix())
	previousKey := fmt.Sprintf("%s:%d", identifier, previousWindow.Unix())


	// GCounters for the current and previous request
	currentCounter := l.getOrCreate(currentKey)
	previousCounter := l.getOrCreate(previousKey)

	// calculating time in current window and total window time in seconds
	timeIntoCurrent := now.Sub(currentWindow).Seconds()
	totalWindowSecond := l.windowSize.Seconds()

	// calculating the overlaping percentage
	overlapPercentage := (totalWindowSecond - timeIntoCurrent)/totalWindowSecond

	// getting the actual value for request from previous window and current window 
	previousCounts := float64(previousCounter.Value())
	currentCounts := float64(currentCounter.Value())


	// actuall calculation for number of request present in currrent rolling window
	estimatedCount := (previousCounts * overlapPercentage) + currentCounts

	if uint64(estimatedCount) >= limit {
		return false 
	}

	currentCounter.Increment(l.localNode)
	return true
}




// returning the deepCopy of State for UDP Gossip payload
func (l *Limiter) SnapShot() map[string]map[string]uint64 {
	l.mu.RLock()
	defer l.mu.RUnlock()
	snapShot := make(map[string]map[string]uint64,len(l.states))

	for key,gCounter := range l.states {
		snapShot[key] = gCounter.State()
	}

	return snapShot
}


// Merging the remote ingested State from a UDP gossip payload
func(l *Limiter) MergeState(remoteState map[string]map[string]uint64) {	
	for key, remoteGCounter := range remoteState {
		localCounter := l.getOrCreate(key)
		localCounter.Merge(remoteGCounter)
	}
}




