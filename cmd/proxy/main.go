package main

import (
	"log"

	"github.com/dhanraj-12/Aegis/pkg/config"
)

func main() {

	log.Println("Proxy server starting...")
	cfg, err := config.LoadConfig("/home/dj/Drive2/aegis/config.yaml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	log.Printf("Bootstrapping Aegis with %d peers\n", len(cfg.Peers))
	log.Printf("Enforcing the sliding window limit %d\n", cfg.RateLimit)
}
