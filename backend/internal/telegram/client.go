package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// APIError is a Telegram Bot API error with optional retry_after.
type APIError struct {
	Code        int
	Description string
	RetryAfter  int
}

func (e *APIError) Error() string {
	return fmt.Sprintf("telegram api %d: %s", e.Code, e.Description)
}

func (e *APIError) IsAuthError() bool { return e.Code == 401 }

func (e *APIError) IsForbidden() bool { return e.Code == 403 }

// Client is a thin hand-rolled Bot API client.
type Client struct {
	token string
	base  string
	http  *http.Client
}

func NewClient(token string) *Client {
	return &Client{
		token: token,
		base:  "https://api.telegram.org/bot" + token + "/",
		http:  &http.Client{Timeout: 60 * time.Second},
	}
}

func (c *Client) do(ctx context.Context, method string, payload any, out any) error {
	var body []byte
	var err error
	if payload != nil {
		body, err = json.Marshal(payload)
		if err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+method, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	var apiResp APIResponse
	if err := json.Unmarshal(raw, &apiResp); err != nil {
		return fmt.Errorf("telegram %s: bad json: %w", method, err)
	}
	if !apiResp.OK {
		apiErr := &APIError{Code: apiResp.ErrorCode, Description: apiResp.Description}
		if apiResp.Parameters != nil {
			apiErr.RetryAfter = apiResp.Parameters.RetryAfter
		}
		return apiErr
	}
	if out != nil && len(apiResp.Result) > 0 {
		return json.Unmarshal(apiResp.Result, out)
	}
	return nil
}

func (c *Client) GetMe(ctx context.Context) (*GetMeResult, error) {
	var out GetMeResult
	if err := c.do(ctx, "getMe", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetUpdates long-polls. Pass a ctx with timeout ≈ long-poll timeout.
func (c *Client) GetUpdates(ctx context.Context, offset int64, timeoutSec int) ([]Update, error) {
	form := url.Values{}
	form.Set("offset", strconv.FormatInt(offset, 10))
	form.Set("timeout", strconv.Itoa(timeoutSec))
	form.Set("allowed_updates", `["message","callback_query"]`)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"getUpdates", bytes.NewBufferString(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	var apiResp APIResponse
	if err := json.Unmarshal(raw, &apiResp); err != nil {
		return nil, err
	}
	if !apiResp.OK {
		apiErr := &APIError{Code: apiResp.ErrorCode, Description: apiResp.Description}
		if apiResp.Parameters != nil {
			apiErr.RetryAfter = apiResp.Parameters.RetryAfter
		}
		return nil, apiErr
	}
	var updates []Update
	if err := json.Unmarshal(apiResp.Result, &updates); err != nil {
		return nil, err
	}
	return updates, nil
}

func (c *Client) SendMessage(ctx context.Context, p SendMessagePayload) (*Message, error) {
	var msg Message
	if err := c.do(ctx, "sendMessage", p, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

func (c *Client) SendPhoto(ctx context.Context, p SendPhotoPayload) (*Message, error) {
	var msg Message
	if err := c.do(ctx, "sendPhoto", p, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

// SendPhotoUpload uploads a local image (multipart) with a caption — used by
// the content template to post rendered PNG cards to a channel.
func (c *Client) SendPhotoUpload(ctx context.Context, chatID any, filename string, image []byte, caption string) (*Message, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("chat_id", fmt.Sprint(chatID))
	if caption != "" {
		_ = mw.WriteField("caption", caption)
	}
	fw, err := mw.CreateFormFile("photo", filename)
	if err != nil {
		return nil, err
	}
	if _, err := fw.Write(image); err != nil {
		return nil, err
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"sendPhoto", &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	var apiResp APIResponse
	if err := json.Unmarshal(raw, &apiResp); err != nil {
		return nil, err
	}
	if !apiResp.OK {
		apiErr := &APIError{Code: apiResp.ErrorCode, Description: apiResp.Description}
		if apiResp.Parameters != nil {
			apiErr.RetryAfter = apiResp.Parameters.RetryAfter
		}
		return nil, apiErr
	}
	var msg Message
	if len(apiResp.Result) > 0 {
		if err := json.Unmarshal(apiResp.Result, &msg); err != nil {
			return nil, err
		}
	}
	return &msg, nil
}

// ForwardMedia copies a message the bot received in fromChatID into toChat.
// This is how the dating chat relays photos, stickers, voice notes, videos,
// GIFs and files between two users without the bot ever downloading them.
func (c *Client) ForwardMedia(ctx context.Context, fromChatID, toChatID, messageID int64) (*Message, error) {
	payload := map[string]any{
		"chat_id":      toChatID,
		"from_chat_id": fromChatID,
		"message_id":   messageID,
	}
	var msg Message
	if err := c.do(ctx, "forwardMessage", payload, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

func (c *Client) EditMessageText(ctx context.Context, p EditMessageTextPayload) (*Message, error) {
	var msg Message
	if err := c.do(ctx, "editMessageText", p, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

func (c *Client) EditMessageCaption(ctx context.Context, p EditMessageCaptionPayload) (*Message, error) {
	var msg Message
	if err := c.do(ctx, "editMessageCaption", p, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

func (c *Client) SendChatAction(ctx context.Context, chatID int64, action string) error {
	return c.do(ctx, "sendChatAction", map[string]any{"chat_id": chatID, "action": action}, nil)
}

func (c *Client) AnswerCallbackQuery(ctx context.Context, p AnswerCallbackPayload) error {
	return c.do(ctx, "answerCallbackQuery", p, nil)
}

// GetChatMemberStatus returns the member status or "" when not a member.
// chatID may be a numeric id or an @username (Bot API accepts both).
func (c *Client) GetChatMemberStatus(ctx context.Context, chatID any, userID int64) (string, error) {
	var out struct {
		Status string `json:"status"`
	}
	err := c.do(ctx, "getChatMember", map[string]any{"chat_id": chatID, "user_id": userID}, &out)
	if err != nil {
		var apiErr *APIError
		if ok := asAPIError(err, &apiErr); ok && apiErr.Code == 400 {
			return "", nil
		}
		return "", err
	}
	return out.Status, nil
}

func asAPIError(err error, target **APIError) bool {
	if e, ok := err.(*APIError); ok {
		*target = e
		return true
	}
	return false
}
