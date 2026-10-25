package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/dhanraj-12/Aegis/pkg/config"
	"github.com/dhanraj-12/Aegis/pkg/crdt"
	"github.com/dhanraj-12/Aegis/pkg/gossip"
)

func main() {
	configPath := flag.String("config", "config.yaml", "Path to the configuration file")
	flag.Parse()

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	localNodeID := fmt.Sprintf("node-%d", cfg.GossipPort)
	window, err := time.ParseDuration(cfg.Protection.RateLimit.Window)
	if err != nil {
		window = 1 * time.Minute
	}
	limiter := crdt.NewLimiter(localNodeID, window)
	defer limiter.Close()

	gossipEngine := gossip.NewEngine(limiter, localNodeID, cfg)
	gossipEngine.Start()

	backendURL, err := url.Parse(cfg.BackendURL)
	if err != nil {
		log.Fatalf("Failed to parse BackendURL: %v", err)
	}
	proxy := httputil.NewSingleHostReverseProxy(backendURL)

	http.HandleFunc("/", rateLimitMiddleware(limiter, cfg, proxy, backendURL.Host))

	log.Printf("[System] Aegis Reverse Proxy listening on HTTP :%d -> Upstream: %s", cfg.ProxyPort, cfg.BackendURL)

	if err := http.ListenAndServe(fmt.Sprintf(":%d", cfg.ProxyPort), nil); err != nil {
		log.Fatalf("Server crashed: %v", err)
	}
}

func rateLimitMiddleware(limiter *crdt.Limiter, cfg *config.Config, proxy *httputil.ReverseProxy, targetHost string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var identifier string
		var limit uint64

		authHeader := r.Header.Get("Authorization")

		if authHeader != "" {
			token := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer"))
			identifier = fmt.Sprintf("auth:%s", token)
			limit = uint64(cfg.Protection.RateLimit.AuthRateLimit)
		} else {
			ip := strings.Split(r.RemoteAddr, ":")[0]
			identifier = fmt.Sprintf("ip:%s", ip)
			limit = uint64(cfg.Protection.RateLimit.IPRateLimit)
		}

		if !limiter.Allow(identifier, limit) {
			log.Printf("[BLOCK] %s exceeded limit of %d", identifier, limit)
			http.Error(w, "429 Too Many Requests - Aegis Shield Activated", http.StatusTooManyRequests)
			return
		}

		r.Host = targetHost

		log.Printf("[ALLOW] %s forwarded", identifier)
		proxy.ServeHTTP(w, r)
	}
}
