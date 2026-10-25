package content

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Brand is the per-bot visual identity sent to the render service.
type Brand struct {
	Key     string `json:"key"`
	NameEN  string `json:"name_en"`
	NameFA  string `json:"name_fa"`
	Handle  string `json:"handle"`
	GlowA   string `json:"glow_a"`
	GlowB   string `json:"glow_b"`
	GlowC   string `json:"glow_c"`
	Accent  string `json:"accent"`
}

// RenderRequest mirrors the renderer microservice contract.
type RenderRequest struct {
	Fmt       string `json:"fmt"` // post | story
	Text      string `json:"text"`
	ImageB64  string `json:"image_b64,omitempty"`
	ImageMime string `json:"image_mime,omitempty"`
	Brand     Brand  `json:"brand"`
}

// RendererAPI renders a card and returns PNG bytes (testable fake).
type RendererAPI interface {
	Render(ctx context.Context, req RenderRequest) ([]byte, error)
}

// Renderer is the HTTP client for the renderer microservice.
type Renderer struct {
	base string
	http *http.Client
}

// NewRenderer returns nil when baseURL is empty (rendering disabled → text fallback).
func NewRenderer(baseURL string) *Renderer {
	if baseURL == "" {
		return nil
	}
	return &Renderer{
		base: baseURL,
		http: &http.Client{Timeout: 60 * time.Second},
	}
}

func (r *Renderer) Render(ctx context.Context, req RenderRequest) ([]byte, error) {
	if r == nil {
		return nil, fmt.Errorf("renderer not configured")
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, r.base+"/render", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := r.http.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("renderer http %d: %s", resp.StatusCode, truncate(string(raw), 200))
	}
	return raw, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
