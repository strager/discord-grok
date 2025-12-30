package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
)

const (
	discordAuthorizeURL = "https://discord.com/oauth2/authorize"
	botPermissions      = 68608 // View Channels + Send Messages + Read Message History
)

func RunOAuthServer(clientID string, port int) {
	mux := http.NewServeMux()

	redirectURI := fmt.Sprintf("http://localhost:%d/callback", port)

	mux.HandleFunc("/invite", func(w http.ResponseWriter, r *http.Request) {
		params := url.Values{}
		params.Set("client_id", clientID)
		params.Set("scope", "bot")
		params.Set("permissions", fmt.Sprintf("%d", botPermissions))
		params.Set("redirect_uri", redirectURI)

		authURL := discordAuthorizeURL + "?" + params.Encode()
		http.Redirect(w, r, authURL, http.StatusFound)
	})

	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		guildID := r.URL.Query().Get("guild_id")
		slog.Info("bot added to guild", "guild_id", guildID)

		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head><title>Success</title></head>
<body>
<h1>Bot added successfully!</h1>
<p>Guild ID: %s</p>
<p>You can close this window.</p>
</body>
</html>`, guildID)
	})

	addr := fmt.Sprintf(":%d", port)
	slog.Info("oauth server listening", "addr", addr)
	http.ListenAndServe(addr, mux)
}
