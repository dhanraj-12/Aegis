package gossip

import (
	"log"
	"net"
	"time"

	"github.com/dhanraj-12/Aegis/api/proto"
	"github.com/dhanraj-12/Aegis/pkg/config"
	"github.com/dhanraj-12/Aegis/pkg/crdt"
	"google.golang.org/protobuf/proto"
)

type Engine struct {
	limiter   *crdt.Limiter
	localnode string
	cfg       *config.Config
}

// NewEngine initializes the gossip engine.
func NewEngine(limiter *crdt.Limiter, localnode string, cfg *config.Config) *Engine {
	return &Engine{
		limiter:   limiter,
		localnode: localnode,
		cfg:       cfg,
	}
}

func (e *Engine) Start() {
	go e.listenUDP()
	go e.broadcastUDP()
}

func (e *Engine) listenUDP() {
	addr := &net.UDPAddr{IP: net.IPv4zero, Port: e.cfg.GossipPort}
	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		log.Fatalf("[Gossip] Failed to start the UDP listener: %v", err)
	}

	defer conn.Close()
	log.Printf("[Gossip] Listening for peer state on UDP port %d (%s)", e.cfg.GossipPort, e.localnode)

	buffer := make([]byte, 1400) // Standard safe MTU limit to avoid IP fragmentation
	var lastLogTime time.Time

	for {
		n, remoteAddr, err := conn.ReadFromUDP(buffer)
		if err != nil {
			log.Printf("[Gossip] Error reading UDP packet: %v", err)
			continue
		}

		// Deserialize the protobuf byte stream
		var payload pb.GossipPayload
		if err := proto.Unmarshal(buffer[:n], &payload); err != nil {
			log.Printf("[Gossip] Dropped malformed packet from %s: %v", remoteAddr.String(), err)
			continue
		}

		// Ignore our own broadcast loop if received accidentally
		if payload.SenderId == e.localnode {
			continue
		}

		remoteState := make(map[string]map[string]uint64, len(payload.States))

		// Map the protobuf schema to our raw Go maps
		for key, pbCounter := range payload.States {
			if pbCounter != nil {
				remoteState[key] = pbCounter.VectorClock
			}
		}

		// Merge the state
		e.limiter.MergeState(remoteState)

		if len(remoteState) > 0 && time.Since(lastLogTime) >= 1*time.Second {
			log.Printf("[Gossip] %s received & merged %d bucket(s) from peer %s", e.localnode, len(remoteState), payload.SenderId)
			lastLogTime = time.Now()
		}
	}
}

func (e *Engine) broadcastUDP() {
	// Ephemeral UDP socket for sending outbound datagrams
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		log.Fatalf("[Gossip] Failed to create outbound UDP socket: %v", err)
	}

	defer conn.Close()

	// Broadcast every 500 milliseconds
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	var lastLogTime time.Time

	for range ticker.C {
		// Thread-safe deep copy of local memory
		snapShot := e.limiter.SnapShot()
		if len(snapShot) == 0 {
			continue
		}

		// Map the raw Go map into protobuf structs
		pbStates := make(map[string]*pb.GCounter, len(snapShot))
		for key, vectorClock := range snapShot {
			pbStates[key] = &pb.GCounter{
				VectorClock: vectorClock,
			}
		}

		payload := &pb.GossipPayload{
			SenderId:  e.localnode,
			Timestamp: time.Now().UnixMilli(),
			States:    pbStates,
		}

		// Serialize to byte array
		data, err := proto.Marshal(payload)
		if err != nil {
			log.Printf("[Gossip] Protobuf serialization failed: %v", err)
			continue
		}

		// Broadcast the bytes to all known peers
		for _, peer := range e.cfg.Peers {
			addr, err := net.ResolveUDPAddr("udp4", peer)
			if err != nil {
				continue
			}
			conn.WriteToUDP(data, addr)
		}

		if time.Since(lastLogTime) >= 2*time.Second {
			log.Printf("[Gossip] %s broadcasting %d bucket(s) to %d peer(s)", e.localnode, len(pbStates), len(e.cfg.Peers))
			lastLogTime = time.Now()
		}
	}
}
