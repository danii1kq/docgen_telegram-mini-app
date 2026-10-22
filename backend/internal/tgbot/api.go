// Package tgbot is the Telegram bot: it runs inside the API process, uses
// long polling (no public URL or webhook required) and talks to the Bot API
// with the standard library only.
package tgbot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// Update is one getUpdates entry (only the fields the bot needs).
type Update struct {
	UpdateID      int64          `json:"update_id"`
	Message       *Message       `json:"message"`
	CallbackQuery *CallbackQuery `json:"callback_query"`
}

// CallbackQuery is an inline-button press.
type CallbackQuery struct {
	ID      string   `json:"id"`
	From    *User    `json:"from"`
	Data    string   `json:"data"`
	Message *Message `json:"message"`
}

// Message is a Telegram message.
type Message struct {
	MessageID int64  `json:"message_id"`
	From      *User  `json:"from"`
	Chat      Chat   `json:"chat"`
	Text      string `json:"text"`
}

// User is a Telegram account.
type User struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Username  string `json:"username"`
}

// DisplayName returns "Имя @username" for logs and replies.
func (u *User) DisplayName() string {
	if u == nil {
		return ""
	}
	name := u.FirstName
	if u.LastName != "" {
		name += " " + u.LastName
	}
	if u.Username != "" {
		name += " (@" + u.Username + ")"
	}
	return name
}

// Chat is a Telegram chat.
type Chat struct {
	ID        int64  `json:"id"`
	Type      string `json:"type"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
}

// client is a minimal Bot API client.
type client struct {
	token string
	http  *http.Client
	base  string
}

func newClient(token string) *client {
	return &client{
		token: token,
		http:  &http.Client{Timeout: 90 * time.Second},
		base:  "https://api.telegram.org",
	}
}

type apiResponse struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result"`
	Description string          `json:"description"`
	ErrorCode   int             `json:"error_code"`
	// RetryAfter arrives with 429 responses.
	Parameters struct {
		RetryAfter int `json:"retry_after"`
	} `json:"parameters"`
}

// apiError carries the Telegram error description.
type apiError struct {
	Code        int
	Description string
	RetryAfter  int
}

func (e *apiError) Error() string {
	return fmt.Sprintf("telegram: %d %s", e.Code, e.Description)
}

// call performs one Bot API method.
func (c *client) call(ctx context.Context, method string, payload any, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("telegram: encode %s: %w", method, err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.base+"/bot"+c.token+"/"+method, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("telegram: request %s: %w", method, err)
	}
	request.Header.Set("Content-Type", "application/json")
	return c.do(request, method, out)
}

// callMultipart performs one Bot API method with a multipart/form-data body
// (file uploads: sendDocument). fields carry the scalar parameters.
func (c *client) callMultipart(ctx context.Context, method string, fields map[string]string, fileField, fileName string, content []byte, out any) error {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range fields {
		if value == "" {
			continue
		}
		if err := writer.WriteField(key, value); err != nil {
			return fmt.Errorf("telegram: %s field %s: %w", method, key, err)
		}
	}
	part, err := writer.CreateFormFile(fileField, fileName)
	if err != nil {
		return fmt.Errorf("telegram: %s file: %w", method, err)
	}
	if _, err := part.Write(content); err != nil {
		return fmt.Errorf("telegram: %s write file: %w", method, err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("telegram: %s close body: %w", method, err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.base+"/bot"+c.token+"/"+method, &body)
	if err != nil {
		return fmt.Errorf("telegram: request %s: %w", method, err)
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return c.do(request, method, out)
}

// do executes a prepared request and unwraps the Bot API envelope.
func (c *client) do(request *http.Request, method string, out any) error {
	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("telegram: %s: %w", method, err)
	}
	defer response.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("telegram: read %s: %w", method, err)
	}

	var parsed apiResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return fmt.Errorf("telegram: decode %s: %w", method, err)
	}
	if !parsed.OK {
		return &apiError{
			Code:        parsed.ErrorCode,
			Description: parsed.Description,
			RetryAfter:  parsed.Parameters.RetryAfter,
		}
	}
	if out != nil && len(parsed.Result) > 0 {
		if err := json.Unmarshal(parsed.Result, out); err != nil {
			return fmt.Errorf("telegram: decode %s result: %w", method, err)
		}
	}
	return nil
}

// getUpdates long-polls the Bot API.
func (c *client) getUpdates(ctx context.Context, offset int64, timeout time.Duration) ([]Update, error) {
	payload := map[string]any{
		"offset":          offset,
		"timeout":         int(timeout.Seconds()),
		"allowed_updates": []string{"message", "callback_query"},
	}
	var updates []Update
	if err := c.call(ctx, "getUpdates", payload, &updates); err != nil {
		return nil, err
	}
	return updates, nil
}

// sendMessage delivers a plain-text message (no parse mode: topics may contain
// any character and MarkdownV2 escaping would be a liability). markup is an
// optional reply_markup (e.g. an inline keyboard).
func (c *client) sendMessage(ctx context.Context, chatID int64, text string, markup json.RawMessage) error {
	payload := map[string]any{
		"chat_id":                  chatID,
		"text":                     text,
		"disable_web_page_preview": true,
	}
	if len(markup) > 0 {
		payload["reply_markup"] = markup
	}
	return c.call(ctx, "sendMessage", payload, nil)
}

// sendDocument uploads a local file into the chat with an optional caption and
// inline keyboard. This is how a finished order reaches the client: the
// document is the primary delivery path, the signed link is the fallback.
//
// The document field is the OWNER's own file (result.docx/pptx), so
// `attach://` is not needed - a fresh multipart upload works on every client.
func (c *client) sendDocument(ctx context.Context, chatID int64, path, name, caption string, markup json.RawMessage) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("telegram: read document: %w", err)
	}
	if name == "" {
		name = filepath.Base(path)
	}
	fields := map[string]string{
		"chat_id": strconv.FormatInt(chatID, 10),
		"caption": caption,
	}
	if len(markup) > 0 {
		fields["reply_markup"] = string(markup)
	}
	return c.callMultipart(ctx, "sendDocument", fields, "document", name, content, nil)
}

// answerCallbackQuery acknowledges a button press (shows a short toast).
func (c *client) answerCallbackQuery(ctx context.Context, id, text string) error {
	payload := map[string]any{"callback_query_id": id}
	if text != "" {
		payload["text"] = text
		payload["show_alert"] = false
	}
	return c.call(ctx, "answerCallbackQuery", payload, nil)
}

// editMessageText rewrites a sent message. An empty keyboard removes the
// buttons (Telegram keeps the old ones when reply_markup is omitted).
func (c *client) editMessageText(ctx context.Context, chatID, messageID int64, text string) error {
	return c.call(ctx, "editMessageText", map[string]any{
		"chat_id":                  chatID,
		"message_id":               messageID,
		"text":                     text,
		"disable_web_page_preview": true,
		"reply_markup":             map[string]any{"inline_keyboard": []any{}},
	}, nil)
}

// getMe validates the token and returns the bot account.
func (c *client) getMe(ctx context.Context) (User, error) {
	var user User
	if err := c.call(ctx, "getMe", map[string]any{}, &user); err != nil {
		return user, err
	}
	return user, nil
}
