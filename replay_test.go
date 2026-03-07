package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPromptLogRoundTrip(t *testing.T) {
	dir := t.TempDir()
	writer, err := NewPromptLogWriter(dir)
	if err != nil {
		t.Fatalf("NewPromptLogWriter: %v", err)
	}

	now := time.Date(2026, 3, 7, 13, 45, 2, 0, time.UTC)
	entry := NewPromptLogEntry(
		now,
		"trigger123",
		[]ContextMessage{
			{
				ID:            "msg1",
				AuthorID:      "user1",
				Author:        "alice",
				Content:       "hello everyone",
				Timestamp:     now.Unix() - 60,
				Images:        []string{"https://example.com/img.png"},
				ReplyToAuthor: "",
				Mentions:      map[string]string{"user2": "Bob"},
			},
			{
				ID:        "trigger123",
				AuthorID:  "user2",
				Author:    "Bob",
				Content:   "@grok what's up",
				Timestamp: now.Unix(),
				Mentions:  map[string]string{},
			},
		},
		"You are Grok, a Discord bot",
		[]ContentPart{
			{Type: "text", Text: "<context>\n<message author=\"alice\">hello everyone</message>\n</context>\n\n<user-message>\n<message author=\"Bob\">@grok what's up</message>\n</user-message>"},
			{Type: "image_url", ImageURL: &ImageURL{URL: "https://example.com/img.png"}},
		},
		"Hello! How can I help?",
	)

	if err := writer.Write(entry); err != nil {
		t.Fatalf("Write: %v", err)
	}

	// Find the written file
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		t.Fatalf("Glob: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}

	loaded, err := LoadPromptLog(files[0])
	if err != nil {
		t.Fatalf("LoadPromptLog: %v", err)
	}

	if loaded.Version != 1 {
		t.Errorf("version = %d, want 1", loaded.Version)
	}
	if loaded.TriggerMsgID != "trigger123" {
		t.Errorf("trigger_message_id = %q, want %q", loaded.TriggerMsgID, "trigger123")
	}
	if loaded.Response != "Hello! How can I help?" {
		t.Errorf("response = %q, want %q", loaded.Response, "Hello! How can I help?")
	}
	if len(loaded.ContextMessages) != 2 {
		t.Fatalf("context_messages length = %d, want 2", len(loaded.ContextMessages))
	}
	if loaded.ContextMessages[0].Author != "alice" {
		t.Errorf("context_messages[0].author = %q, want %q", loaded.ContextMessages[0].Author, "alice")
	}
	if len(loaded.ContextMessages[0].Images) != 1 || loaded.ContextMessages[0].Images[0] != "https://example.com/img.png" {
		t.Errorf("context_messages[0].images = %v, want [https://example.com/img.png]", loaded.ContextMessages[0].Images)
	}
	if loaded.Prompt.System != "You are Grok, a Discord bot" {
		t.Errorf("prompt.system = %q, want %q", loaded.Prompt.System, "You are Grok, a Discord bot")
	}
	if len(loaded.Prompt.UserContent) != 2 {
		t.Fatalf("prompt.user_content length = %d, want 2", len(loaded.Prompt.UserContent))
	}
	if loaded.Prompt.UserContent[1].ImageURL == nil || loaded.Prompt.UserContent[1].ImageURL.URL != "https://example.com/img.png" {
		t.Errorf("prompt.user_content[1].image_url = %v, want https://example.com/img.png", loaded.Prompt.UserContent[1].ImageURL)
	}
}

func TestPromptLogIsPrettyPrinted(t *testing.T) {
	dir := t.TempDir()
	writer, err := NewPromptLogWriter(dir)
	if err != nil {
		t.Fatalf("NewPromptLogWriter: %v", err)
	}

	now := time.Date(2026, 3, 7, 13, 0, 0, 0, time.UTC)
	entry := NewPromptLogEntry(now, "msg1", []ContextMessage{
		{ID: "msg1", Author: "alice", Content: "hi"},
	}, "system prompt", []ContentPart{{Type: "text", Text: "hello"}}, "response")

	if err := writer.Write(entry); err != nil {
		t.Fatalf("Write: %v", err)
	}

	files, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	data, _ := os.ReadFile(files[0])
	content := string(data)

	// Pretty-printed JSON should contain newlines and indentation
	if !strings.Contains(content, "\n") {
		t.Error("JSON should be pretty-printed with newlines")
	}
	if !strings.Contains(content, "  ") {
		t.Error("JSON should be pretty-printed with indentation")
	}
}

func TestPromptReconstruction_Identical(t *testing.T) {
	now := time.Date(2026, 3, 7, 13, 45, 0, 0, time.UTC)

	contextMessages := []ContextMessage{
		{ID: "msg1", Author: "alice", Content: "hey everyone"},
		{ID: "msg2", Author: "bob", Content: "what's up"},
		{ID: "trigger1", Author: "strager", Content: "@grok hello"},
	}

	// Generate the original prompt
	cb := &ContextBuilder{}
	origSystem, origContent := cb.ToXMLPrompt(contextMessages, "trigger1", now)

	// Simulate writing and reading back
	entry := NewPromptLogEntry(now, "trigger1", contextMessages, origSystem, origContent, "some response")

	// Reconstruct from structured data
	logTime := parseLogTime(entry)
	newSystem, newContent := cb.ToXMLPrompt(entry.ContextMessages, entry.TriggerMsgID, logTime)

	if origSystem != newSystem {
		t.Errorf("system prompts differ:\noriginal:      %s\nreconstructed: %s", origSystem, newSystem)
	}
	if origContent[0].Text != newContent[0].Text {
		t.Errorf("user content text differs:\noriginal:      %s\nreconstructed: %s", origContent[0].Text, newContent[0].Text)
	}
}

func TestPromptReconstruction_TriggerOnly(t *testing.T) {
	now := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)

	contextMessages := []ContextMessage{
		{ID: "msg1", Author: "strager", Content: "@grok hi"},
	}

	cb := &ContextBuilder{}
	origSystem, origContent := cb.ToXMLPrompt(contextMessages, "msg1", now)

	entry := NewPromptLogEntry(now, "msg1", contextMessages, origSystem, origContent, "response")
	logTime := parseLogTime(entry)
	newSystem, newContent := cb.ToXMLPrompt(entry.ContextMessages, entry.TriggerMsgID, logTime)

	if origSystem != newSystem {
		t.Error("system prompts should be identical for trigger-only case")
	}
	if origContent[0].Text != newContent[0].Text {
		t.Error("user content should be identical for trigger-only case")
	}
}

