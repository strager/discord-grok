# discord-grok

Discord bot that responds to @mentions using xAI's Grok API.

## Setup

1. Copy `config.toml.example` to `config.toml`
2. Add your Discord bot token and client ID from [Discord Developer Portal](https://discord.com/developers/applications)
3. Add your xAI API key from [x.ai](https://console.x.ai)
4. Enable **MESSAGE CONTENT INTENT** in Discord Developer Portal > Bot > Privileged Gateway Intents

## Run

```
go build && ./discord-grok
```

Visit the invite URL printed at startup to add the bot to your server.
