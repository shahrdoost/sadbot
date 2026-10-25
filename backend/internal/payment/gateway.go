// Package payment is a client for the catroom.ir (Zibal-compatible) gateway
// plus the dev-mode bypass used for whitelisted test users.
package payment

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type Gateway struct {
	host    string
	http    *http.Client
	devMode bool
	devSet  map[int64]bool
}

// NewGateway builds a gateway from env:
//
//	PAYMENT_HOST_URL    — default https://catroom.ir
//	PAYMENT_DEV_MODE    — "true" enables instant verification
//	PAYMENT_DEV_USERS   — comma-separated telegram ids allowed in dev mode
func NewGateway() *Gateway {
	host := strings.TrimRight(envOr("PAYMENT_HOST_URL", "https://catroom.ir"), "/")
	g := &Gateway{
		host:    host,
		http:    &http.Client{Timeout: 20 * time.Second},
		devMode: os.Getenv("PAYMENT_DEV_MODE") == "true",
		devSet:  map[int64]bool{},
	}
	for _, part := range strings.Split(os.Getenv("PAYMENT_DEV_USERS"), ",") {
		if id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64); err == nil && id > 0 {
			g.devSet[id] = true
		}
	}
	return g
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// DevAllowed reports whether the user bypasses the gateway (instant verify).
func (g *Gateway) DevAllowed(telegramID int64) bool {
	return g.devMode && g.devSet[telegramID]
}

type CreateResult struct {
	PaymentID string
	PayURL    string
}

// CreatePayment requests a payment link. amountTomans is converted to rials (×10)
// exactly like kpkchat. callbackURL must be the public /api/v1/payments/callback URL.
func (g *Gateway) CreatePayment(ctx context.Context, orderID string, amountTomans int64, callbackURL string) (*CreateResult, error) {
	body := map[string]any{
		"orderId":     orderID,
		"amount":      amountTomans * 10,
		"callbackUrl": callbackURL,
		"payerId":     "0",
	}
	data, err := g.call(ctx, http.MethodPost, g.host+"/payment/api/create-payment", body)
	if err != nil {
		return nil, fmt.Errorf("gateway create: %w", err)
	}
	if data["success"] == false {
		return nil, fmt.Errorf("gateway rejected create: %v", data["message"])
	}
	payID := jsonString(data, "paymentId")
	if payID == "" {
		if tid, ok := data["trackId"].(float64); ok {
			payID = strconv.FormatInt(int64(tid), 10)
		}
	}
	if payID == "" {
		return nil, fmt.Errorf("gateway returned no paymentId")
	}
	payURL := jsonString(data, "paymentLink")
	if payURL == "" {
		payURL = g.host + "/payment/pay/" + payID
	}
	payURL = strings.Replace(payURL, "http://", "https://", 1)
	return &CreateResult{PaymentID: payID, PayURL: payURL}, nil
}

// Verify asks the gateway whether the order was paid. err != nil means
// "unknown" (network/parse) — callers must treat it as not-verified but retryable.
func (g *Gateway) Verify(ctx context.Context, orderID string) (bool, error) {
	data, err := g.call(ctx, http.MethodGet, g.host+"/payment/api/verify/"+orderID, nil)
	if err != nil {
		return false, fmt.Errorf("gateway verify: %w", err)
	}
	if verifyResult(data) {
		return true, nil
	}
	return false, nil
}

// verifyResult accepts every success shape kpkchat observed (Zibal variants).
func verifyResult(data map[string]any) bool {
	if v, ok := data["verify"].(map[string]any); ok {
		if r, ok := v["result"].(float64); ok && r == 100 {
			return true
		}
	}
	if r, ok := data["result"].(float64); ok && r == 100 {
		return true
	}
	if p, ok := data["payment"].(map[string]any); ok {
		if s, _ := p["status"].(string); s == "verified" {
			return true
		}
	}
	if s, _ := data["status"].(string); s == "verified" {
		return true
	}
	return false
}

// call POSTs/GETs JSON and parses the JSON response strictly (HTML error pages
// must never crash the caller — lesson from kpkchat's payment handler).
func (g *Gateway) call(ctx context.Context, method, url string, body map[string]any) (map[string]any, error) {
	var rd io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rd = strings.NewReader(string(raw))
	}
	req, err := http.NewRequestWithContext(ctx, method, url, rd)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := g.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("http %d: %s", resp.StatusCode, snippet(raw))
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "application/json") && !strings.HasPrefix(strings.TrimSpace(string(raw)), "{") {
		return nil, fmt.Errorf("non-json response (%s): %s", ct, snippet(raw))
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("json parse: %s", snippet(raw))
	}
	return out, nil
}

func snippet(raw []byte) string {
	s := strings.Join(strings.Fields(string(raw)), " ")
	if len(s) > 160 {
		s = s[:160]
	}
	return s
}

func jsonString(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}
