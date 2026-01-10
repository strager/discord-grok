package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	configPath := flag.String("config", "config.toml", "path to config file")
	flag.Parse()

	// Load configuration
	cfg, err := LoadConfig(*configPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// Configure log level
	if cfg.Debug {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})))
	}

	// Start OAuth server in background if client ID is configured
	if cfg.DiscordClientID != "" {
		go RunOAuthServer(cfg.DiscordClientID, cfg.OAuthPort)
		slog.Info("invite URL", "url", fmt.Sprintf("http://localhost:%d/invite", cfg.OAuthPort))
	}

	// Initialize components
	grokClient := NewGrokClient(cfg.XAIAPIKey)
	rateLimiter := NewRateLimiter()

	// Create and start bot
	bot, err := NewBot(cfg.DiscordToken, grokClient, rateLimiter, cfg.ContextMessages)
	if err != nil {
		slog.Error("failed to create bot", "error", err)
		os.Exit(1)
	}

	if err := bot.Start(); err != nil {
		slog.Error("failed to start bot", "error", err)
		os.Exit(1)
	}

	slog.Info("bot is running, press Ctrl+C to stop")

	// Wait for interrupt signal
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	slog.Info("shutting down...")
	if err := bot.Stop(); err != nil {
		slog.Error("error stopping bot", "error", err)
	}
}
