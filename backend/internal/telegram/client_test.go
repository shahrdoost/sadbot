package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newTestClient points a Client at a local httptest server.
func newTestClient(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c := NewClient("TESTTOKEN")
	c.base = srv.URL + "/botTESTTOKEN/"
	return c, srv
}

func okResponse(result any) []byte {
	raw, _ := json.Marshal(result)
	out, _ := json.Marshal(APIResponse{OK: true, Result: raw})
	return out
}

func errResponse(code int, desc string, retryAfter int) []byte {
	resp := APIResponse{OK: false, ErrorCode: code, Description: desc}
	if retryAfter > 0 {
		resp.Parameters = &struct {
			RetryAfter int `json:"retry_after"`
		}{RetryAfter: retryAfter}
	}
	out, _ := json.Marshal(resp)
	return out
}

func TestGetMe_Success(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/botTESTTOKEN/getMe" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.Write(okResponse(map[string]any{
			"id": 42, "is_bot": true, "first_name": "Test", "username": "test_bot",
		}))
	})

	me, err := c.GetMe(context.Background())
	if err != nil {
		t.Fatalf("GetMe: %v", err)
	}
	if me.Username != "test_bot" || !me.IsBot {
		t.Fatalf("me = %+v", me)
	}
}

func TestGetMe_AuthError(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write(errResponse(401, "Unauthorized", 0))
	})

	_, err := c.GetMe(context.Background())
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %v", err)
	}
	if !apiErr.IsAuthError() || apiErr.IsForbidden() {
		t.Fatalf("error flags wrong: %+v", apiErr)
	}
}

func TestSendMessage_Forbidden(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		_ = json.NewDecoder(r.Body).Decode(&payload)
		if payload["chat_id"].(float64) != 99 {
			t.Errorf("chat_id = %v", payload["chat_id"])
		}
		w.Write(errResponse(403, "Forbidden: bot was blocked by the user", 0))
	})

	_, err := c.SendMessage(context.Background(), SendMessagePayload{ChatID: 99, Text: "سلام"})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || !apiErr.IsForbidden() {
		t.Fatalf("expected forbidden APIError, got %v", err)
	}
}

func TestGetUpdates_ParsesAndRateLimits(t *testing.T) {
	calls := 0
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			// first: one update
			w.Write(okResponse([]any{
				map[string]any{"update_id": 1, "message": map[string]any{
					"message_id": 10, "text": "سلام",
					"chat": map[string]any{"id": 5, "type": "private"},
					"from": map[string]any{"id": 7, "first_name": "ع", "is_bot": false},
				}},
			}))
			return
		}
		// second: rate limited
		w.Write(errResponse(429, "Too Many Requests: retry after 30", 30))
	})

	updates, err := c.GetUpdates(context.Background(), 0, 30)
	if err != nil {
		t.Fatalf("GetUpdates: %v", err)
	}
	if len(updates) != 1 || updates[0].Message.Text != "سلام" {
		t.Fatalf("updates = %+v", updates)
	}
	if updates[0].EffectiveUser().ID != 7 || updates[0].EffectiveChatID() != 5 {
		t.Fatalf("effective accessors wrong: %+v", updates[0])
	}

	_, err = c.GetUpdates(context.Background(), 1, 30)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.RetryAfter != 30 {
		t.Fatalf("expected retry_after=30, got %v", err)
	}
}

func TestGetChatMemberStatus_NotMember(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		// Telegram answers 400 when the user is not a member
		w.Write(errResponse(400, "Bad Request: user not found", 0))
	})

	status, err := c.GetChatMemberStatus(context.Background(), 1, 2)
	if err != nil {
		t.Fatalf("expected no error for non-member, got %v", err)
	}
	if status != "" {
		t.Fatalf("status = %q", status)
	}
}

func TestGetChatMemberStatus_Member(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write(okResponse(map[string]any{"status": "member", "user": map[string]any{"id": 2}}))
	})

	status, err := c.GetChatMemberStatus(context.Background(), 1, 2)
	if err != nil {
		t.Fatalf("GetChatMemberStatus: %v", err)
	}
	if status != "member" {
		t.Fatalf("status = %q", status)
	}
}

func TestSendPhoto_WithFileID(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/botTESTTOKEN/sendPhoto" {
			t.Errorf("path = %s", r.URL.Path)
		}
		var payload SendPhotoPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if payload.Photo != "file-abc" || payload.Caption != "سلام" {
			t.Fatalf("payload = %+v", payload)
		}
		w.Write(okResponse(map[string]any{
			"message_id": 7, "date": 1, "chat": map[string]any{"id": 5, "type": "private"},
		}))
	})

	msg, err := c.SendPhoto(context.Background(), SendPhotoPayload{ChatID: 5, Photo: "file-abc", Caption: "سلام"})
	if err != nil {
		t.Fatalf("SendPhoto: %v", err)
	}
	if msg.MessageID != 7 {
		t.Fatalf("msg = %+v", msg)
	}
}

func TestMessage_PhotoFileID_Largest(t *testing.T) {
	m := &Message{Photo: []PhotoSize{
		{FileID: "small"}, {FileID: "large"},
	}}
	if got := m.PhotoFileID(); got != "large" {
		t.Fatalf("PhotoFileID = %q", got)
	}
	if got := (&Message{}).PhotoFileID(); got != "" {
		t.Fatalf("empty PhotoFileID = %q", got)
	}
}

func TestAnswerCallbackQuery_OK(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write(okResponse(true))
	})
	if err := c.AnswerCallbackQuery(context.Background(), AnswerCallbackPayload{CallbackQueryID: "cb1"}); err != nil {
		t.Fatalf("AnswerCallbackQuery: %v", err)
	}
}

func TestBadJSONResponse(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	})
	if _, err := c.GetMe(context.Background()); err == nil {
		t.Fatal("non-JSON response must produce an error")
	}
}
