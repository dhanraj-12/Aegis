package gossip

import (
	"log"
	"net"

	"github.com/dhanraj-12/Aegis/pkg/config"
	"github.com/dhanraj-12/Aegis/pkg/crdt"
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
}


func (e *Engine) broadcastUDP() {

}