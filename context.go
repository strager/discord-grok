package main

import (
	"sort"

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
	ID        string
	AuthorID  string
	Author    string
	Content   string
	Timestamp int64
	Images    []string // attachment URLs
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

	return ContextMessage{
		ID:        msg.ID,
		AuthorID:  msg.Author.ID,
		Author:    msg.Author.Username,
		Content:   msg.Content,
		Timestamp: msg.Timestamp.Unix(),
		Images:    images,
	}
}

func isImageURL(contentType string) bool {
	switch contentType {
	case "image/png", "image/jpeg", "image/jpg", "image/gif", "image/webp":
		return true
	}
	return false
}

// ToGrokMessages converts context messages to Grok API format
func (cb *ContextBuilder) ToGrokMessages(messages []ContextMessage) []ChatMessage {
	var result []ChatMessage

	for _, msg := range messages {
		role := "user"
		if msg.AuthorID == cb.botID {
			role = "assistant"
		}

		// Build content - either simple string or multi-part with images
		if len(msg.Images) == 0 {
			result = append(result, ChatMessage{
				Role:    role,
				Content: msg.Author + ": " + msg.Content,
			})
		} else {
			// Multi-part content with images
			parts := []ContentPart{
				{Type: "text", Text: msg.Author + ": " + msg.Content},
			}
			for _, imgURL := range msg.Images {
				parts = append(parts, ContentPart{
					Type:     "image_url",
					ImageURL: &ImageURL{URL: imgURL},
				})
			}
			result = append(result, ChatMessage{
				Role:    role,
				Content: parts,
			})
		}
	}

	return result
}
