package gossip

import (
	"fmt"
	"testing"
	"time"

	"github.com/dhanraj-12/Aegis/pkg/config"
	"github.com/dhanraj-12/Aegis/pkg/crdt"
)

func TestGossip_TwoNodesSync(t *testing.T) {
	port1 := 19081
	port2 := 19083

	cfg1 := &config.Config{
		GossipPort: port1,
		Peers:      []string{fmt.Sprintf("127.0.0.1:%d", port2)},
	}
	cfg2 := &config.Config{
		GossipPort: port2,
		Peers:      []string{fmt.Sprintf("127.0.0.1:%d", port1)},
	}

	limiter1 := crdt.NewLimiter("node-1", 1*time.Minute)
	defer limiter1.Close()

	limiter2 := crdt.NewLimiter("node-2", 1*time.Minute)
	defer limiter2.Close()

	engine1 := NewEngine(limiter1, "node-1", cfg1)
	engine1.Start()

	engine2 := NewEngine(limiter2, "node-2", cfg2)
	engine2.Start()

	// Increment node-1
	limiter1.Allow("client-gossip-test", 100)

	// Wait up to 2 seconds for gossip ticker to sync to node-2
	deadline := time.Now().Add(2 * time.Second)
	synced := false

	for time.Now().Before(deadline) {
		snap2 := limiter2.SnapShot()
		for key, vectorClock := range snap2 {
			if vectorClock["node-1"] > 0 {
				synced = true
				_ = key
				break
			}
		}
		if synced {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if !synced {
		t.Fatalf("expected node-2 to receive state from node-1 via UDP gossip")
	}
}
