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
	limiter 	*crdt.Limiter
	localnode 	string
	cfg 		*config.Config
}

// Intialize the New Engine
func NewEngine(limiter *crdt.Limiter, localnode string, cfg *config.Config) *Engine {
	return &Engine{
		limiter: limiter,
		localnode: localnode,
		cfg: cfg,
	}
}

func (e *Engine) Start() {
	go e.listenUDP()
	go e.broadcastUDP()
}


func (e *Engine) listenUDP() {
	addr := &net.UDPAddr{Port: e.cfg.GossipPort}
	conn, err := net.ListenUDP("udp",addr)

	if err != nil {
		log.Fatalf("[Gossip] Failed to start the UDP listener %v", err)
	}

	defer conn.Close()
	log.Printf("[Gossip] Listen Peer State on UDP port %d", e.cfg.GossipPort)

	buffer := make([]byte,1400) // Standard safe MTU limit to avoid IP fragmentation

	for {
		n, remoteAddr, err := conn.ReadFromUDP(buffer)
		if err != nil {
			log.Printf("[Gossip] Error reading UDP packet: %v", err)
			log.Printf("[Gossip] Dropped malformed Packet from %s: %v", remoteAddr.String(), err)
			continue
		}

		
		// Desearlize the protobuf byte stream
		var paylaod pb.GossipPayload
		if err := proto.Unmarshal(buffer[:n], &paylaod); err != nil {
			log.Printf("[Gossip] Dropped malformed Packet from %s: %v", remoteAddr.String(), err)
			continue
		}

		// Ignore our own broadcast loop if we get accidently
		if paylaod.SenderId == e.localnode {
			continue
		}


		remoteState := make(map[string]map[string]uint64)


		// Map the protobuf schema to our raw Go maps
		for key, pbCounter := range paylaod.States {
			remoteState[key] = pbCounter.VectorClock
		}

		// Merege the State
		e.limiter.MergeState(remoteState)
	}
}


func (e *Engine) broadcastUDP() {
	conn,err := net.Dial("udp", "255.255.255.255:0") // Dummy dial to get UDP socket
	if err != nil {
		log.Fatalf("[Gossip] Failed to create the UDP socket: %v", err)
	}

	defer conn.Close()
	udpConn := conn.(*net.UDPConn)

	// broadcaast every 500 millisecond
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		// getting thread safe deep copy of localmemory
		snapShot := e.limiter.SnapShot()
		if len(snapShot) == 0 {
			continue
		}

		// Map the raw Go map in in protobuf stucts 
		pbStates := make(map[string]*pb.GCounter)
		for key,vectorClock := range snapShot {
			pbStates[key] = &pb.GCounter{
				VectorClock: vectorClock,
			}
		}


		payload := &pb.GossipPayload{
			SenderId: e.localnode,
			Timestamp: time.Now().UnixMilli(),
			States: pbStates,
		}

		// searlize it to byte array
		data, err := proto.Marshal(payload)
		if err != nil {
			log.Printf("[Gossip] Protobuf serialization failed: %v", err)
		}

		// broadcast the bytes to all know peers
		for _,peer := range e.cfg.Peers {
			addr,err := net.ResolveUDPAddr("udp",peer) 
			if err != nil {
				continue
			}
			udpConn.WriteToUDP(data,addr)
		}
	}

}