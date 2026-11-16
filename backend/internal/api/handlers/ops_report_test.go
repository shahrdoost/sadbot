package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"sadbot/internal/payment"
	"sadbot/internal/telegram"
)

// captureAPI stands in for the Telegram endpoint the ops channel talks to.
type captureAPI struct {
	mu    sync.Mutex
	texts []string
	srv   *httptest.Server
}

func newCaptureAPI(t *testing.T) *captureAPI {
	t.Helper()
	c := &captureAPI{}
	c.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Text string `json:"text"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		c.mu.Lock()
		c.texts = append(c.texts, body.Text)
		c.mu.Unlock()
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1}}`))
	}))
	t.Cleanup(c.srv.Close)
	return c
}

func (c *captureAPI) posted() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.texts...)
}

func channelFor(t *testing.T, api *captureAPI) *payment.OpsChannel {
	t.Helper()
	return payment.NewOpsChannelWithClient(
		telegram.NewClientWithBase(api.srv.URL+"/bot", "T"), "-1004306622791", nil)
}

func post(t *testing.T, h *OpsReportHandler, token string, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ops/payment-report", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.Report(rec, req)
	return rec
}

func validBody(status string) string {
	b, _ := json.Marshal(map[string]any{
		"platform": "sadai", "status": status, "amount": 294_998,
		"user": "u_42", "item": "بستهٔ اعتبار", "currency": "تومان",
	})
	return string(b)
}

// No token, wrong token, and a token with the right prefix must all be
// refused — this endpoint is public, so it is the only thing standing between
// the channel and anyone who finds the path.
func TestReportRejectsBadTokens(t *testing.T) {
	api := newCaptureAPI(t)
	h := NewOpsReportHandler(channelFor(t, api), "secret-token-value")

	for _, tc := range []struct{ name, token string }{
		{"missing", ""},
		{"wrong", "nope"},
		{"prefix", "secret-token-valu"},
		{"extended", "secret-token-valueX"},
	} {
		rec := post(t, h, tc.token, validBody("success"))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: status = %d (want 401)", tc.name, rec.Code)
		}
	}
	if n := len(api.posted()); n != 0 {
		t.Fatalf("unauthorized requests produced %d posts", n)
	}
}

func TestReportPostsSuccessAndFailure(t *testing.T) {
	api := newCaptureAPI(t)
	h := NewOpsReportHandler(channelFor(t, api), "tok")

	if rec := post(t, h, "tok", validBody("success")); rec.Code != http.StatusOK {
		t.Fatalf("success status = %d", rec.Code)
	}
	if rec := post(t, h, "tok", validBody("failed")); rec.Code != http.StatusOK {
		t.Fatalf("failed status = %d", rec.Code)
	}

	posts := api.posted()
	if len(posts) != 2 {
		t.Fatalf("posts = %d (want 2): %v", len(posts), posts)
	}
	if !strings.Contains(posts[0], "پرداخت موفق") {
		t.Errorf("success post lost its heading:\n%s", posts[0])
	}
	if !strings.Contains(posts[1], "پرداخت ناموفق") {
		t.Errorf("failure post lost its heading:\n%s", posts[1])
	}
	for _, p := range posts {
		if !strings.Contains(p, "294,998") {
			t.Errorf("post missing the grouped amount:\n%s", p)
		}
		if !strings.Contains(p, "سعدی") {
			t.Errorf("post missing the platform label:\n%s", p)
		}
	}
}

// A token set in the environment but an unconfigured channel must answer
// "accepted, nothing to do" rather than 401 — otherwise the platform starts
// retrying a request we deliberately do not want to fulfil.
func TestReportWithDisabledChannelIsQuiet(t *testing.T) {
	h := NewOpsReportHandler(nil, "tok")
	rec := post(t, h, "tok", validBody("success"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (want 200)", rec.Code)
	}
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out["skipped"] == nil {
		t.Errorf("response should say it was skipped: %s", rec.Body.String())
	}
}

func TestReportValidatesInput(t *testing.T) {
	api := newCaptureAPI(t)
	h := NewOpsReportHandler(channelFor(t, api), "tok")

	cases := map[string]string{
		"no platform": `{"status":"success","amount":10}`,
		"no status":   `{"platform":"sadai","amount":10}`,
		"negative":    `{"platform":"sadai","status":"success","amount":-1}`,
		"not json":    `{`,
		"unknown key": `{"platform":"sadai","status":"success","amount":1,"evil":true}`,
	}
	for name, body := range cases {
		rec := post(t, h, "tok", body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d (want 400)", name, rec.Code)
		}
	}
	if n := len(api.posted()); n != 0 {
		t.Fatalf("invalid input produced %d posts", n)
	}
}

// An oversized body must be refused before it is buffered whole.
func TestReportRejectsOversizedBody(t *testing.T) {
	api := newCaptureAPI(t)
	h := NewOpsReportHandler(channelFor(t, api), "tok")

	big := strings.Repeat("x", maxReportBytes+1024)
	body := `{"platform":"sadai","status":"success","amount":1,"detail":"` + big + `"}`
	rec := post(t, h, "tok", body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d (want 400)", rec.Code)
	}
}

// The header-based alternative must work for clients that cannot set
// Authorization, and must be held to the same comparison.
func TestReportAcceptsOpsTokenHeader(t *testing.T) {
	api := newCaptureAPI(t)
	h := NewOpsReportHandler(channelFor(t, api), "tok")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/ops/payment-report", bytes.NewBufferString(validBody("success")))
	req.Header.Set("X-Ops-Token", "tok")
	rec := httptest.NewRecorder()
	h.Report(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (want 200)", rec.Code)
	}
	if len(api.posted()) != 1 {
		t.Fatal("header-authenticated report did not post")
	}
}
