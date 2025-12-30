package main

import (
	"github.com/BurntSushi/toml"
)

type RateLimitConfig struct {
	RequestsPerMinute int `toml:"requests_per_minute"`
}

type Config struct {
	DiscordToken    string          `toml:"discord_token"`
	XAIAPIKey       string          `toml:"xai_api_key"`
	ContextMessages int             `toml:"context_messages"`
	RateLimit       RateLimitConfig `toml:"rate_limit"`
}

func LoadConfig(path string) (*Config, error) {
	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, err
	}
	if cfg.ContextMessages == 0 {
		cfg.ContextMessages = 20
	}
	if cfg.RateLimit.RequestsPerMinute == 0 {
		cfg.RateLimit.RequestsPerMinute = 10
	}
	return &cfg, nil
}
