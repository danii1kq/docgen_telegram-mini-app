package store_test

import (
	"strings"
	"testing"

	"github.com/srunal/srunal-go/internal/seed"
)

// TestClientLoginAndAutoAttach walks the client authorization: the site asks
// for a code, the bot confirms it for a chat, the poll returns a client session
// and an order created with that session is attached to the same chat.
func TestClientLoginAndAutoAttach(t *testing.T) {
	svc, ctx := testService(t)
	seedDemo(t, svc, ctx)

	start, err := svc.StartClientLogin(ctx)
	if err != nil {
		t.Fatalf("StartClientLogin: %v", err)
	}
	if len(start.Code) != 6 || start.BotURL == "" {
		t.Fatalf("unexpected start payload: %+v", start)
	}

	// The bot receives /login <code> from the client's chat.
	const chatID = int64(555000111)
	if err := svc.Store.ConfirmLoginChallenge(ctx, start.Code, chatID, "@client_kz"); err != nil {
		t.Fatalf("ConfirmLoginChallenge: %v", err)
	}

	result, pending, err := svc.PollClientLogin(ctx, start.Code, "10.0.0.1", "test")
	if err != nil || pending {
		t.Fatalf("PollClientLogin: pending=%v err=%v", pending, err)
	}
	if result.Token == "" || result.Session == nil || result.Session.ChatID != chatID {
		t.Fatalf("no client session: %+v", result)
	}
	// The chat is not bound to an operator, so no panel session is issued.
	if result.Operator != nil {
		t.Fatalf("unexpected operator session: %+v", result.Operator)
	}

	// The bearer token resolves to the session (what the API middleware does).
	session, err := svc.ClientSession(ctx, result.Token)
	if err != nil || session.ChatID != chatID || session.Username != "@client_kz" {
		t.Fatalf("ClientSession: %+v (%v)", session, err)
	}

	// An order created with this session is attached to the chat automatically.
	created, err := svc.CreateOrder(ctx, form(""), "client-order-key", &session)
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	stored, err := svc.Store.GetOrder(ctx, created.Number)
	if err != nil {
		t.Fatalf("GetOrder: %v", err)
	}
	if stored.TelegramChatID != chatID {
		t.Fatalf("order chat = %d, want %d", stored.TelegramChatID, chatID)
	}
	if stored.Contact != "@client_kz" {
		t.Fatalf("order contact = %q, want @client_kz", stored.Contact)
	}

	// A second order from the same chat is no longer a "first order".
	if _, err := svc.Store.CountOrdersByChat(ctx, chatID); err != nil {
		t.Fatalf("CountOrdersByChat: %v", err)
	}
	count, err := svc.Store.CountOrdersByChat(ctx, chatID)
	if err != nil || count != 1 {
		t.Fatalf("orders attached to chat = %d (%v), want 1", count, err)
	}

	// The code is single-use.
	if _, pending, err := svc.PollClientLogin(ctx, start.Code, "10.0.0.1", "test"); err == nil || pending {
		t.Fatalf("replayed code: pending=%v err=%v", pending, err)
	} else if !strings.Contains(err.Error(), "использован") {
		t.Fatalf("unexpected replay error: %v", err)
	}

	// Logging out revokes the token.
	if err := svc.ClientLogout(ctx, result.Token); err != nil {
		t.Fatalf("ClientLogout: %v", err)
	}
	if _, err := svc.ClientSession(ctx, result.Token); err == nil {
		t.Fatal("the session still resolves after logout")
	}
}

// TestClientLoginAlsoOpensPanelForOperatorChat checks that an operator logging
// in on the site gets both sessions from one bot confirmation.
func TestClientLoginAlsoOpensPanelForOperatorChat(t *testing.T) {
	svc, ctx := testService(t)
	seedDemo(t, svc, ctx)

	operatorChat := int64(100200300)
	if err := svc.Store.BindOperatorChat(ctx, seed.DemoOperatorID, &operatorChat); err != nil {
		t.Fatalf("bind chat: %v", err)
	}

	start, err := svc.StartClientLogin(ctx)
	if err != nil {
		t.Fatalf("StartClientLogin: %v", err)
	}
	if err := svc.Store.ConfirmLoginChallenge(ctx, start.Code, operatorChat, "@operator_almaty"); err != nil {
		t.Fatalf("ConfirmLoginChallenge: %v", err)
	}
	result, pending, err := svc.PollClientLogin(ctx, start.Code, "10.0.0.1", "test")
	if err != nil || pending {
		t.Fatalf("PollClientLogin: pending=%v err=%v", pending, err)
	}
	if result.Session == nil || result.Session.ChatID != operatorChat {
		t.Fatalf("client session missing: %+v", result)
	}
	if result.Operator == nil || result.Operator.Session == nil {
		t.Fatalf("operator session missing: %+v", result)
	}
	if result.Operator.Session.OperatorID != seed.DemoOperatorID {
		t.Fatalf("operator = %q", result.Operator.Session.OperatorID)
	}
	if _, err := svc.AdminSession(ctx, result.Operator.Token); err != nil {
		t.Fatalf("AdminSession: %v", err)
	}
}

// TestClientLoginKindIsolation makes sure a code from one login page cannot be
// exchanged on the other.
func TestClientLoginKindIsolation(t *testing.T) {
	svc, ctx := testService(t)
	seedDemo(t, svc, ctx)

	clientStart, err := svc.StartClientLogin(ctx)
	if err != nil {
		t.Fatalf("StartClientLogin: %v", err)
	}
	if _, _, err := svc.PollTelegramLogin(ctx, clientStart.Code, "10.0.0.1", "test"); err == nil {
		t.Fatal("a client code was accepted by the operator poll")
	}

	operatorStart, err := svc.StartTelegramLogin(ctx, "")
	if err != nil {
		t.Fatalf("StartTelegramLogin: %v", err)
	}
	if _, _, err := svc.PollClientLogin(ctx, operatorStart.Code, "10.0.0.1", "test"); err == nil {
		t.Fatal("an operator code was accepted by the client poll")
	}
}
