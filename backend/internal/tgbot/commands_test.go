package tgbot

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/srunal/srunal-go/internal/config"
	"github.com/srunal/srunal-go/internal/service"
)

type sentMessage struct {
	chatID int64
	text   string
	markup json.RawMessage
}

// fakeSender collects the replies instead of calling the Bot API.
type fakeSender struct {
	mu       sync.Mutex
	messages []sentMessage
	answers  []string
	edits    []string
}

func (f *fakeSender) sendMessage(_ context.Context, chatID int64, text string, markup json.RawMessage) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.messages = append(f.messages, sentMessage{chatID: chatID, text: text, markup: markup})
	return nil
}

func (f *fakeSender) answerCallbackQuery(_ context.Context, _ string, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.answers = append(f.answers, text)
	return nil
}

func (f *fakeSender) editMessageText(_ context.Context, _ int64, _ int64, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.edits = append(f.edits, text)
	return nil
}

func (f *fakeSender) last() sentMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.messages) == 0 {
		return sentMessage{}
	}
	return f.messages[len(f.messages)-1]
}

func (f *fakeSender) lastAnswer() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.answers) == 0 {
		return ""
	}
	return f.answers[len(f.answers)-1]
}

// testBot builds a bot with a fake sender. The store stays nil: these tests
// only exercise commands that answer without touching the database (/id, /help).
func testBot(t *testing.T) (*Bot, *fakeSender) {
	t.Helper()
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	cfg := &config.Config{
		Bot: config.TelegramConfig{Enabled: true, Token: "test", Username: "srunal_bot"},
	}
	bot := New(service.New(nil, cfg, log), cfg, log)
	sender := &fakeSender{}
	bot.sender = sender
	return bot, sender
}

// TestChatIDCommand checks the /id answer an operator needs for
// TELEGRAM_ADMIN_CHAT_IDS.
func TestChatIDCommand(t *testing.T) {
	bot, sender := testBot(t)

	for _, command := range []string{"/id", "/chatid", "/myid", "/ID"} {
		bot.handleMessage(context.Background(), Message{
			Chat: Chat{ID: 123456789, Type: "private"},
			From: &User{ID: 123456789, Username: "operator_almaty"},
			Text: command,
		})
		reply := sender.last().text
		if !strings.Contains(reply, "123456789") {
			t.Errorf("%s: chat id missing from %q", command, reply)
		}
		if !strings.Contains(reply, "TELEGRAM_ADMIN_CHAT_IDS=123456789") {
			t.Errorf("%s: env hint missing from %q", command, reply)
		}
		if sender.last().chatID != 123456789 {
			t.Errorf("%s: reply sent to chat %d", command, sender.last().chatID)
		}
	}
}

// TestChatIDInGroup checks the negative group ids the service also accepts.
func TestChatIDInGroup(t *testing.T) {
	bot, sender := testBot(t)
	bot.handleMessage(context.Background(), Message{
		Chat: Chat{ID: -1001234567890, Type: "supergroup"},
		From: &User{ID: 987654321, FirstName: "Ануар"},
		Text: "/id@srunal_bot",
	})
	reply := sender.last().text
	if !strings.Contains(reply, "-1001234567890") {
		t.Fatalf("group id missing from %q", reply)
	}
	if !strings.Contains(reply, "987654321") {
		t.Errorf("personal id missing from %q", reply)
	}
}

// TestHelpAndUnknownCommand checks the fallback replies and that the operator
// command list is not advertised in the public help.
//
// The public help follows design.md §8.2 template 1: it names the service and
// the university, points the client into the Mini App and mentions only the
// client commands. The website login code (/login) left the client flow with
// the Telegram-first migration; the operator panel still uses it, so the
// operator help keeps it out of the public list on purpose.
func TestHelpAndUnknownCommand(t *testing.T) {
	bot, sender := testBot(t)

	bot.handleMessage(context.Background(), Message{Chat: Chat{ID: 1}, Text: "/help"})
	help := sender.last().text
	for _, needle := range []string{"Satbayev", "/status", "/help"} {
		if !strings.Contains(help, needle) {
			t.Errorf("help text misses %s", needle)
		}
	}
	for _, hidden := range []string{"/id", "/confirm", "/2fa", "/bind", "/orders", "/pause"} {
		if strings.Contains(help, hidden) {
			t.Errorf("public help leaks %q: %q", hidden, help)
		}
	}

	// /help op is operator-only: an unbound chat gets a refusal.
	bot.handleMessage(context.Background(), Message{Chat: Chat{ID: 1}, Text: "/help op"})
	if !strings.Contains(sender.last().text, "только операторам") {
		t.Errorf("/help op reply = %q", sender.last().text)
	}

	bot.handleMessage(context.Background(), Message{Chat: Chat{ID: 1}, Text: "/nonsense"})
	if !strings.Contains(sender.last().text, "Не понял команду") {
		t.Errorf("unknown command reply = %q", sender.last().text)
	}
}

