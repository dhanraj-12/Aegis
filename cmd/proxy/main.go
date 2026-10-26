package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dhanraj-12/Aegis/pkg/config"
	"github.com/dhanraj-12/Aegis/pkg/crdt"
	"github.com/dhanraj-12/Aegis/pkg/gossip"
	"github.com/dhanraj-12/Aegis/pkg/shield"
)

func main() {
	configPath := flag.String("config", "config.yaml", "Path to the configuration file")
	flag.Parse()

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// 1. Parse upstream backend URL
	backendURL, err := url.Parse(cfg.BackendURL)
	if err != nil {
		log.Fatalf("Failed to parse BackendURL: %v", err)
	}

	// 2. Initialize CRDT rate limiter and gossip engine if enabled by protection mode
	var rateLimiter *crdt.Limiter
	var gossipEngine *gossip.Engine
	localNodeID := fmt.Sprintf("node-%d", cfg.GossipPort)

	if cfg.Protection.Mode != "ConcurrencyOnly" {
		window, err := time.ParseDuration(cfg.Protection.RateLimit.Window)
		if err != nil {
			window = 1 * time.Minute
		}
		rateLimiter = crdt.NewLimiter(localNodeID, window)
		defer rateLimiter.Close()

		gossipEngine = gossip.NewEngine(rateLimiter, localNodeID, cfg)
		gossipEngine.Start()
	}

	// 3. Build unified Guard strategy (RateLimitOnly, ConcurrencyOnly, or Strict)
	guard := shield.BuildGuard(cfg, rateLimiter)
	defer guard.Close()

	// 4. Initialize reverse proxy with high-concurrency connection pooling
	proxy := httputil.NewSingleHostReverseProxy(backendURL)
	proxy.Transport = &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          2000,
		MaxIdleConnsPerHost:   1000,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	// 5. Mount protection middleware
	http.Handle("/", shield.ProtectionMiddleware(guard, cfg, proxy, backendURL.Host))

	// 6. Graceful shutdown handler
	server := &http.Server{
		Addr: fmt.Sprintf(":%d", cfg.ProxyPort),
	}

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		sig := <-sigCh
		log.Printf("[System] Caught signal %v, initiating graceful shutdown...", sig)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := server.Shutdown(ctx); err != nil {
			log.Printf("[System] HTTP server shutdown error: %v", err)
		}
		_ = guard.Close()
		log.Println("[System] Aegis Reverse Proxy shutdown complete.")
	}()

	log.Printf("[System] Aegis Reverse Proxy listening on HTTP :%d (Mode: %s) -> Upstream: %s",
		cfg.ProxyPort, cfg.Protection.Mode, cfg.BackendURL)

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server crashed: %v", err)
	}
}
