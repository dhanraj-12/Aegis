package shield

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/dhanraj-12/Aegis/pkg/config"
)

// ExtractIdentifier extracts the rate limit identifier using the strict 4-tier hierarchy:
// 1. X-API-Key header
// 2. Authorization header (Bearer Token / JWT)
// 3. CF-Connecting-IP / X-Forwarded-For (First IP in comma-separated list)
// 4. Fallback: Raw http.Request.RemoteAddr
func ExtractIdentifier(r *http.Request, cfg *config.Config) (identifier string, limit uint64) {
	// 1. X-API-Key header
	apiKey := r.Header.Get("X-API-Key")
	if apiKey != "" {
		return fmt.Sprintf("apikey:%s", strings.TrimSpace(apiKey)), uint64(cfg.Protection.RateLimit.AuthRateLimit)
	}

	// 2. Authorization header
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		token := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer"))
		return fmt.Sprintf("auth:%s", token), uint64(cfg.Protection.RateLimit.AuthRateLimit)
	}

	// 3. Cloudflare Connecting IP
	cfIP := r.Header.Get("CF-Connecting-IP")
	if cfIP != "" {
		return fmt.Sprintf("ip:%s", strings.TrimSpace(cfIP)), uint64(cfg.Protection.RateLimit.IPRateLimit)
	}

	// 3. X-Forwarded-For (First IP in comma-separated chain)
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		parts := strings.Split(xff, ",")
		firstIP := strings.TrimSpace(parts[0])
		if firstIP != "" {
			return fmt.Sprintf("ip:%s", firstIP), uint64(cfg.Protection.RateLimit.IPRateLimit)
		}
	}

	// 4. Fallback: RemoteAddr using net.SplitHostPort to support both IPv4 and IPv6
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return fmt.Sprintf("ip:%s", strings.TrimSpace(host)), uint64(cfg.Protection.RateLimit.IPRateLimit)
}

// statusRecorder captures the response status code written by upstream handlers.
type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

// ProtectionMiddleware enforces rate limiting and adaptive concurrency shedding.
func ProtectionMiddleware(guard Guard, cfg *config.Config, next http.Handler, targetHost string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identifier, limit := ExtractIdentifier(r, cfg)
		decision, done := guard.Acquire(r.Context(), identifier, limit)

		if !decision.Allowed {
			log.Printf("[BLOCK] %s rejected: %s (status %d)", identifier, decision.Reason, decision.StatusCode)
			http.Error(w, decision.Reason, decision.StatusCode)
			return
		}

		start := time.Now()
		recorder := &statusRecorder{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		defer func() {
			done(recorder.statusCode, time.Since(start))
		}()

		if targetHost != "" {
			r.Host = targetHost
		}

		log.Printf("[ALLOW] %s forwarded", identifier)
		next.ServeHTTP(recorder, r)
	}
}
