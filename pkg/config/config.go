package config

import (
	"fmt"
	"log"
	"os"

	"go.yaml.in/yaml/v4"
)

type Config struct {
	ProxyPort 	int 		`yaml:"ProxyPort"`
	GossipPort 	int 		`yaml:"GossipPort"`
	Peers 		[]string 	`yaml:"Peers"`
	RateLimit	int 		`yaml:"RateLimit"`
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
	
	log.Printf("Config loaded: ProxyPort: %d, GossipPort: %d, Peers: %v, RateLimit: %d\n", config.ProxyPort, config.GossipPort, config.Peers, config.RateLimit)
	return &config, nil
}


