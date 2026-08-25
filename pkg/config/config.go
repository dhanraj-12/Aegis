package config

import (
	"fmt"
	"log"
	"os"

	"go.yaml.in/yaml/v4"
)

type Config struct {
	ProxyPort 		int 		`yaml:"ProxyPort"`
	GossipPort 		int 		`yaml:"GossipPort"`
	Peers 			[]string 	`yaml:"Peers"`
	AuthRateLimit	int 		`yaml:"AuthRateLimit"`
	IPRateLimit		int			`yaml:"IPRateLimit"`
}

func LoadConfig(path string) (*Config , error) {
	yamlFile, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	err = yaml.Unmarshal(yamlFile, &config)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal config file: %w", err)
	}
	
	log.Printf("Config loaded: ProxyPort: %d, GossipPort: %d, Peers: %v, AuthRateLimite: %d, IPRateLimite: %d\n", config.ProxyPort, config.GossipPort, config.Peers, config.AuthRateLimit, config.IPRateLimit)
	return &config, nil
}


