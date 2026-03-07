package main

import (
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type Config struct {
	DiscordToken    string `toml:"discord_token"`
	DiscordClientID string `toml:"discord_client_id"`
	OAuthPort       int    `toml:"oauth_port"`
	XAIAPIKey       string `toml:"xai_api_key"`
	ContextMessages int    `toml:"context_messages"`
	Debug           bool   `toml:"debug"`
	PromptLogDir    string `toml:"prompt_log_dir"`
}

func LoadConfig(path string) (*Config, error) {
	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, err
	}
	if cfg.PromptLogDir != "" && !filepath.IsAbs(cfg.PromptLogDir) {
		cfg.PromptLogDir = filepath.Join(filepath.Dir(path), cfg.PromptLogDir)
	}
	if cfg.ContextMessages == 0 {
		cfg.ContextMessages = 20
	}
	if cfg.OAuthPort == 0 {
		cfg.OAuthPort = 8080
	}
	return &cfg, nil
}
