package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTempConfigFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	filePath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp config file: %v", err)
	}
	return filePath
}

func TestLoadConfig_ValidStrict(t *testing.T) {
	yamlContent := `
ProxyPort: 8080
GossipPort: 8081
Peers:
  - "127.0.0.1:9000"
BackendURL: "https://httpbin.org"

Protection:
  Mode: "Strict"
  RateLimit:
    Window: "1m"
    AuthRateLimit: 100
    IPRateLimit: 10
  Concurrency:
    MaxInFlight: 200
    MinInFlight: 10
    InitialLimit: 50
    Alpha: 3.0
    Beta: 6.0
    WindowSize: "10s"
`
	path := writeTempConfigFile(t, yamlContent)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("expected valid config, got error: %v", err)
	}

	if cfg.ProxyPort != 8080 || cfg.GossipPort != 8081 {
		t.Errorf("unexpected ports: ProxyPort=%d, GossipPort=%d", cfg.ProxyPort, cfg.GossipPort)
	}
	if cfg.Protection.Mode != "Strict" {
		t.Errorf("expected Mode Strict, got %s", cfg.Protection.Mode)
	}
	if cfg.Protection.RateLimit.AuthRateLimit != 100 {
		t.Errorf("expected AuthRateLimit 100, got %d", cfg.Protection.RateLimit.AuthRateLimit)
	}
	if cfg.Protection.Concurrency.InitialLimit != 50 {
		t.Errorf("expected InitialLimit 50, got %d", cfg.Protection.Concurrency.InitialLimit)
	}
}

func TestLoadConfig_LegacyFallback(t *testing.T) {
	yamlContent := `
ProxyPort: 8080
GossipPort: 8081
AuthRateLimit: 50
IPRateLimit: 5
`
	path := writeTempConfigFile(t, yamlContent)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("expected legacy config to be parsed and normalized, got error: %v", err)
	}

	if cfg.Protection.Mode != "RateLimitOnly" {
		t.Errorf("expected default Mode RateLimitOnly, got %s", cfg.Protection.Mode)
	}
	if cfg.Protection.RateLimit.AuthRateLimit != 50 {
		t.Errorf("expected normalized AuthRateLimit 50, got %d", cfg.Protection.RateLimit.AuthRateLimit)
	}
	if cfg.Protection.RateLimit.IPRateLimit != 5 {
		t.Errorf("expected normalized IPRateLimit 5, got %d", cfg.Protection.RateLimit.IPRateLimit)
	}
	if cfg.BackendURL != "https://httpbin.org" {
		t.Errorf("expected default BackendURL, got %s", cfg.BackendURL)
	}
}

func TestValidate_InvalidPorts(t *testing.T) {
	cfg := &Config{
		ProxyPort:  0,
		GossipPort: 8081,
	}
	if err := cfg.Validate(); err == nil {
		t.Errorf("expected error for ProxyPort 0")
	}

	cfg.ProxyPort = 8080
	cfg.GossipPort = 8080 // same ports
	if err := cfg.Validate(); err == nil {
		t.Errorf("expected error for matching ProxyPort and GossipPort")
	}
}

func TestValidate_InvalidBackendURL(t *testing.T) {
	cfg := &Config{
		ProxyPort:  8080,
		GossipPort: 8081,
		BackendURL: "not-a-valid-url-no-scheme",
		Protection: ProtectionConfig{Mode: "RateLimitOnly", RateLimit: RateLimitConfig{AuthRateLimit: 10, IPRateLimit: 5}},
	}
	if err := cfg.Validate(); err == nil {
		t.Errorf("expected error for invalid BackendURL without scheme")
	}
}

func TestValidate_InvalidMode(t *testing.T) {
	cfg := &Config{
		ProxyPort:  8080,
		GossipPort: 8081,
		Protection: ProtectionConfig{Mode: "InvalidMode"},
	}
	if err := cfg.Validate(); err == nil {
		t.Errorf("expected error for invalid Protection.Mode")
	}
}

func TestValidate_ConcurrencyBounds(t *testing.T) {
	cfg := &Config{
		ProxyPort:  8080,
		GossipPort: 8081,
		Protection: ProtectionConfig{
			Mode: "ConcurrencyOnly",
			Concurrency: ConcurrencyConfig{
				MinInFlight:  100,
				InitialLimit: 50, // MinInFlight > InitialLimit
				MaxInFlight:  200,
				Alpha:        3.0,
				Beta:         6.0,
				WindowSize:   "10s",
			},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Errorf("expected error when MinInFlight > InitialLimit")
	}

	// Test InitialLimit > MaxInFlight
	cfg.Protection.Concurrency.MinInFlight = 10
	cfg.Protection.Concurrency.InitialLimit = 250
	if err := cfg.Validate(); err == nil {
		t.Errorf("expected error when InitialLimit > MaxInFlight")
	}

	// Test Alpha >= Beta
	cfg.Protection.Concurrency.InitialLimit = 50
	cfg.Protection.Concurrency.Alpha = 6.0
	cfg.Protection.Concurrency.Beta = 3.0
	if err := cfg.Validate(); err == nil {
		t.Errorf("expected error when Alpha >= Beta")
	}
}

func TestValidate_InvalidDuration(t *testing.T) {
	cfg := &Config{
		ProxyPort:  8080,
		GossipPort: 8081,
		Protection: ProtectionConfig{
			Mode: "RateLimitOnly",
			RateLimit: RateLimitConfig{
				Window:        "not-a-duration",
				AuthRateLimit: 10,
				IPRateLimit:   5,
			},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Errorf("expected error for invalid Window duration")
	}
}