func TestPromptReconstruction_WithImages(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	contextMessages := []ContextMessage{
		{ID: "msg1", Author: "alice", Content: "look at this", Images: []string{"https://example.com/a.png"}},
		{ID: "msg2", Author: "strager", Content: "@grok rate this", Images: []string{"https://example.com/b.jpg"}},
	}

	cb := &ContextBuilder{}
	origSystem, origContent := cb.ToXMLPrompt(contextMessages, "msg2", now)

	entry := NewPromptLogEntry(now, "msg2", contextMessages, origSystem, origContent, "response")
	logTime := parseLogTime(entry)
	newSystem, newContent := cb.ToXMLPrompt(entry.ContextMessages, entry.TriggerMsgID, logTime)

	if origSystem != newSystem {
		t.Error("system prompts should be identical")
	}
	if len(origContent) != len(newContent) {
		t.Fatalf("content parts count: original=%d, reconstructed=%d", len(origContent), len(newContent))
	}
	for i := range origContent {
		if origContent[i].Type != newContent[i].Type {
			t.Errorf("part[%d] type: original=%q, reconstructed=%q", i, origContent[i].Type, newContent[i].Type)
		}
		if origContent[i].Text != newContent[i].Text {
			t.Errorf("part[%d] text differs", i)
		}
		if origContent[i].ImageURL != nil && newContent[i].ImageURL != nil {
			if origContent[i].ImageURL.URL != newContent[i].ImageURL.URL {
				t.Errorf("part[%d] image URL differs", i)
			}
		}
	}
}

func TestPromptReconstruction_WithMentions(t *testing.T) {
	now := time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)

	contextMessages := []ContextMessage{
		{
			ID:       "msg1",
			Author:   "strager",
			Content:  "hey <@111> check this",
			Mentions: map[string]string{"111": "CoolUser"},
		},
	}

	cb := &ContextBuilder{}
	origSystem, origContent := cb.ToXMLPrompt(contextMessages, "msg1", now)

	entry := NewPromptLogEntry(now, "msg1", contextMessages, origSystem, origContent, "response")
	logTime := parseLogTime(entry)
	newSystem, newContent := cb.ToXMLPrompt(entry.ContextMessages, entry.TriggerMsgID, logTime)

	if origContent[0].Text != newContent[0].Text {
		t.Errorf("user content differs:\noriginal:      %s\nreconstructed: %s", origContent[0].Text, newContent[0].Text)
	}
	if !strings.Contains(newContent[0].Text, "@CoolUser") {
		t.Error("reconstructed content should contain resolved mention")
	}
	if origSystem != newSystem {
		t.Error("system prompts should be identical")
	}
}

