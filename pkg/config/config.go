package config

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"time"

	"go.yaml.in/yaml/v4"
)

type Config struct {
	ProxyPort   int              `yaml:"ProxyPort"`
	GossipPort  int              `yaml:"GossipPort"`
	Peers       []string         `yaml:"Peers"`
	BackendURL  string           `yaml:"BackendURL"`
	Protection  ProtectionConfig `yaml:"Protection"`

	// Backward-compatible fallback fields
	AuthRateLimit int `yaml:"AuthRateLimit,omitempty"`
	IPRateLimit   int `yaml:"IPRateLimit,omitempty"`
}

type ProtectionConfig struct {
	// Mode options: "RateLimitOnly" | "ConcurrencyOnly" | "Strict"
	Mode        string            `yaml:"Mode"`
	RateLimit   RateLimitConfig   `yaml:"RateLimit"`
	Concurrency ConcurrencyConfig `yaml:"Concurrency"`
}

type RateLimitConfig struct {
	Window        string `yaml:"Window"` // e.g., "1m"
	AuthRateLimit int    `yaml:"AuthRateLimit"`
	IPRateLimit   int    `yaml:"IPRateLimit"`
}

type ConcurrencyConfig struct {
	MaxInFlight  int     `yaml:"MaxInFlight"`
	MinInFlight  int     `yaml:"MinInFlight"`
	InitialLimit int     `yaml:"InitialLimit"`
	Alpha        float64 `yaml:"Alpha"`
	Beta         float64 `yaml:"Beta"`
	WindowSize   string  `yaml:"WindowSize"` // e.g., "10s"
}

// Validate checks configuration values and applies defaults.
func (c *Config) Validate() error {
	if c.ProxyPort <= 0 || c.ProxyPort > 65535 {
		return fmt.Errorf("invalid ProxyPort: %d (must be between 1 and 65535)", c.ProxyPort)
	}
	if c.GossipPort <= 0 || c.GossipPort > 65535 {
		return fmt.Errorf("invalid GossipPort: %d (must be between 1 and 65535)", c.GossipPort)
	}
	if c.ProxyPort == c.GossipPort {
		return fmt.Errorf("ProxyPort and GossipPort cannot be the same (%d)", c.ProxyPort)
	}

	if c.BackendURL == "" {
		c.BackendURL = "https://httpbin.org"
	}
	parsedURL, err := url.Parse(c.BackendURL)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		return fmt.Errorf("invalid BackendURL %q: must be an absolute URL with scheme and host", c.BackendURL)
	}

	// Default mode to RateLimitOnly if not specified
	if c.Protection.Mode == "" {
		c.Protection.Mode = "RateLimitOnly"
	}

	switch c.Protection.Mode {
	case "RateLimitOnly", "ConcurrencyOnly", "Strict":
		// valid
	default:
		return fmt.Errorf("invalid Protection.Mode %q: must be 'RateLimitOnly', 'ConcurrencyOnly', or 'Strict'", c.Protection.Mode)
	}

	// Validate RateLimit configuration
	if c.Protection.Mode == "RateLimitOnly" || c.Protection.Mode == "Strict" {
		if c.Protection.RateLimit.AuthRateLimit == 0 && c.AuthRateLimit > 0 {
			c.Protection.RateLimit.AuthRateLimit = c.AuthRateLimit
		}
		if c.Protection.RateLimit.IPRateLimit == 0 && c.IPRateLimit > 0 {
			c.Protection.RateLimit.IPRateLimit = c.IPRateLimit
		}
		if c.Protection.RateLimit.Window == "" {
			c.Protection.RateLimit.Window = "1m"
		}

		if _, err := time.ParseDuration(c.Protection.RateLimit.Window); err != nil {
			return fmt.Errorf("invalid Protection.RateLimit.Window %q: %w", c.Protection.RateLimit.Window, err)
		}
		if c.Protection.RateLimit.AuthRateLimit <= 0 {
			return fmt.Errorf("Protection.RateLimit.AuthRateLimit must be > 0 (got %d)", c.Protection.RateLimit.AuthRateLimit)
		}
		if c.Protection.RateLimit.IPRateLimit <= 0 {
			return fmt.Errorf("Protection.RateLimit.IPRateLimit must be > 0 (got %d)", c.Protection.RateLimit.IPRateLimit)
		}
	}

	// Validate Concurrency configuration
	if c.Protection.Mode == "ConcurrencyOnly" || c.Protection.Mode == "Strict" {
		cc := &c.Protection.Concurrency
		if cc.MinInFlight <= 0 {
			cc.MinInFlight = 10
		}
		if cc.InitialLimit <= 0 {
			cc.InitialLimit = 50
		}
		if cc.MaxInFlight <= 0 {
			cc.MaxInFlight = 200
		}
		if cc.Alpha <= 0 {
			cc.Alpha = 3.0
		}
		if cc.Beta <= 0 {
			cc.Beta = 6.0
		}
		if cc.WindowSize == "" {
			cc.WindowSize = "10s"
		}

		if cc.MinInFlight > cc.InitialLimit {
			return fmt.Errorf("Concurrency.MinInFlight (%d) cannot exceed InitialLimit (%d)", cc.MinInFlight, cc.InitialLimit)
		}
		if cc.InitialLimit > cc.MaxInFlight {
			return fmt.Errorf("Concurrency.InitialLimit (%d) cannot exceed MaxInFlight (%d)", cc.InitialLimit, cc.MaxInFlight)
		}
		if cc.Alpha >= cc.Beta {
			return fmt.Errorf("Concurrency.Alpha (%.2f) must be strictly less than Beta (%.2f)", cc.Alpha, cc.Beta)
		}
		if _, err := time.ParseDuration(cc.WindowSize); err != nil {
			return fmt.Errorf("invalid Concurrency.WindowSize %q: %w", cc.WindowSize, err)
		}
	}

	return nil
}

// LoadConfig reads, unmarshals, and validates the configuration file.
func LoadConfig(path string) (*Config, error) {
	yamlFile, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	err = yaml.Unmarshal(yamlFile, &config)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal config file: %w", err)
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	log.Printf("Config loaded: ProxyPort: %d, GossipPort: %d, Backend: %s, Mode: %s\n",
		config.ProxyPort, config.GossipPort, config.BackendURL, config.Protection.Mode)
	return &config, nil
}
