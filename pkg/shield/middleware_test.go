package shield

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dhanraj-12/Aegis/pkg/config"
	"github.com/dhanraj-12/Aegis/pkg/crdt"
)

func TestKeyExtractor_PriorityHierarchy(t *testing.T) {
	cfg := &config.Config{
		Protection: config.ProtectionConfig{
			RateLimit: config.RateLimitConfig{
				AuthRateLimit: 100,
				IPRateLimit:   10,
			},
		},
	}

	// 1. API Key takes priority over everything else
	req1 := httptest.NewRequest("GET", "/", nil)
	req1.Header.Set("X-API-Key", "secret-key-123")
	req1.Header.Set("Authorization", "Bearer token-abc")
	req1.Header.Set("CF-Connecting-IP", "1.1.1.1")
	req1.Header.Set("X-Forwarded-For", "2.2.2.2, 3.3.3.3")
	req1.RemoteAddr = "4.4.4.4:12345"

	id1, limit1 := ExtractIdentifier(req1, cfg)
	if id1 != "apikey:secret-key-123" || limit1 != 100 {
		t.Errorf("expected API Key, got id=%s, limit=%d", id1, limit1)
	}

	// 2. Authorization takes priority over IPs
	req2 := httptest.NewRequest("GET", "/", nil)
	req2.Header.Set("Authorization", "Bearer my-jwt-token")
	req2.Header.Set("CF-Connecting-IP", "1.1.1.1")
	req2.Header.Set("X-Forwarded-For", "2.2.2.2")
	req2.RemoteAddr = "4.4.4.4:12345"

	id2, limit2 := ExtractIdentifier(req2, cfg)
	if id2 != "auth:my-jwt-token" || limit2 != 100 {
		t.Errorf("expected Auth token, got id=%s, limit=%d", id2, limit2)
	}

	// 3. CF-Connecting-IP takes priority over X-Forwarded-For
	req3 := httptest.NewRequest("GET", "/", nil)
	req3.Header.Set("CF-Connecting-IP", "1.1.1.1")
	req3.Header.Set("X-Forwarded-For", "2.2.2.2, 3.3.3.3")
	req3.RemoteAddr = "4.4.4.4:12345"

	id3, limit3 := ExtractIdentifier(req3, cfg)
	if id3 != "ip:1.1.1.1" || limit3 != 10 {
		t.Errorf("expected CF IP, got id=%s, limit=%d", id3, limit3)
	}

	// 4. X-Forwarded-For first IP
	req4 := httptest.NewRequest("GET", "/", nil)
	req4.Header.Set("X-Forwarded-For", "2.2.2.2, 3.3.3.3")
	req4.RemoteAddr = "4.4.4.4:12345"

	id4, limit4 := ExtractIdentifier(req4, cfg)
	if id4 != "ip:2.2.2.2" || limit4 != 10 {
		t.Errorf("expected XFF first IP, got id=%s, limit=%d", id4, limit4)
	}

	// 5. Fallback RemoteAddr (IPv6 support)
	req5 := httptest.NewRequest("GET", "/", nil)
	req5.RemoteAddr = "[2001:db8::1]:8080"

	id5, limit5 := ExtractIdentifier(req5, cfg)
	if id5 != "ip:2001:db8::1" || limit5 != 10 {
		t.Errorf("expected IPv6 host from RemoteAddr, got id=%s, limit=%d", id5, limit5)
	}
}

func TestProtectionMiddleware_AllowAndBlock(t *testing.T) {
	limiter := crdt.NewLimiter("node-1", 1*time.Minute)
	defer limiter.Close()

	cfg := &config.Config{
		Protection: config.ProtectionConfig{
			Mode: "RateLimitOnly",
			RateLimit: config.RateLimitConfig{
				IPRateLimit: 2,
			},
		},
	}

	guard := NewRateLimitGuard(limiter)
	handlerCalled := 0
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled++
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	middleware := ProtectionMiddleware(guard, cfg, upstream, "httpbin.org")

	// 1st request -> allowed (200)
	req1 := httptest.NewRequest("GET", "/", nil)
	req1.RemoteAddr = "10.0.0.1:1234"
	rec1 := httptest.NewRecorder()
	middleware.ServeHTTP(rec1, req1)

	if rec1.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec1.Code)
	}

	// 2nd request -> allowed (200)
	req2 := httptest.NewRequest("GET", "/", nil)
	req2.RemoteAddr = "10.0.0.1:1234"
	rec2 := httptest.NewRecorder()
	middleware.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec2.Code)
	}

	// 3rd request -> blocked (429)
	req3 := httptest.NewRequest("GET", "/", nil)
	req3.RemoteAddr = "10.0.0.1:1234"
	rec3 := httptest.NewRecorder()
	middleware.ServeHTTP(rec3, req3)

	if rec3.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rec3.Code)
	}

	if handlerCalled != 2 {
		t.Fatalf("upstream should have only been invoked 2 times, got %d", handlerCalled)
	}
}
