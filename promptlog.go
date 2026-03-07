package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type PromptLog struct {
	Version         int              `json:"version"`
	Timestamp       string           `json:"timestamp"`
	Timezone        string           `json:"timezone"`
	TimezoneOffset  int              `json:"timezone_offset"`
	TriggerMsgID    string           `json:"trigger_message_id"`
	ContextMessages []ContextMessage `json:"context_messages"`
	Prompt          PromptData       `json:"prompt"`
	Response        string           `json:"response"`
	Model           string           `json:"model"`
}

type PromptData struct {
	System      string        `json:"system"`
	UserContent []ContentPart `json:"user_content"`
}

type PromptLogWriter struct {
	logDir string
}

func NewPromptLogWriter(logDir string) (*PromptLogWriter, error) {
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create prompt log directory: %w", err)
	}
	return &PromptLogWriter{logDir: logDir}, nil
}

func (w *PromptLogWriter) Write(entry PromptLog) error {
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal prompt log: %w", err)
	}

	ts := strings.ReplaceAll(entry.Timestamp, ":", "-")
	filename := fmt.Sprintf("%s_%s.json", ts, entry.TriggerMsgID)
	finalPath := filepath.Join(w.logDir, filename)

	// Write atomically: temp file + rename
	tmpFile, err := os.CreateTemp(w.logDir, "promptlog-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	if err := os.Rename(tmpPath, finalPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

func LoadPromptLog(path string) (*PromptLog, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read prompt log: %w", err)
	}
	var log PromptLog
	if err := json.Unmarshal(data, &log); err != nil {
		return nil, fmt.Errorf("failed to parse prompt log: %w", err)
	}
	return &log, nil
}

func NewPromptLogEntry(now time.Time, triggerMsgID string, contextMessages []ContextMessage, systemPrompt string, userContent []ContentPart, response string) PromptLog {
	tzName, tzOffset := now.Zone()
	return PromptLog{
		Version:         1,
		Timestamp:       now.Format(time.RFC3339),
		Timezone:        tzName,
		TimezoneOffset:  tzOffset,
		TriggerMsgID:    triggerMsgID,
		ContextMessages: contextMessages,
		Prompt: PromptData{
			System:      systemPrompt,
			UserContent: userContent,
		},
		Response: response,
		Model:    xaiModel,
	}
}
