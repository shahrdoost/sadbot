package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestChat_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("auth = %q", r.Header.Get("Authorization"))
		}
		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if req.Model != "test-model" || len(req.Messages) != 2 || req.Messages[0].Role != "system" {
			t.Fatalf("request = %+v", req)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"role": "assistant", "content": "سلام!"}}},
		})
	}))
	defer srv.Close()

	c := &Client{baseURL: srv.URL + "/v1", apiKey: "test-key", model: "test-model", http: srv.Client()}
	got, err := c.Chat(context.Background(), "system-prompt", []Message{{Role: "user", Content: "سلام"}})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if got != "سلام!" {
		t.Fatalf("got = %q", got)
	}
}

func TestChat_DisabledWithoutKey(t *testing.T) {
	c := &Client{baseURL: "http://x", model: "m", http: http.DefaultClient}
	if c.Enabled() {
		t.Fatal("must be disabled without key")
	}
	if _, err := c.Chat(context.Background(), "s", nil); err == nil {
		t.Fatal("Chat must fail when disabled")
	}
	var nilClient *Client
	if nilClient.Enabled() {
		t.Fatal("nil client must be disabled")
	}
}

func TestChat_HTTPErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	c := &Client{baseURL: srv.URL, apiKey: "k", model: "m", http: srv.Client()}
	if _, err := c.Chat(context.Background(), "s", nil); err == nil {
		t.Fatal("429 must error")
	}
}
