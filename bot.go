package main

import (
	"log/slog"

	"github.com/bwmarrin/discordgo"
)

type Bot struct {
	session        *discordgo.Session
	grokClient     *GrokClient
	rateLimiter    *RateLimiter
	contextBuilder *ContextBuilder
	contextLimit   int
}

func NewBot(token string, grokClient *GrokClient, rateLimiter *RateLimiter, contextLimit int) (*Bot, error) {
	session, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, err
	}

	bot := &Bot{
		session:      session,
		grokClient:   grokClient,
		rateLimiter:  rateLimiter,
		contextLimit: contextLimit,
	}

	session.AddHandler(bot.onMessageCreate)
	session.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsMessageContent

	return bot, nil
}

func (b *Bot) Start() error {
	if err := b.session.Open(); err != nil {
		return err
	}

	// Initialize context builder with bot ID
	b.contextBuilder = NewContextBuilder(b.session, b.session.State.User.ID)

	slog.Info("bot started", "user", b.session.State.User.Username)
	return nil
}

func (b *Bot) Stop() error {
	return b.session.Close()
}

func (b *Bot) onMessageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Ignore messages from the bot itself
	if m.Author.ID == s.State.User.ID {
		return
	}

	// Check if the bot is mentioned
	if !b.isMentioned(m.Message) {
		return
	}

	slog.Info("received mention",
		"author", m.Author.Username,
		"channel", m.ChannelID,
		"content", m.Content,
	)

	// Check rate limit
	if !b.rateLimiter.Allow(m.Author.ID) {
		slog.Warn("rate limited", "user", m.Author.Username)
		b.replyWithError(m, "You're sending messages too fast. Please wait a moment.")
		return
	}

	// Build context
	contextMessages, err := b.contextBuilder.BuildContext(m.ChannelID, m.Message, b.contextLimit)
	if err != nil {
		slog.Error("failed to build context", "error", err)
		b.replyWithError(m, "Sorry, I couldn't process that request. Please try again.")
		return
	}

	// Convert to Grok messages
	grokMessages := b.contextBuilder.ToGrokMessages(contextMessages)

	slog.Debug("calling Grok API", "contextMessages", len(contextMessages), "grokMessages", len(grokMessages))

	// Call Grok API
	response, err := b.grokClient.SendMessage(grokMessages)
	if err != nil {
		slog.Error("failed to call Grok API", "error", err)
		b.replyWithError(m, "Sorry, I couldn't process that request. Please try again.")
		return
	}

	// Reply to the message
	_, err = s.ChannelMessageSendReply(m.ChannelID, response, m.Reference())
	if err != nil {
		slog.Error("failed to send reply", "error", err)
	}
}

func (b *Bot) isMentioned(m *discordgo.Message) bool {
	for _, user := range m.Mentions {
		if user.ID == b.session.State.User.ID {
			return true
		}
	}
	return false
}

func (b *Bot) replyWithError(m *discordgo.MessageCreate, errMsg string) {
	_, err := b.session.ChannelMessageSendReply(m.ChannelID, errMsg, m.Reference())
	if err != nil {
		slog.Error("failed to send error reply", "error", err)
	}
}
