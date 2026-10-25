// Package llm is a minimal OpenAI-compatible chat-completions client.
// Configure via LLM_BASE_URL / LLM_API_KEY / LLM_MODEL; without a key the
// client reports Disabled and callers fall back to scripted replies.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"
)

type Message struct {
	Role    string `json:"role"` // "user" | "assistant"
	Content string `json:"content"`
}

type Client struct {
	baseURL     string
	apiKey      string
	model       string
	temperature float64
	http        *http.Client
}

// NewFromEnv builds a client from LLM_* env vars (all optional).
// LLM_TEMPERATURE defaults to 0 (model default) — set 0.3 to match
// sadcontent's cleaning calls.
func NewFromEnv() *Client {
	base := os.Getenv("LLM_BASE_URL")
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	model := os.Getenv("LLM_MODEL")
	if model == "" {
		model = "gpt-4o-mini"
	}
	temp := 0.0
	if v := os.Getenv("LLM_TEMPERATURE"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			temp = f
		}
	}
	return &Client{
		baseURL:     base,
		apiKey:      os.Getenv("LLM_API_KEY"),
		model:       model,
		temperature: temp,
		http:        &http.Client{Timeout: 45 * time.Second},
	}
}

// Enabled reports whether a real LLM endpoint is configured.
func (c *Client) Enabled() bool { return c != nil && c.apiKey != "" }

type chatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature *float64  `json:"temperature,omitempty"` // nil = provider default
}

type chatResponse struct {
	Choices []struct {
		Message Message `json:"message"`
	} `json:"choices"`
}

// Chat sends the system prompt + history and returns the assistant reply.
func (c *Client) Chat(ctx context.Context, system string, history []Message) (string, error) {
	if !c.Enabled() {
		return "", fmt.Errorf("llm not configured")
	}
	msgs := make([]Message, 0, len(history)+1)
	msgs = append(msgs, Message{Role: "system", Content: system})
	msgs = append(msgs, history...)
	reqBody := chatRequest{Model: c.model, Messages: msgs}
	if c.temperature != 0 {
		t := c.temperature
		reqBody.Temperature = &t
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("llm http %d", resp.StatusCode)
	}
	var out chatResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("llm bad json: %w", err)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("llm empty choices")
	}
	return out.Choices[0].Message.Content, nil
}
