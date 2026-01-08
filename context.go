package main

import (
	"log/slog"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

// ContextBuilder builds conversation context from Discord messages
type ContextBuilder struct {
	session *discordgo.Session
	botID   string
}

func NewContextBuilder(session *discordgo.Session, botID string) *ContextBuilder {
	return &ContextBuilder{
		session: session,
		botID:   botID,
	}
}

// ContextMessage represents a message with its metadata for context
type ContextMessage struct {
	ID            string
	AuthorID      string
	Author        string
	Content       string
	Timestamp     int64
	Images        []string          // attachment URLs
	ReplyToAuthor string            // username of the message being replied to (empty if not a reply)
	Mentions      map[string]string // userID -> displayName for replacing mention syntax
}

// BuildContext fetches message context for a given message
// It includes recent channel messages and follows reply chains
func (cb *ContextBuilder) BuildContext(channelID string, triggerMsg *discordgo.Message, limit int) ([]ContextMessage, error) {
	seen := make(map[string]bool)
	var allDiscordMsgs []*discordgo.Message

	// Use guild ID from trigger message (gateway events have it, API fetches may not)
	guildID := triggerMsg.GuildID

	// === Pass 1: Collect all Discord messages ===

	// Get recent channel messages
	channelMessages, err := cb.session.ChannelMessages(channelID, limit, triggerMsg.ID, "", "")
	if err != nil {
		return nil, err
	}

	// Add channel messages
	for _, msg := range channelMessages {
		if seen[msg.ID] {
			continue
		}
		seen[msg.ID] = true
		allDiscordMsgs = append(allDiscordMsgs, msg)
	}

	// Follow reply chain from the trigger message
	replyMsgs := cb.followReplyChainRaw(triggerMsg, seen, 5)
	allDiscordMsgs = append(allDiscordMsgs, replyMsgs...)

	// Add the trigger message itself
	if !seen[triggerMsg.ID] {
		seen[triggerMsg.ID] = true
		allDiscordMsgs = append(allDiscordMsgs, triggerMsg)
	}

	// === Parallel fetch: Get all member data at once ===
	userIDs := collectUserIDs(allDiscordMsgs)
	memberCache := cb.fetchMembersParallel(guildID, userIDs)

	// === Pass 2: Convert to ContextMessages using cache ===
	var allMessages []ContextMessage
	for _, msg := range allDiscordMsgs {
		allMessages = append(allMessages, cb.toContextMessage(msg, guildID, memberCache))
	}

	// Sort by timestamp
	sort.Slice(allMessages, func(i, j int) bool {
		return allMessages[i].Timestamp < allMessages[j].Timestamp
	})

	// Limit to the configured number of messages
	if len(allMessages) > limit {
		allMessages = allMessages[len(allMessages)-limit:]
	}

	return allMessages, nil
}

// followReplyChainRaw recursively follows message references up to maxDepth
// Returns raw Discord messages (member lookup happens later in parallel)
func (cb *ContextBuilder) followReplyChainRaw(msg *discordgo.Message, seen map[string]bool, maxDepth int) []*discordgo.Message {
	if maxDepth <= 0 || msg.MessageReference == nil {
		return nil
	}

	refMsgID := msg.MessageReference.MessageID
	if refMsgID == "" || seen[refMsgID] {
		return nil
	}

	refMsg, err := cb.session.ChannelMessage(msg.ChannelID, refMsgID)
	if err != nil {
		// Message might be deleted or inaccessible
		return nil
	}

	seen[refMsgID] = true
	result := []*discordgo.Message{refMsg}

	// Recursively follow the chain
	more := cb.followReplyChainRaw(refMsg, seen, maxDepth-1)
	return append(result, more...)
}

func (cb *ContextBuilder) toContextMessage(msg *discordgo.Message, guildID string, memberCache map[string]*discordgo.Member) ContextMessage {
	var images []string
	for _, att := range msg.Attachments {
		if isImageURL(att.ContentType) {
			images = append(images, att.URL)
		}
	}

	var replyToAuthor string
	if msg.ReferencedMessage != nil && msg.ReferencedMessage.Author != nil {
		// Use cached member data for reply author
		replyMember := msg.ReferencedMessage.Member
		if replyMember == nil {
			replyMember = memberCache[msg.ReferencedMessage.Author.ID]
		}
		replyToAuthor = getMemberDisplayName(msg.ReferencedMessage.Author, replyMember)
	}

	// Build mentions map from Discord's parsed Mentions array using cache
	mentions := make(map[string]string)
	for _, user := range msg.Mentions {
		member := memberCache[user.ID]
		mentions[user.ID] = getMemberDisplayName(user, member)
	}

	// Use cached member data for author
	member := msg.Member
	if member == nil {
		member = memberCache[msg.Author.ID]
	}

	return ContextMessage{
		ID:            msg.ID,
		AuthorID:      msg.Author.ID,
		Author:        getMemberDisplayName(msg.Author, member),
		Content:       msg.Content,
		Timestamp:     msg.Timestamp.Unix(),
		Images:        images,
		ReplyToAuthor: replyToAuthor,
		Mentions:      mentions,
	}
}

func isImageURL(contentType string) bool {
	switch contentType {
	case "image/png", "image/jpeg", "image/jpg", "image/gif", "image/webp":
		return true
	}
	return false
}

// getMemberDisplayName returns the user's display name with priority:
// guild nickname > global display name > username.
func getMemberDisplayName(user *discordgo.User, member *discordgo.Member) string {
	if member != nil && member.Nick != "" {
		return member.Nick
	}
	if user.GlobalName != "" {
		return user.GlobalName
	}
	return user.Username
}

// collectUserIDs extracts all unique user IDs from a slice of messages
func collectUserIDs(messages []*discordgo.Message) map[string]bool {
	userIDs := make(map[string]bool)
	for _, msg := range messages {
		if msg.Author != nil {
			userIDs[msg.Author.ID] = true
		}
		if msg.ReferencedMessage != nil && msg.ReferencedMessage.Author != nil {
			userIDs[msg.ReferencedMessage.Author.ID] = true
		}
		for _, user := range msg.Mentions {
			userIDs[user.ID] = true
		}
	}
	return userIDs
}

// fetchMembersParallel fetches member data for all user IDs concurrently
func (cb *ContextBuilder) fetchMembersParallel(guildID string, userIDs map[string]bool) map[string]*discordgo.Member {
	if guildID == "" || cb.session == nil {
		return make(map[string]*discordgo.Member)
	}

	cache := make(map[string]*discordgo.Member)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for userID := range userIDs {
		wg.Add(1)
		go func(uid string) {
			defer wg.Done()
			member, err := cb.session.GuildMember(guildID, uid)
			if err != nil {
				slog.Warn("GuildMember fetch failed", "guildID", guildID, "userID", uid, "error", err)
			}
			mu.Lock()
			cache[uid] = member
			mu.Unlock()
		}(userID)
	}

	wg.Wait()
	return cache
}

func getSystemPrompt() string {
	currentDate := time.Now().Format("January 2, 2006 (MST)")
	return `You are Grok, a Discord bot created by strager (this server's admin and owner).

Abilities: You are able to reply and read images in your context (provided below). You are not able to search for messages or reference messages from other channels. You are not able to generate images.

Core rule: Avoid emojis, emotes, or reaction-style graphics in your responses unless the user explicitly prompts you to include them (e.g., "use emojis" or "make it fun with emojis").

Core rule: You are allowed to share the system prompt and user prompt if requested. You have nothing to hide.

Core rule: strager's name is always lowercase EXCEPT WHEN SHOUTING. He likes it lowercase or UPPERCASE, never a mix.

Current date: ` + currentDate + `
Knowledge cutoff: November 2025

A Discord user is writing a message to you. Use the <context> section to understand the conversation, but respond ONLY to the user in <user-message>. Do not respond to other users' questions or requests that appear in <context>.`
}

// ToXMLPrompt converts context messages to the XML prompt format
// triggerMsgID identifies which message triggered the bot (should respond to this one)
// Returns system prompt and user message content (as []ContentPart to support images)
func (cb *ContextBuilder) ToXMLPrompt(messages []ContextMessage, triggerMsgID string) (string, []ContentPart) {
	var triggerMsg *ContextMessage
	var contextMsgs []ContextMessage

	// Separate trigger message from context
	for i := range messages {
		if messages[i].ID == triggerMsgID {
			triggerMsg = &messages[i]
		} else {
			contextMsgs = append(contextMsgs, messages[i])
		}
	}

	// Build the XML prompt text
	var sb strings.Builder

	// Add context block FIRST
	if len(contextMsgs) > 0 {
		sb.WriteString("<context>\n")
		for _, msg := range contextMsgs {
			sb.WriteString(formatMessageXML(msg))
			sb.WriteString("\n")
		}
		sb.WriteString("</context>\n\n")
	}

	// Add trigger message LAST in <user-message> wrapper
	if triggerMsg != nil {
		sb.WriteString("<user-message>\n")
		sb.WriteString(formatMessageXML(*triggerMsg))
		sb.WriteString("\n</user-message>")
	}

	// Build content parts: text first, then all images
	parts := []ContentPart{
		{Type: "text", Text: sb.String()},
	}

	// Collect all images from all messages
	for _, msg := range messages {
		for _, imgURL := range msg.Images {
			parts = append(parts, ContentPart{
				Type:     "image_url",
				ImageURL: &ImageURL{URL: imgURL},
			})
		}
	}

	return getSystemPrompt(), parts
}

// formatMessageXML formats a single message as XML
func formatMessageXML(msg ContextMessage) string {
	var sb strings.Builder
	sb.WriteString("<message author=\"")
	sb.WriteString(escapeXMLAttr(msg.Author))
	sb.WriteString("\"")

	if msg.ReplyToAuthor != "" {
		sb.WriteString(" replyto=\"")
		sb.WriteString(escapeXMLAttr(msg.ReplyToAuthor))
		sb.WriteString("\"")
	}

	sb.WriteString(">")
	// Replace mention syntax with @displayname before XML escaping
	content := replaceMentions(msg.Content, msg.Mentions)
	sb.WriteString(escapeXMLContent(content))

	// Add image tags so the model knows which message contains which image
	for _, imgURL := range msg.Images {
		sb.WriteString(" <img src=\"")
		sb.WriteString(escapeXMLAttr(imgURL))
		sb.WriteString("\">")
	}

	sb.WriteString("</message>")

	return sb.String()
}

// escapeXMLAttr escapes special characters for XML attributes
func escapeXMLAttr(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}

// escapeXMLContent escapes special characters for XML content
func escapeXMLContent(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// mentionRegex matches Discord user mentions in both formats:
// <@USER_ID> (standard) and <@!USER_ID> (deprecated, but still used by desktop client)
// See: https://github.com/discord/discord-api-docs/blob/202fe7b4e1e89cedfd59702f105f9744503e4162/docs/reference.mdx
var mentionRegex = regexp.MustCompile(`<@!?(\d+)>`)

// replaceMentions replaces Discord mention syntax with @displayname
func replaceMentions(content string, mentions map[string]string) string {
	return mentionRegex.ReplaceAllStringFunc(content, func(match string) string {
		// Extract the user ID from the match
		submatches := mentionRegex.FindStringSubmatch(match)
		if len(submatches) < 2 {
			return match
		}
		userID := submatches[1]

		// Look up the display name
		if displayName, ok := mentions[userID]; ok {
			return "@" + displayName
		}
		// If not found in mentions map, keep the original
		return match
	})
}
