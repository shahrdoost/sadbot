// Package tgsidecar is a thin HTTP client for the Telethon sidecar service.
package tgsidecar

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	base string
	key  string
	http *http.Client
}

func New(baseURL, key string) *Client {
	return &Client{
		base: strings.TrimRight(baseURL, "/"),
		key:  key,
		http: &http.Client{Timeout: 90 * time.Second},
	}
}

// Error is a sidecar HTTP error with a machine-readable code.
type Error struct {
	Status  int
	Code    string // PASSWORD_NEEDED | WRONG_PASSWORD | INVALID_CODE | FLOOD_WAIT | account not found | ...
	Seconds int    // FLOOD_WAIT only
}

func (e *Error) Error() string {
	if e.Code == "FLOOD_WAIT" {
		return fmt.Sprintf("flood wait %ds", e.Seconds)
	}
	return fmt.Sprintf("tgsidecar %d: %s", e.Status, e.Code)
}

type User struct {
	ID        string `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Username  string `json:"username"`
	Phone     string `json:"phone"`
}

type Account struct {
	AccountID string `json:"account_id"`
	User      User   `json:"user"`
}

type LastMessage struct {
	Text string  `json:"text"`
	Date *string `json:"date"`
}

type Chat struct {
	ChatID      string       `json:"chat_id"`
	Title       string       `json:"title"`
	Type        string       `json:"type"`
	Unread      int          `json:"unread"`
	Pinned      bool         `json:"pinned"`
	LastMessage *LastMessage `json:"last_message"`
}

type Sender struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	IsOut bool   `json:"is_out"`
}

type Media struct {
	Kind string `json:"kind"`
}

type Message struct {
	ID         int     `json:"id"`
	Date       *string `json:"date"`
	Sender     Sender  `json:"sender"`
	Text       string  `json:"text"`
	Media      Media   `json:"media"`
	ReplyToID  *int    `json:"reply_to_id"`
	Forwarded  bool    `json:"forwarded"`
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	return c.doWith(c.http, ctx, method, path, body, out)
}

func (c *Client) doWith(hc *http.Client, ctx context.Context, method, path string, body any, out any) error {
	var rd io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = strings.NewReader(string(raw))
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("X-Sidecar-Key", c.key)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return parseError(resp.StatusCode, raw)
	}
	if out != nil {
		return json.Unmarshal(raw, out)
	}
	return nil
}

func parseError(status int, raw []byte) *Error {
	e := &Error{Status: status, Code: "sidecar error"}
	var envelope struct {
		Detail json.RawMessage `json:"detail"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || len(envelope.Detail) == 0 {
		return e
	}
	// detail is either a string or {"error":..,"seconds":..}
	var s string
	if err := json.Unmarshal(envelope.Detail, &s); err == nil {
		e.Code = s
		if s == "PASSWORD_NEEDED" {
			e.Status = 401
		}
		return e
	}
	var obj struct {
		Error   string `json:"error"`
		Seconds int    `json:"seconds"`
	}
	if err := json.Unmarshal(envelope.Detail, &obj); err == nil {
		e.Code = obj.Error
		e.Seconds = obj.Seconds
	}
	return e
}

func (c *Client) LoginStart(ctx context.Context, phone string) (pendingID string, err error) {
	var out struct {
		PendingID string `json:"pending_id"`
	}
	if err := c.do(ctx, http.MethodPost, "/login/start", map[string]string{"phone": phone}, &out); err != nil {
		return "", err
	}
	return out.PendingID, nil
}

func (c *Client) LoginComplete(ctx context.Context, pendingID, phone, code, password string) (*Account, error) {
	body := map[string]string{"pending_id": pendingID, "phone": phone, "code": code}
	if password != "" {
		body["password"] = password
	}
	var out struct {
		AccountID string `json:"account_id"`
		User      User   `json:"user"`
	}
	if err := c.do(ctx, http.MethodPost, "/login/complete", body, &out); err != nil {
		return nil, err
	}
	return &Account{AccountID: out.AccountID, User: out.User}, nil
}

func (c *Client) ListAccounts(ctx context.Context) ([]Account, error) {
	var out struct {
		Accounts []Account `json:"accounts"`
	}
	if err := c.do(ctx, http.MethodGet, "/accounts", nil, &out); err != nil {
		return nil, err
	}
	return out.Accounts, nil
}

func (c *Client) DeleteAccount(ctx context.Context, accountID string) error {
	return c.do(ctx, http.MethodDelete, "/accounts/"+accountID, nil, nil)
}

