package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
)

const (
	xaiAPIURL = "https://api.x.ai/v1/chat/completions"
	xaiModel  = "grok-4-1-fast-reasoning"
)

type GrokClient struct {
	apiKey     string
	httpClient *http.Client
}

func NewGrokClient(apiKey string) *GrokClient {
	return &GrokClient{
		apiKey:     apiKey,
		httpClient: &http.Client{},
	}
}

// ChatMessage represents a message in the conversation
type ChatMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

// ImageURL represents an image URL for vision models
type ImageURL struct {
	URL string `json:"url"`
}

// ContentPart represents a part of a multi-modal message
type ContentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
}

type chatRequest struct {
	Model     string        `json:"model"`
	Messages  []ChatMessage `json:"messages"`
	MaxTokens int           `json:"max_tokens,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *string `json:"error,omitempty"`
}

// SendOptions configures the Grok API request
type SendOptions struct {
	MaxTokens int
}

// SendMessage sends messages to the Grok API and returns the response
func (c *GrokClient) SendMessage(messages []ChatMessage, opts SendOptions) (string, error) {
	reqBody := chatRequest{
		Model:     xaiModel,
		Messages:  messages,
		MaxTokens: opts.MaxTokens,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	slog.Debug("sending request to Grok API", "body", string(jsonBody))

	req, err := http.NewRequest("POST", xaiAPIURL, bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	slog.Debug("received response from Grok API", "status", resp.StatusCode, "body", string(body))

	var chatResp chatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		slog.Error("failed to unmarshal response", "body", string(body), "error", err)
		return "", fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if chatResp.Error != nil {
		slog.Error("API returned error", "body", string(body))
		return "", fmt.Errorf("API error: %s", *chatResp.Error)
	}

	if len(chatResp.Choices) == 0 {
		slog.Error("no choices in API response", "body", string(body))
		return "", fmt.Errorf("no response from API")
	}

	return chatResp.Choices[0].Message.Content, nil
}
