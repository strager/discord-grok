package main

import (
	"strings"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
)

func TestToXMLPrompt_SingleTriggerOnly(t *testing.T) {
	cb := &ContextBuilder{botID: "bot123"}
	messages := []ContextMessage{
		{ID: "msg1", Author: "strager", Content: "@grok hello"},
	}

	sysPrompt, parts := cb.ToXMLPrompt(messages, "msg1")

	if !strings.Contains(sysPrompt, "You are Grok") {
		t.Errorf("unexpected system prompt: %s", sysPrompt)
	}

	if len(parts) != 1 {
		t.Fatalf("expected 1 content part, got %d", len(parts))
	}

	text := parts[0].Text
	if !strings.Contains(text, `<message author="strager">@grok hello</message>`) {
		t.Errorf("trigger message not found in output: %s", text)
	}
	if !strings.Contains(text, "<user-message>") {
		t.Errorf("trigger message should be wrapped in <user-message>: %s", text)
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

	// Context should appear BEFORE trigger (trigger is last for recency bias)
	contextIdx := strings.Index(text, "<context>")
	userMsgIdx := strings.Index(text, "<user-message>")

	if contextIdx == -1 {
		t.Fatal("context block not found")
	}
	if userMsgIdx == -1 {
		t.Fatal("user-message block not found")
	}
	if contextIdx > userMsgIdx {
		t.Error("context block should appear before user-message block")
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

	// Verify key phrases are present rather than exact match
	requiredPhrases := []string{
		"You are Grok",
		"Discord bot",
		"A Discord user is writing a message to you",
		"Do not respond to other users",
	}
	for _, phrase := range requiredPhrases {
		if !strings.Contains(sysPrompt, phrase) {
			t.Errorf("system prompt missing required phrase %q.\ngot: %s", phrase, sysPrompt)
		}
	}
}

func TestReplaceMentions(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		mentions map[string]string
		want     string
	}{
		{
			name:     "standard mention format",
			content:  "hello <@123456789>!",
			mentions: map[string]string{"123456789": "Alice"},
			want:     "hello @Alice!",
		},
		{
			name:     "deprecated mention format with exclamation",
			content:  "hey <@!987654321> what's up",
			mentions: map[string]string{"987654321": "Bob"},
			want:     "hey @Bob what's up",
		},
		{
			name:     "multiple mentions",
			content:  "<@111> and <@!222> are here",
			mentions: map[string]string{"111": "Alice", "222": "Bob"},
			want:     "@Alice and @Bob are here",
		},
		{
			name:     "unknown mention stays unchanged",
			content:  "hello <@999999>",
			mentions: map[string]string{},
			want:     "hello <@999999>",
		},
		{
			name:     "no mentions",
			content:  "just a regular message",
			mentions: map[string]string{},
			want:     "just a regular message",
		},
		{
			name:     "mixed known and unknown mentions",
			content:  "<@111> talked to <@999>",
			mentions: map[string]string{"111": "Alice"},
			want:     "@Alice talked to <@999>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := replaceMentions(tt.content, tt.mentions)
			if got != tt.want {
				t.Errorf("replaceMentions() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestToXMLPrompt_WithMentions(t *testing.T) {
	cb := &ContextBuilder{botID: "bot123"}
	messages := []ContextMessage{
		{
			ID:       "msg1",
			Author:   "strager",
			Content:  "hey <@111222333> check this out",
			Mentions: map[string]string{"111222333": "CoolUser"},
		},
	}

	_, parts := cb.ToXMLPrompt(messages, "msg1")

	text := parts[0].Text
	if !strings.Contains(text, "@CoolUser") {
		t.Errorf("mention not replaced with display name: %s", text)
	}
	if strings.Contains(text, "<@111222333>") || strings.Contains(text, "&lt;@111222333&gt;") {
		t.Errorf("raw mention ID should not appear in output: %s", text)
	}
}

func TestGetMemberDisplayName(t *testing.T) {
	tests := []struct {
		name       string
		user       *discordgo.User
		member     *discordgo.Member
		wantResult string
	}{
		{
			name:       "prefers guild nick over global name",
			user:       &discordgo.User{Username: "alice123", GlobalName: "Alice Global"},
			member:     &discordgo.Member{Nick: "Alice Server Nick"},
			wantResult: "Alice Server Nick",
		},
		{
			name:       "falls back to GlobalName when nick is empty",
			user:       &discordgo.User{Username: "alice123", GlobalName: "Alice Global"},
			member:     &discordgo.Member{Nick: ""},
			wantResult: "Alice Global",
		},
		{
			name:       "falls back to GlobalName when member is nil",
			user:       &discordgo.User{Username: "alice123", GlobalName: "Alice Global"},
			member:     nil,
			wantResult: "Alice Global",
		},
		{
			name:       "falls back to Username when both nick and GlobalName are empty",
			user:       &discordgo.User{Username: "alice123", GlobalName: ""},
			member:     &discordgo.Member{Nick: ""},
			wantResult: "alice123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getMemberDisplayName(tt.user, tt.member)
			if got != tt.wantResult {
				t.Errorf("getMemberDisplayName() = %q, want %q", got, tt.wantResult)
			}
		})
	}
}

func TestToXMLPrompt_AuthorUsesDisplayName(t *testing.T) {
	cb := &ContextBuilder{botID: "bot123"}

	t.Run("author with display name", func(t *testing.T) {
		messages := []ContextMessage{
			{ID: "msg1", Author: "Cool Display Name", Content: "hello"},
		}
		_, parts := cb.ToXMLPrompt(messages, "msg1")
		text := parts[0].Text

		if !strings.Contains(text, `author="Cool Display Name"`) {
			t.Errorf("author attribute should use display name: %s", text)
		}
	})

	t.Run("author with username fallback", func(t *testing.T) {
		messages := []ContextMessage{
			{ID: "msg1", Author: "username123", Content: "hello"},
		}
		_, parts := cb.ToXMLPrompt(messages, "msg1")
		text := parts[0].Text

		if !strings.Contains(text, `author="username123"`) {
			t.Errorf("author attribute should use username as fallback: %s", text)
		}
	})
}

func TestToXMLPrompt_ReplyToUsesDisplayName(t *testing.T) {
	cb := &ContextBuilder{botID: "bot123"}

	t.Run("replyto with display name", func(t *testing.T) {
		messages := []ContextMessage{
			{ID: "msg1", Author: "alice", Content: "original message"},
			{ID: "msg2", Author: "bob", Content: "reply", ReplyToAuthor: "Alice Display Name"},
		}
		_, parts := cb.ToXMLPrompt(messages, "msg2")
		text := parts[0].Text

		if !strings.Contains(text, `replyto="Alice Display Name"`) {
			t.Errorf("replyto attribute should use display name: %s", text)
		}
	})

	t.Run("replyto with username fallback", func(t *testing.T) {
		messages := []ContextMessage{
			{ID: "msg1", Author: "alice", Content: "original message"},
			{ID: "msg2", Author: "bob", Content: "reply", ReplyToAuthor: "alice_username"},
		}
		_, parts := cb.ToXMLPrompt(messages, "msg2")
		text := parts[0].Text

		if !strings.Contains(text, `replyto="alice_username"`) {
			t.Errorf("replyto attribute should use username as fallback: %s", text)
		}
	})
}

func TestGetSystemPrompt_ContainsCurrentDate(t *testing.T) {
	// Note: This test does not handle the edge case of time changing during the test run.
	expectedDate := time.Now().Format("January 2, 2006 (MST)")
	sysPrompt := getSystemPrompt()

	if !strings.Contains(sysPrompt, "Current date:") {
		t.Error("system prompt should contain 'Current date:'")
	}

	if !strings.Contains(sysPrompt, expectedDate) {
		t.Errorf("system prompt should contain current date %q, got: %s", expectedDate, sysPrompt)
	}
}