func TestPromptReconstruction_WithReplyTo(t *testing.T) {
	now := time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)

	contextMessages := []ContextMessage{
		{ID: "msg1", Author: "alice", Content: "hello"},
		{ID: "msg2", Author: "bob", Content: "@grok help", ReplyToAuthor: "alice"},
	}

	cb := &ContextBuilder{}
	origSystem, origContent := cb.ToXMLPrompt(contextMessages, "msg2", now)

	entry := NewPromptLogEntry(now, "msg2", contextMessages, origSystem, origContent, "response")
	logTime := parseLogTime(entry)
	_, newContent := cb.ToXMLPrompt(entry.ContextMessages, entry.TriggerMsgID, logTime)

	if origContent[0].Text != newContent[0].Text {
		t.Errorf("user content differs:\noriginal:      %s\nreconstructed: %s", origContent[0].Text, newContent[0].Text)
	}
	if !strings.Contains(newContent[0].Text, `replyto="alice"`) {
		t.Error("reconstructed content should contain replyto attribute")
	}
}

func TestPromptReconstruction_DetectsDiff(t *testing.T) {
	now := time.Date(2026, 3, 7, 13, 0, 0, 0, time.UTC)

	contextMessages := []ContextMessage{
		{ID: "msg1", Author: "strager", Content: "@grok hello"},
	}

	// Generate prompt with the current code
	cb := &ContextBuilder{}
	origSystem, origContent := cb.ToXMLPrompt(contextMessages, "msg1", now)

	// Simulate a log where the stored prompt was different (e.g., old code produced different output)
	entry := NewPromptLogEntry(now, "msg1", contextMessages, origSystem, origContent, "old response")
	// Tamper with the stored prompt to simulate a code change
	entry.Prompt.System = strings.Replace(entry.Prompt.System, "You are Grok", "You are GrokBot", 1)

	// Reconstruct from structured data with current code
	logTime := parseLogTime(entry)
	newSystem, _ := cb.ToXMLPrompt(entry.ContextMessages, entry.TriggerMsgID, logTime)

	// The reconstructed prompt should differ from the tampered stored prompt
	if newSystem == entry.Prompt.System {
		t.Error("expected system prompts to differ after tampering")
	}
	if !strings.Contains(newSystem, "You are Grok") {
		t.Error("reconstructed prompt should contain original text")
	}
	if strings.Contains(newSystem, "You are GrokBot") {
		t.Error("reconstructed prompt should not contain tampered text")
	}
}

func parseLogTime(entry PromptLog) time.Time {
	t, _ := time.Parse(time.RFC3339, entry.Timestamp)
	loc := time.FixedZone(entry.Timezone, entry.TimezoneOffset)
	return t.In(loc)
}

func TestPromptReconstruction_PreservesTimezone(t *testing.T) {
	est := time.FixedZone("EST", -5*60*60)
	now := time.Date(2026, 3, 7, 13, 45, 0, 0, est)

	contextMessages := []ContextMessage{
		{ID: "msg1", Author: "strager", Content: "@grok hello"},
	}

	cb := &ContextBuilder{}
	origSystem, origContent := cb.ToXMLPrompt(contextMessages, "msg1", now)

	entry := NewPromptLogEntry(now, "msg1", contextMessages, origSystem, origContent, "response")
	logTime := parseLogTime(entry)
	newSystem, newContent := cb.ToXMLPrompt(entry.ContextMessages, entry.TriggerMsgID, logTime)

	if origSystem != newSystem {
		t.Errorf("system prompts differ:\noriginal:      %s\nreconstructed: %s", origSystem, newSystem)
	}
	if !strings.Contains(newSystem, "(EST)") {
		t.Errorf("reconstructed system prompt should contain (EST), got: %s", newSystem)
	}
	if origContent[0].Text != newContent[0].Text {
		t.Errorf("user content differs")
	}
}

func TestContextMessageJSONTags(t *testing.T) {
	msg := ContextMessage{
		ID:            "123",
		AuthorID:      "456",
		Author:        "alice",
		Content:       "hello",
		Timestamp:     1000,
		Images:        []string{"https://example.com/img.png"},
		ReplyToAuthor: "bob",
		Mentions:      map[string]string{"789": "charlie"},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	jsonStr := string(data)
	// Verify snake_case JSON keys
	for _, key := range []string{`"id"`, `"author_id"`, `"author"`, `"content"`, `"timestamp"`, `"images"`, `"reply_to_author"`, `"mentions"`} {
		if !strings.Contains(jsonStr, key) {
			t.Errorf("JSON should contain key %s, got: %s", key, jsonStr)
		}
	}
	// Verify Go-style keys are NOT present
	for _, key := range []string{`"ID"`, `"AuthorID"`, `"ReplyToAuthor"`} {
		if strings.Contains(jsonStr, key) {
			t.Errorf("JSON should not contain Go-style key %s, got: %s", key, jsonStr)
		}
	}
}
