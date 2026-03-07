package main

import (
	"log/slog"
	"time"

	"github.com/bwmarrin/discordgo"
)

const (
	maxResponseBytes = 2000
	bytesPerToken    = 4
	truncateSuffix   = "[Message truncated]"
)

type Bot struct {
	session         *discordgo.Session
	grokClient      *GrokClient
	rateLimiter     *RateLimiter
	contextBuilder  *ContextBuilder
	contextLimit    int
	promptLogWriter *PromptLogWriter
}

func NewBot(token string, grokClient *GrokClient, rateLimiter *RateLimiter, contextLimit int, promptLogWriter *PromptLogWriter) (*Bot, error) {
	session, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, err
	}

	bot := &Bot{
		session:         session,
		grokClient:      grokClient,
		rateLimiter:     rateLimiter,
		contextLimit:    contextLimit,
		promptLogWriter: promptLogWriter,
	}

	session.AddHandler(bot.onMessageCreate)
	session.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildMessages | discordgo.IntentsMessageContent

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
		err := s.MessageReactionAdd(m.ChannelID, m.ID, "🕐")
		if err != nil {
			slog.Error("failed to add rate limit reaction", "error", err)
		}
		return
	}

	// Start typing indicator that refreshes every 5 seconds
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				s.ChannelTyping(m.ChannelID)
			}
		}
	}()
	defer close(done)

	// Initial typing indicator
	s.ChannelTyping(m.ChannelID)

	// Build context
	contextMessages, err := b.contextBuilder.BuildContext(m.ChannelID, m.Message, b.contextLimit)
	if err != nil {
		slog.Error("failed to build context", "error", err)
		b.replyWithError(m, "Sorry, I couldn't process that request. Please try again.")
		return
	}

	// Convert to XML prompt format
	systemPrompt, userContent := b.contextBuilder.ToXMLPrompt(contextMessages, m.ID, time.Now())
	grokMessages := []ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userContent},
	}

	slog.Debug("calling Grok API", "contextMessages", len(contextMessages))

	// Call Grok API with token limit
	maxTokens := maxResponseBytes / bytesPerToken
	opts := SendOptions{MaxTokens: maxTokens}
	response, err := b.grokClient.SendMessage(grokMessages, opts)
	if err != nil {
		slog.Error("failed to call Grok API", "error", err)
		b.replyWithError(m, "Sorry, I couldn't process that request. Please try again.")
		return
	}

	// Retry once if response exceeds byte limit
	if len(response) > maxResponseBytes {
		slog.Debug("response exceeded byte limit, retrying", "bytes", len(response))
		response, err = b.grokClient.SendMessage(grokMessages, opts)
		if err != nil {
			slog.Error("failed to call Grok API on retry", "error", err)
			b.replyWithError(m, "Sorry, I couldn't process that request. Please try again.")
			return
		}
	}

	// Truncate if still over limit
	if len(response) > maxResponseBytes {
		slog.Debug("response still exceeded byte limit after retry, truncating", "bytes", len(response))
		response = truncateToByteLimit(response, maxResponseBytes, truncateSuffix)
	}

	// Log prompt and response
	if b.promptLogWriter != nil {
		entry := NewPromptLogEntry(time.Now(), m.ID, contextMessages, systemPrompt, userContent, response)
		if err := b.promptLogWriter.Write(entry); err != nil {
			slog.Error("failed to write prompt log", "error", err)
		}
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
	if len(m.MentionRoles) > 0 && m.GuildID != "" {
		member, err := b.session.State.Member(m.GuildID, b.session.State.User.ID)
		if err != nil {
			return false
		}
		for _, mentionedRole := range m.MentionRoles {
			for _, botRole := range member.Roles {
				if mentionedRole == botRole {
					return true
				}
			}
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

// truncateToByteLimit truncates a string to fit within the byte limit,
// ensuring we don't cut in the middle of a UTF-8 character.
// The suffix is appended if truncation occurs.
func truncateToByteLimit(s string, limit int, suffix string) string {
	if len(s) <= limit {
		return s
	}

	// Reserve space for suffix
	targetLen := limit - len(suffix)
	if targetLen <= 0 {
		return suffix[:limit]
	}

	// Find the last valid UTF-8 boundary within targetLen
	truncated := s[:targetLen]
	// Ensure we don't cut in the middle of a multi-byte UTF-8 character
	for len(truncated) > 0 && truncated[len(truncated)-1]&0xC0 == 0x80 {
		truncated = truncated[:len(truncated)-1]
	}
	// If we stopped at a start byte of a multi-byte sequence, remove it too
	if len(truncated) > 0 && truncated[len(truncated)-1]&0x80 != 0 {
		truncated = truncated[:len(truncated)-1]
	}

	return truncated + suffix
}
