package tgsidecar

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestServer(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return New(srv.URL, "test-key")
}

func TestClient_SendsSidecarKeyHeader(t *testing.T) {
	var gotKey string
	c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("X-Sidecar-Key")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "pending_id": "p1"})
	})
	pending, err := c.LoginStart(context.Background(), "+98903")
	if err != nil {
		t.Fatal(err)
	}
	if gotKey != "test-key" {
		t.Fatalf("header = %q", gotKey)
	}
	if pending != "p1" {
		t.Fatalf("pending = %q", pending)
	}
}

func TestClient_ListChats_StringIDs(t *testing.T) {
	c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/accounts/acc1/chats" {
			t.Errorf("path = %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"chats": []map[string]any{
				{"chat_id": "-1003825098640", "title": "کانال", "type": "channel", "unread": 3, "pinned": true},
			},
		})
	})
	chats, err := c.ListChats(context.Background(), "acc1")
	if err != nil {
		t.Fatal(err)
	}
	if len(chats) != 1 || chats[0].ChatID != "-1003825098640" || !chats[0].Pinned {
		t.Fatalf("chats = %+v", chats)
	}
}

func TestClient_ListMessages(t *testing.T) {
	c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "offset_id=100") {
			t.Errorf("query = %s", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"messages": []map[string]any{
				{"id": 7, "text": "سلام", "sender": map[string]any{"id": "1", "name": "علی", "is_out": true}, "media": map[string]any{"kind": "photo"}, "reply_to_id": 2},
			},
			"oldest_id": 7,
		})
	})
	msgs, oldest, err := c.ListMessages(context.Background(), "acc1", "-1001", 50, 100)
	if err != nil {
		t.Fatal(err)
	}
	if oldest != 7 || len(msgs) != 1 {
		t.Fatalf("msgs=%+v oldest=%d", msgs, oldest)
	}
	if !msgs[0].Sender.IsOut || msgs[0].Media.Kind != "photo" || msgs[0].ReplyToID == nil || *msgs[0].ReplyToID != 2 {
		t.Fatalf("message = %+v", msgs[0])
	}
}

func TestClient_SendMessage(t *testing.T) {
	c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/accounts/acc1/chats/-1001/messages" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		var body struct {
			Text string `json:"text"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Text != "سلام" {
			t.Errorf("body = %+v", body)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"message": map[string]any{"id": 9, "text": "سلام", "sender": map[string]any{"id": "42", "is_out": true}, "media": map[string]any{"kind": "none"}},
		})
	})
	msg, err := c.SendMessage(context.Background(), "acc1", "-1001", "سلام")
	if err != nil {
		t.Fatal(err)
	}
	if msg.ID != 9 || !msg.Sender.IsOut {
		t.Fatalf("message = %+v", msg)
	}
}

func TestClient_SendMessage_WriteForbidden(t *testing.T) {
	c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"detail": "WRITE_FORBIDDEN"})
	})
	_, err := c.SendMessage(context.Background(), "acc1", "-1001", "hi")
	var scErr *Error
	if !asSidecarError(err, &scErr) || scErr.Code != "WRITE_FORBIDDEN" {
		t.Fatalf("err = %v", err)
	}
}

func TestClient_PasswordNeeded(t *testing.T) {
	c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{"detail": "PASSWORD_NEEDED"})
	})
	_, err := c.LoginComplete(context.Background(), "p", "+98903", "123", "")
	var scErr *Error
	if !asSidecarError(err, &scErr) || scErr.Code != "PASSWORD_NEEDED" || scErr.Status != 401 {
		t.Fatalf("err = %v", err)
	}
}

func TestClient_FloodWait(t *testing.T) {
	c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_ = json.NewEncoder(w).Encode(map[string]any{"detail": map[string]any{"error": "FLOOD_WAIT", "seconds": 33}})
	})
	_, _, err := c.ListMessages(context.Background(), "a", "1", 10, 0)
	var scErr *Error
	if !asSidecarError(err, &scErr) || scErr.Code != "FLOOD_WAIT" || scErr.Seconds != 33 {
		t.Fatalf("err = %v", err)
	}
}

func TestClient_GetPhoto_StreamsBody(t *testing.T) {
	c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte{0xFF, 0xD8, 0xFF, 0xD9})
	})
	body, ctype, err := c.GetPhoto(context.Background(), "acc1", "-1001", 5)
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	raw, _ := io.ReadAll(body)
	if ctype != "image/jpeg" || len(raw) != 4 {
		t.Fatalf("ctype=%s len=%d", ctype, len(raw))
	}
}

// asSidecarError is errors.As for tests.
func asSidecarError(err error, target **Error) bool {
	e, ok := err.(*Error)
	if ok {
		*target = e
	}
	return ok
}

func TestClient_ChatPhoto(t *testing.T) {
	c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/accounts/acc1/chats/-1001/photo" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte{0xFF, 0xD8, 0xFF, 0xD9})
	})
	body, ctype, err := c.ChatPhoto(context.Background(), "acc1", "-1001")
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	raw, _ := io.ReadAll(body)
	if ctype != "image/jpeg" || len(raw) != 4 {
		t.Fatalf("ctype=%s len=%d", ctype, len(raw))
	}
}
