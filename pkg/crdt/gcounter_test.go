package crdt

import (
	"sync"
	"testing"
)

func TestGCounter_Basic(t *testing.T) {
	g := NewGCounter()

	g.Increment("node-1")
	g.Increment("node-1")
	g.Increment("node-2")

	if val := g.Value(); val != 3 {
		t.Fatalf("expected value 3, got %d", val)
	}

	state := g.State()
	if state["node-1"] != 2 || state["node-2"] != 1 {
		t.Fatalf("unexpected state: %+v", state)
	}
}

func TestGCounter_Merge(t *testing.T) {
	g1 := NewGCounter()
	g1.Increment("node-1")
	g1.Increment("node-1")

	g2 := NewGCounter()
	g2.Increment("node-1")
	g2.Increment("node-2")
	g2.Increment("node-2")

	// Merge g2 into g1
	g1.Merge(g2.State())

	if val := g1.Value(); val != 4 { // max(2, 1) + max(0, 2) = 2 + 2 = 4
		t.Fatalf("expected merged value 4, got %d", val)
	}
}

func TestGCounter_ConcurrentIncrements(t *testing.T) {
	g := NewGCounter()
	var wg sync.WaitGroup

	nodes := []string{"node-1", "node-2", "node-3"}
	const count = 100

	for _, node := range nodes {
		for i := 0; i < count; i++ {
			wg.Add(1)
			go func(n string) {
				defer wg.Done()
				g.Increment(n)
			}(node)
		}
	}

	wg.Wait()

	expected := uint64(len(nodes) * count)
	if val := g.Value(); val != expected {
		t.Fatalf("expected value %d, got %d", expected, val)
	}
}
