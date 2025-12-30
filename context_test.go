package main

import (
	"strings"
	"testing"
)

func TestToXMLPrompt_SingleTriggerOnly(t *testing.T) {
	cb := &ContextBuilder{botID: "bot123"}
	messages := []ContextMessage{
		{ID: "msg1", Author: "strager", Content: "@grok hello"},
	}

	sysPrompt, parts := cb.ToXMLPrompt(messages, "msg1")

	if sysPrompt != systemPrompt {
		t.Errorf("unexpected system prompt: %s", sysPrompt)
	}

	if len(parts) != 1 {
		t.Fatalf("expected 1 content part, got %d", len(parts))
	}

	text := parts[0].Text
	if !strings.Contains(text, `<message author="strager">@grok hello</message>`) {
		t.Errorf("trigger message not found in output: %s", text)
	}
	if strings.Contains(text, "<context>") {
		t.Errorf("should not have context block with no context messages: %s", text)
	}
}

func TestToXMLPrompt_TriggerWithContext(t *testing.T) {
	cb := &ContextBuilder{botID: "bot123"}
	messages := []ContextMessage{
		{ID: "msg1", Author: "alice", Content: "hey everyone"},
		{ID: "msg2", Author: "bob", Content: "what's up"},
		{ID: "msg3", Author: "strager", Content: "@grok hello"},
	}

	_, parts := cb.ToXMLPrompt(messages, "msg3")

	text := parts[0].Text

	// Trigger should be before context block
	triggerIdx := strings.Index(text, `<message author="strager">`)
	contextIdx := strings.Index(text, "<context>")

	if triggerIdx == -1 {
		t.Fatal("trigger message not found")
	}
	if contextIdx == -1 {
		t.Fatal("context block not found")
	}
	if triggerIdx > contextIdx {
		t.Error("trigger message should appear before context block")
	}

	// Context should contain alice and bob
	if !strings.Contains(text, `<message author="alice">hey everyone</message>`) {
		t.Error("alice's message not in context")
	}
	if !strings.Contains(text, `<message author="bob">what's up</message>`) {
		t.Error("bob's message not in context")
	}
}

func TestToXMLPrompt_BotMessagesInContext(t *testing.T) {
	cb := &ContextBuilder{botID: "bot123"}
	messages := []ContextMessage{
		{ID: "msg1", Author: "alice", Content: "@grok what's 2+2"},
		{ID: "msg2", AuthorID: "bot123", Author: "grok", Content: "2+2 is 4"},
		{ID: "msg3", Author: "strager", Content: "@grok hello"},
	}

	_, parts := cb.ToXMLPrompt(messages, "msg3")

	text := parts[0].Text

	// Bot's previous message should be in context
	if !strings.Contains(text, `<message author="grok">2+2 is 4</message>`) {
		t.Errorf("bot message not found in context: %s", text)
	}
}

func TestToXMLPrompt_WithImages(t *testing.T) {
	cb := &ContextBuilder{botID: "bot123"}
	messages := []ContextMessage{
		{ID: "msg1", Author: "alice", Content: "check this out", Images: []string{"https://example.com/img1.png"}},
		{ID: "msg2", Author: "strager", Content: "@grok rate this", Images: []string{"https://example.com/img2.jpg"}},
	}

	_, parts := cb.ToXMLPrompt(messages, "msg2")

	// Should have text part + 2 image parts
	if len(parts) != 3 {
		t.Fatalf("expected 3 content parts (1 text + 2 images), got %d", len(parts))
	}

	// Verify text part has img tags
	text := parts[0].Text
	if !strings.Contains(text, `<img src="https://example.com/img1.png">`) {
		t.Error("img tag for alice's image not found")
	}
	if !strings.Contains(text, `<img src="https://example.com/img2.jpg">`) {
		t.Error("img tag for strager's image not found")
	}

	// Verify image_url parts
	if parts[1].Type != "image_url" || parts[1].ImageURL.URL != "https://example.com/img1.png" {
		t.Errorf("first image part wrong: %+v", parts[1])
	}
	if parts[2].Type != "image_url" || parts[2].ImageURL.URL != "https://example.com/img2.jpg" {
		t.Errorf("second image part wrong: %+v", parts[2])
	}
}

func TestToXMLPrompt_WithReplyTo(t *testing.T) {
	cb := &ContextBuilder{botID: "bot123"}
	messages := []ContextMessage{
		{ID: "msg1", Author: "alice", Content: "hello"},
		{ID: "msg2", Author: "strager", Content: "@grok what did alice say", ReplyToAuthor: "alice"},
	}

	_, parts := cb.ToXMLPrompt(messages, "msg2")

	text := parts[0].Text
	if !strings.Contains(text, `replyto="alice"`) {
		t.Errorf("replyto attribute not found: %s", text)
	}
}

func TestToXMLPrompt_SystemPrompt(t *testing.T) {
	cb := &ContextBuilder{botID: "bot123"}
	messages := []ContextMessage{
		{ID: "msg1", Author: "strager", Content: "@grok hi"},
	}

	sysPrompt, _ := cb.ToXMLPrompt(messages, "msg1")

	expected := "You are Grok, a Discord bot. A Discord user is writing a message to you. Please respond. Message context is provided below."
	if sysPrompt != expected {
		t.Errorf("system prompt mismatch.\ngot:  %s\nwant: %s", sysPrompt, expected)
	}
}
