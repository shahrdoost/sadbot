package payment

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestGateway(t *testing.T, handler http.HandlerFunc) (*Gateway, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	g := &Gateway{host: srv.URL, http: srv.Client(), devSet: map[int64]bool{}}
	return g, srv
}

func TestCreatePayment_Success(t *testing.T) {
	var got map[string]any
	g, _ := newTestGateway(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/payment/api/create-payment" || r.Method != http.MethodPost {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true, "paymentId": "12345", "paymentLink": "http://gateway/pay/12345",
		})
	})
	res, err := g.CreatePayment(context.Background(), "order-1", 50_000, "https://panel/callback")
	if err != nil {
		t.Fatal(err)
	}
	if res.PaymentID != "12345" {
		t.Fatalf("paymentID = %q", res.PaymentID)
	}
	if res.PayURL != "https://gateway/pay/12345" {
		t.Fatalf("payURL = %q (must be https)", res.PayURL)
	}
	if got["amount"] != float64(500_000) { // tomans × 10 = rials
		t.Fatalf("amount = %v", got["amount"])
	}
	if got["orderId"] != "order-1" || got["callbackUrl"] != "https://panel/callback" {
		t.Fatalf("body = %+v", got)
	}
}

func TestCreatePayment_GatewayRejects(t *testing.T) {
	g, _ := newTestGateway(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "message": "bad order"})
	})
	if _, err := g.CreatePayment(context.Background(), "o", 10_000, "cb"); err == nil {
		t.Fatal("expected error on success=false")
	}
}

func TestCreatePayment_NonJSONPage(t *testing.T) {
	g, _ := newTestGateway(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html>502 Bad Gateway</html>"))
	})
	if _, err := g.CreatePayment(context.Background(), "o", 10_000, "cb"); err == nil {
		t.Fatal("HTML page must not parse as success")
	}
}

func TestVerify_ZibalShapes(t *testing.T) {
	shapes := []map[string]any{
		{"verify": map[string]any{"result": float64(100)}},
		{"result": float64(100)},
		{"payment": map[string]any{"status": "verified"}},
		{"status": "verified"},
	}
	for i, shape := range shapes {
		g, _ := newTestGateway(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/payment/api/verify/order-1" {
				t.Errorf("path = %s", r.URL.Path)
			}
			_ = json.NewEncoder(w).Encode(shape)
		})
		ok, err := g.Verify(context.Background(), "order-1")
		if err != nil || !ok {
			t.Fatalf("shape %d: ok=%v err=%v", i, ok, err)
		}
	}
}

func TestVerify_NotPaid(t *testing.T) {
	g, _ := newTestGateway(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"result": float64(201)})
	})
	ok, err := g.Verify(context.Background(), "order-1")
	if err != nil || ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}

func TestDevAllowed(t *testing.T) {
	g := &Gateway{devMode: true, devSet: map[int64]bool{42: true}}
	if !g.DevAllowed(42) || g.DevAllowed(7) {
		t.Fatal("dev whitelist mismatch")
	}
	g.devMode = false
	if g.DevAllowed(42) {
		t.Fatal("dev mode off must deny")
	}
}
