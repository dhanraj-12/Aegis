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

	
	configPath := flag.String("config","/home/dj/Drive2/aegis/config.yaml","Path to the configuration file")
	flag.Parse()
	
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	localNodeID := fmt.Sprintf("node-%d",cfg.GossipPort)
	limiter := crdt.NewLimiter(localNodeID,1*time.Minute)


	gossipEngine := gossip.NewEngine(limiter,localNodeID,cfg)
	gossipEngine.Start()


	backendURL,_ := url.Parse("https://httpbin.org")
	proxy := httputil.NewSingleHostReverseProxy(backendURL)

	http.HandleFunc("/",rateLimitMiddleware(limiter,cfg,proxy))

	log.Printf("[System] Aegis Reverse Proxy listening on HTTP :%d", cfg.ProxyPort)

	if err := http.ListenAndServe(fmt.Sprintf(":%d", cfg.ProxyPort),nil); err != nil {
		log.Fatalf("Server crashed: %v", err)
	}


}

func rateLimitMiddleware(limiter *crdt.Limiter, cfg *config.Config, proxy *httputil.ReverseProxy) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var identifier string
		var limit uint64
		
		authHeader := r.Header.Get("Authorization")

		if authHeader != "" {
			token := strings.TrimPrefix(authHeader,"Bearer")
			identifier = fmt.Sprintf("auth:%s",token)
			limit = uint64(cfg.AuthRateLimit)
		} else {
			ip := strings.Split(r.RemoteAddr, ":")[0]
			identifier = fmt.Sprintf("ip:%s",ip)
			limit = uint64(cfg.IPRateLimit)
		}

		if !limiter.Allow(identifier,limit) {
			log.Printf("[BLOCK] %s exceeded limit of %d", identifier, limit)
			http.Error(w,"429 Too many Requests - Aegis Shiel Activated", http.StatusTooManyRequests)
			return 
		}

		r.Host = "httpbin.org"

		log.Printf("[ALLOW] %s forwarded", identifier)
		proxy.ServeHTTP(w,r)
	}
}
