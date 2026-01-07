package main

import (
	"regexp"
	"sort"
	"strings"

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
	var allMessages []ContextMessage

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
		allMessages = append(allMessages, cb.toContextMessage(msg))
	}

	// Follow reply chain from the trigger message
	replyMessages, err := cb.followReplyChain(triggerMsg, seen, 5)
	if err != nil {
		return nil, err
	}
	allMessages = append(allMessages, replyMessages...)

	// Add the trigger message itself
	if !seen[triggerMsg.ID] {
		seen[triggerMsg.ID] = true
		allMessages = append(allMessages, cb.toContextMessage(triggerMsg))
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

// followReplyChain recursively follows message references up to maxDepth
func (cb *ContextBuilder) followReplyChain(msg *discordgo.Message, seen map[string]bool, maxDepth int) ([]ContextMessage, error) {
	if maxDepth <= 0 || msg.MessageReference == nil {
		return nil, nil
	}

	refMsgID := msg.MessageReference.MessageID
	if refMsgID == "" || seen[refMsgID] {
		return nil, nil
	}

	refMsg, err := cb.session.ChannelMessage(msg.ChannelID, refMsgID)
	if err != nil {
		// Message might be deleted or inaccessible
		return nil, nil
	}

	seen[refMsgID] = true
	result := []ContextMessage{cb.toContextMessage(refMsg)}

	// Recursively follow the chain
	more, err := cb.followReplyChain(refMsg, seen, maxDepth-1)
	if err != nil {
		return result, nil
	}

	return append(result, more...), nil
}

func (cb *ContextBuilder) toContextMessage(msg *discordgo.Message) ContextMessage {
	var images []string
	for _, att := range msg.Attachments {
		if isImageURL(att.ContentType) {
			images = append(images, att.URL)
		}
	}

	var replyToAuthor string
	if msg.ReferencedMessage != nil && msg.ReferencedMessage.Author != nil {
		replyToAuthor = getDisplayName(msg.ReferencedMessage.Author)
	}

	// Build mentions map from Discord's parsed Mentions array
	mentions := make(map[string]string)
	for _, user := range msg.Mentions {
		mentions[user.ID] = getDisplayName(user)
	}

	return ContextMessage{
		ID:            msg.ID,
		AuthorID:      msg.Author.ID,
		Author:        getDisplayName(msg.Author),
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

// getDisplayName returns the user's display name (GlobalName) if set,
// otherwise falls back to their username.
func getDisplayName(user *discordgo.User) string {
	if user.GlobalName != "" {
		return user.GlobalName
	}
	return user.Username
}

const systemPrompt = `You are Grok, a Discord bot created by strager (this server's admin and owner).

Abilities: You are able to reply and read images in your context (provided below). You are not able to search for messages or reference messages from other channels. You are not able to generate images.

Core rule: Avoid emojis, emotes, or reaction-style graphics in your responses unless the user explicitly prompts you to include them (e.g., "use emojis" or "make it fun with emojis").

A Discord user is writing a message to you. Please respond. Message context is provided below.`

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

	// Add trigger message first
	if triggerMsg != nil {
		sb.WriteString(formatMessageXML(*triggerMsg))
		sb.WriteString("\n\n")
	}

	// Add context block
	if len(contextMsgs) > 0 {
		sb.WriteString("<context>\n")
		for _, msg := range contextMsgs {
			sb.WriteString(formatMessageXML(msg))
			sb.WriteString("\n")
		}
		sb.WriteString("</context>")
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

	return systemPrompt, parts
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