func (c *Client) ListChats(ctx context.Context, accountID string) ([]Chat, error) {
	var out struct {
		Chats []Chat `json:"chats"`
	}
	if err := c.do(ctx, http.MethodGet, "/accounts/"+accountID+"/chats", nil, &out); err != nil {
		return nil, err
	}
	return out.Chats, nil
}

// FolderPeer is one chat inside a Telegram dialog folder.
type FolderPeer struct {
	ChatID   string `json:"chat_id"`
	Title    string `json:"title"`
	Username string `json:"username"`
	Type     string `json:"type"` // user | group | channel | bot
}

// Folder is a Telegram dialog folder (chat folder) with its included peers.
type Folder struct {
	Name  string       `json:"name"`
	Peers []FolderPeer `json:"peers"`
}

// ListFolders returns the account's dialog folders.
func (c *Client) ListFolders(ctx context.Context, accountID string) ([]Folder, error) {
	var out struct {
		Folders []Folder `json:"folders"`
	}
	if err := c.do(ctx, http.MethodGet, "/accounts/"+accountID+"/folders", nil, &out); err != nil {
		return nil, err
	}
	return out.Folders, nil
}

func (c *Client) ListMessages(ctx context.Context, accountID, chatID string, limit, offsetID int) ([]Message, int, error) {
	path := fmt.Sprintf("/accounts/%s/chats/%s/messages?limit=%d&offset_id=%d", accountID, chatID, limit, offsetID)
	var out struct {
		Messages []Message `json:"messages"`
		OldestID int       `json:"oldest_id"`
	}
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, 0, err
	}
	return out.Messages, out.OldestID, nil
}

// SendMessage posts a text message in the chat and returns the sent message.
func (c *Client) SendMessage(ctx context.Context, accountID, chatID, text string) (*Message, error) {
	path := fmt.Sprintf("/accounts/%s/chats/%s/messages", accountID, chatID)
	var out struct {
		Message Message `json:"message"`
	}
	if err := c.do(ctx, http.MethodPost, path, map[string]string{"text": text}, &out); err != nil {
		return nil, err
	}
	return &out.Message, nil
}

// SendPhotoMeta publishes a PNG (base64) with a caption from the MTProto
// account — the sadcontent/telethon path for channels the running bot cannot
// post into.
func (c *Client) SendPhotoMeta(ctx context.Context, accountID, chatID, imageB64, filename, caption string) (*Message, error) {
	path := fmt.Sprintf("/accounts/%s/chats/%s/photo", accountID, chatID)
	var out struct {
		Message Message `json:"message"`
	}
	body := map[string]string{"image_b64": imageB64, "filename": filename, "caption": caption}
	if err := c.do(ctx, http.MethodPost, path, body, &out); err != nil {
		return nil, err
	}
	return &out.Message, nil
}

// GetPhoto returns the photo stream; caller must close the reader.
func (c *Client) GetPhoto(ctx context.Context, accountID, chatID string, msgID int) (io.ReadCloser, string, error) {
	path := fmt.Sprintf("/accounts/%s/chats/%s/messages/%d/photo", accountID, chatID, msgID)
	return c.stream(ctx, path)
}

// ChatPhoto returns the chat/entity profile photo stream; caller must close the reader.
func (c *Client) ChatPhoto(ctx context.Context, accountID, chatID string) (io.ReadCloser, string, error) {
	return c.stream(ctx, fmt.Sprintf("/accounts/%s/chats/%s/photo", accountID, chatID))
}

type BotFatherResult struct {
	Token    string `json:"token"`
	Username string `json:"username"`
}

// CreateBotViaBotFather drives the BotFather /newbot conversation from the account.
// The conversation can take a while (multiple candidate usernames), so it gets its own budget.
func (c *Client) CreateBotViaBotFather(ctx context.Context, accountID, name, hint string) (*BotFatherResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 6*time.Minute)
	defer cancel()
	body := map[string]string{"name": name}
	if hint != "" {
		body["username_hint"] = hint
	}
	var out BotFatherResult
	// dedicated client: the shared 90s timeout would abort mid-conversation;
	// the 6-minute context budget above is the real bound here
	longHTTP := &http.Client{Timeout: 0}
	if err := c.doWith(longHTTP, ctx, http.MethodPost, "/accounts/"+accountID+"/botfather/create-bot", body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) stream(ctx context.Context, path string) (io.ReadCloser, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("X-Sidecar-Key", c.key)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		return nil, "", parseError(resp.StatusCode, raw)
	}
	return resp.Body, resp.Header.Get("Content-Type"), nil
}