// TestBindHintShowsRealHandle checks the /bind instruction an unbound operator
// sees: it must name a real handle. The old literal "@operator" placeholder was
// copied as a login, so every /bind failed with «неверный логин или пароль».
func TestBindHintShowsRealHandle(t *testing.T) {
	bot, sender := testBot(t)
	const hint = "/bind @operator_almaty пароль"

	replies := []string{}
	for _, command := range []string{"/help op", "/orders", "/stats"} {
		bot.handleMessage(context.Background(), Message{Chat: Chat{ID: 1}, Text: command})
		replies = append(replies, sender.last().text)
	}
	for _, reply := range replies {
		if !strings.Contains(reply, hint) {
			t.Errorf("reply %q misses %q", reply, hint)
		}
		if strings.Contains(reply, "/bind @operator пароль") {
			t.Errorf("reply %q still prints the bare placeholder", reply)
		}
	}
}

// TestOperatorHelpForAdminChat checks the positive path of /help op: a chat
// listed in TELEGRAM_ADMIN_CHAT_IDS is trusted without /bind.
func TestOperatorHelpForAdminChat(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	cfg := &config.Config{
		Bot: config.TelegramConfig{Enabled: true, Token: "test", Username: "srunal_bot", AdminChatIDs: []int64{42}},
	}
	bot := New(service.New(nil, cfg, log), cfg, log)
	sender := &fakeSender{}
	bot.sender = sender

	bot.handleMessage(context.Background(), Message{Chat: Chat{ID: 42}, Text: "/help op"})
	reply := sender.last().text
	for _, needle := range []string{"/orders", "/confirm", "/stats", "/pause"} {
		if !strings.Contains(reply, needle) {
			t.Errorf("/help op misses %s: %q", needle, reply)
		}
	}
}

// TestCallbackNeedsOperator checks that the inline payment buttons only work
// for operator chats and that unknown payloads are answered politely.
func TestCallbackNeedsOperator(t *testing.T) {
	bot, sender := testBot(t)

	// A stranger presses a button: no action, just a toast.
	bot.handleCallback(context.Background(), CallbackQuery{
		ID:      "cb-1",
		Data:    "confirm:DG-260391",
		Message: &Message{MessageID: 10, Chat: Chat{ID: 999}},
	})
	if !strings.Contains(sender.lastAnswer(), "только оператор") {
		t.Errorf("answer = %q", sender.lastAnswer())
	}
	if len(sender.edits) != 0 {
		t.Fatalf("the message was edited for a non-operator: %v", sender.edits)
	}

	// A malformed payload never reaches the service.
	bot.handleCallback(context.Background(), CallbackQuery{
		ID:      "cb-2",
		Data:    "garbage",
		Message: &Message{MessageID: 11, Chat: Chat{ID: 999}},
	})
	if !strings.Contains(sender.lastAnswer(), "Не понял действие") {
		t.Errorf("answer = %q", sender.lastAnswer())
	}

	// A button without a message body is handled gracefully.
	bot.handleCallback(context.Background(), CallbackQuery{ID: "cb-3", Data: "reject:DG-260391"})
	if !strings.Contains(sender.lastAnswer(), "только оператор") {
		t.Errorf("answer = %q", sender.lastAnswer())
	}
}

// TestCallbackButtonsParsing checks the payloads the payment notification carries.
func TestCallbackButtonsParsing(t *testing.T) {
	markup := service.ClaimedPaymentButtons("DG-260391")
	if len(markup) == 0 {
		t.Fatal("no markup rendered")
	}
	var parsed struct {
		InlineKeyboard [][]struct {
			Text string `json:"text"`
			Data string `json:"callback_data"`
		} `json:"inline_keyboard"`
	}
	if err := json.Unmarshal(markup, &parsed); err != nil {
		t.Fatalf("markup is not valid JSON: %v", err)
	}
	if len(parsed.InlineKeyboard) != 1 || len(parsed.InlineKeyboard[0]) != 2 {
		t.Fatalf("expected one row with two buttons, got %+v", parsed.InlineKeyboard)
	}
	if parsed.InlineKeyboard[0][0].Data != "confirm:DG-260391" {
		t.Errorf("confirm payload = %q", parsed.InlineKeyboard[0][0].Data)
	}
	if parsed.InlineKeyboard[0][1].Data != "reject:DG-260391" {
		t.Errorf("reject payload = %q", parsed.InlineKeyboard[0][1].Data)
	}
	for _, button := range parsed.InlineKeyboard[0] {
		if len(button.Data) > 64 {
			t.Errorf("callback_data exceeds the Telegram limit: %q", button.Data)
		}
	}
}

func TestEmptyMessageIsIgnored(t *testing.T) {
	bot, sender := testBot(t)
	bot.handleMessage(context.Background(), Message{Chat: Chat{ID: 1}, Text: "   "})
	if len(sender.messages) != 0 {
		t.Fatalf("expected no replies, got %d", len(sender.messages))
	}
}
