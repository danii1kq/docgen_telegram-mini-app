package store_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/srunal/srunal-go/internal/config"
	"github.com/srunal/srunal-go/internal/domain"
	"github.com/srunal/srunal-go/internal/seed"
	"github.com/srunal/srunal-go/internal/service"
	"github.com/srunal/srunal-go/internal/store"
	"github.com/srunal/srunal-go/internal/validate"
)

// These tests need a real PostgreSQL database and are skipped without it:
//
//	createdb srunal_test
//	$env:TEST_DATABASE_URL="postgres://srunal:srunal@127.0.0.1:5432/srunal_test?sslmode=disable"
//	go test ./internal/store/...
//
// The database is truncated before every test; never point this at production.
func testService(t *testing.T) (*service.Service, context.Context) {
	t.Helper()

	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set: skipping database integration tests")
	}

	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	st, err := store.Open(ctx, url, log)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(st.Close)
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Fresh state: truncate everything the tests touch.
	if _, err := st.Pool().Exec(ctx, `truncate order_events, order_files, order_jobs, orders,
		reviews, promo_code_usages, promo_codes, promo_code_products, product_pricing,
		volume_discounts, idempotency_keys, counters, telegram_outbox, app_settings,
		operators, admin_sessions, login_attempts, telegram_login_challenges, app_kv cascade`); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	cfg := &config.Config{
		Env:              "test",
		TokenPepper:      "test-pepper",
		SessionTTL:       time.Hour,
		ClientSessionTTL: 24 * time.Hour,
		StorageDir:       t.TempDir(),
		ResultTTL:        24 * time.Hour,
		SeedOnStart:      false,
		Bot: config.TelegramConfig{
			Enabled:  true,
			Username: "srunal_bot",
		},
		Worker: config.WorkerConfig{
			GenerationMin: time.Millisecond,
			GenerationMax: time.Millisecond,
			ExpireAfter:   24 * time.Hour,
		},
	}
	return service.New(st, cfg, log), ctx
}

// form builds a valid order payload.
func form(promoCode string) validate.OrderForm {
	form := validate.OrderForm{
		Product: "essay", Works: 1, Units: 10,
		Topics:   "Интеграционный тест: сквозной сценарий заказа",
		Language: "ru", Institute: "ИЦИТ", Department: "Программная инженерия",
		StudentName: "Тест Тестович", DisciplineCode: "CS-101", TeacherName: "Доц. Тестов Т.Т.",
		ContactName: "Тест Тестович", Consent: true,
	}
	if promoCode != "" {
		form.PromoCode = &promoCode
	}
	return form
}

// seedDemo imports the demo data (prices, promos, settings, operator) into an
// empty database, exactly like the service does at startup.
func seedDemo(t *testing.T, svc *service.Service, ctx context.Context) {
	t.Helper()
	if err := seed.Import(ctx, svc.Store, svc.Cfg.TokenPepper, svc.Log); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

// TestOrderLifecycle walks awaiting_payment -> payment_claimed -> queued ->
// generating -> completed and checks the frozen price, the promo usage and the
// generated document row.
func TestOrderLifecycle(t *testing.T) {
	svc, ctx := testService(t)
	seedDemo(t, svc, ctx)

	// The demo promo codes define "first order" through the database.
	created, err := svc.CreateOrder(ctx, form("SRUNAL10"), "key-lifecycle", nil)
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	if created.Amount != 720 {
		t.Fatalf("amount = %d, want 720", created.Amount)
	}
	if !strings.HasPrefix(created.Number, "DG-") {
		t.Fatalf("unexpected number %s", created.Number)
	}
	if created.Token == "" || len(created.Token) != 6 {
		t.Fatalf("token missing: %q", created.Token)
	}
	if created.PromoCode == nil || *created.PromoCode != "SRUNAL10" {
		t.Fatalf("promo not recorded: %v", created.PromoCode)
	}

	// This test drives its own order through the worker FIFO, so drop the demo
	// orders (the seed ships one `queued` record). Prices, promos and settings
	// stay, because the order already carries the seeded promo code.
	if _, err := svc.Store.Pool().Exec(ctx,
		`delete from reviews where order_number <> $1`, created.Number); err != nil {
		t.Fatalf("clear demo reviews: %v", err)
	}
	if _, err := svc.Store.Pool().Exec(ctx,
		`delete from orders where number <> $1`, created.Number); err != nil {
		t.Fatalf("clear demo orders: %v", err)
	}

	// Idempotency replay returns the same order and token.
	replay, err := svc.CreateOrder(ctx, form("SRUNAL10"), "key-lifecycle", nil)
	if err != nil {
		t.Fatalf("idempotent CreateOrder: %v", err)
	}
	if replay.Number != created.Number || replay.Token != created.Token {
		t.Fatalf("idempotency mismatch: %s/%s vs %s/%s",
			replay.Number, replay.Token, created.Number, created.Token)
	}

	// The promo usage counter grew exactly once.
	promo, err := svc.Store.GetPromoByCode(ctx, "SRUNAL10")
	if err != nil {
		t.Fatalf("GetPromoByCode: %v", err)
	}
	if promo.UsedCount != 8 { // seeded 7
		t.Fatalf("usedCount = %d, want 8", promo.UsedCount)
	}

	// A wrong token pair must not leak the order.
	if _, err := svc.GetOrderByToken(ctx, created.Number, "AAAAAA"); err == nil {
		t.Fatal("wrong token accepted")
	}

	// The client linked the chat, so status pushes must reach the outbox.
	if err := svc.Store.LinkOrderChat(ctx, created.Number, 777001, "@tester"); err != nil {
		t.Fatalf("LinkOrderChat: %v", err)
	}

	// Claim -> queued -> generation.
	claimed, err := svc.ClaimPayment(ctx, created.Number, created.Token, "Тест Т.")
	if err != nil {
		t.Fatalf("ClaimPayment: %v", err)
	}
	if claimed.Status != domain.StatusPaymentClaimed || claimed.SenderName != "Тест Т." {
		t.Fatalf("claim result: %s/%s", claimed.Status, claimed.SenderName)
	}
	if _, err := svc.ClaimPayment(ctx, created.Number, created.Token, "Тест Т."); err == nil {
		t.Fatal("double claim accepted")
	}

	confirmed, err := svc.ConfirmPayment(ctx, created.Number, "@tester")
	if err != nil {
		t.Fatalf("ConfirmPayment: %v", err)
	}
	if confirmed.Status != domain.StatusQueued || confirmed.ConfirmedAt == nil {
		t.Fatalf("confirm result: %s", confirmed.Status)
	}
	if _, err := svc.ConfirmPayment(ctx, created.Number, "@tester"); err == nil {
		t.Fatal("double confirm accepted")
	}

	// Notifications are queued in the outbox (delivered by the bot loop).
	var outbox int
	if err := svc.Store.Pool().QueryRow(ctx,
		`select count(*) from telegram_outbox where chat_id = 777001`).Scan(&outbox); err != nil {
		t.Fatalf("outbox count: %v", err)
	}
	if outbox < 2 {
		t.Fatalf("expected client notifications in the outbox, got %d", outbox)
	}

	// The worker takes the order, then completes it with a document.
	order, jobID, err := svc.Store.TakeNextQueued(ctx)
	if err != nil {
		t.Fatalf("TakeNextQueued: %v", err)
	}
	if order.Number != created.Number || jobID == 0 {
		t.Fatalf("worker took %s (job %d)", order.Number, jobID)
	}
	expires := time.Now().UTC().Add(24 * time.Hour)
	completed, err := svc.Store.CompleteGeneration(ctx, order.Number, []store.OrderFileRow{{
		Number: order.Number, Kind: "result", FileName: "resultat.docx", SizeBytes: 2048,
		MimeType:   "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		StorageKey: "results/" + order.Number + ".docx", ExpiresAt: &expires,
	}}, "ok")
	if err != nil {
		t.Fatalf("CompleteGeneration: %v", err)
	}
	if completed.Status != domain.StatusCompleted || completed.CompletedAt == nil {
		t.Fatalf("completion result: %s", completed.Status)
	}
	if completed.ResultFile == nil || completed.ResultFile.Name != "resultat.docx" {
		t.Fatalf("result file missing: %+v", completed.ResultFile)
	}
	if len(completed.Events) < 5 {
		t.Fatalf("timeline has %d entries, want >= 5", len(completed.Events))
	}
	if completed.Events[0].Text != "Результат готов и доступен для скачивания" {
		t.Fatalf("newest event = %q", completed.Events[0].Text)
	}
	if _, _, err := svc.Store.TakeNextQueued(ctx); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("queue should be empty, got %v", err)
	}
}

// TestPublicOrderIsRedacted checks that the public status page hides the
// operator's notes and the client's contact details.
func TestPublicOrderIsRedacted(t *testing.T) {
	svc, ctx := testService(t)
	seedDemo(t, svc, ctx)

	created, err := svc.CreateOrder(ctx, form(""), "", nil)
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	public, err := svc.GetOrderPublic(ctx, created.Number)
	if err != nil {
		t.Fatalf("GetOrderPublic: %v", err)
	}
	redacted := public.Redacted()
	if redacted.Contact != "" || redacted.ClientNote != "" || redacted.OperatorNotes != nil || redacted.SenderName != "" {
		t.Fatalf("private fields leaked: %+v", redacted)
	}
	if redacted.Number != created.Number || redacted.Amount != 800 {
		t.Fatalf("public order lost data: %+v", redacted)
	}
}

// TestValidationAndMaintenance covers the 422/503 paths of the order endpoint.
func TestValidationAndMaintenance(t *testing.T) {
	svc, ctx := testService(t)
	seedDemo(t, svc, ctx)

	bad := form("")
	bad.Topics = "ab"
	bad.Consent = false
	if _, err := svc.CreateOrder(ctx, bad, "", nil); err == nil {
		t.Fatal("invalid form accepted")
	} else {
		var apiError *service.APIError
		if !errors.As(err, &apiError) || apiError.Status != 422 {
			t.Fatalf("expected 422, got %v", err)
		}
		if apiError.Fields["topics"] == "" || apiError.Fields["consent"] == "" {
			t.Fatalf("fields missing: %v", apiError.Fields)
		}
	}

	settings, err := svc.GetSettings(ctx)
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	settings.Maintenance = true
	if _, err := svc.SaveSettings(ctx, settings); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}
	if _, err := svc.CreateOrder(ctx, form(""), "", nil); err == nil {
		t.Fatal("order accepted during maintenance")
	} else {
		var apiError *service.APIError
		if !errors.As(err, &apiError) || apiError.Code != service.CodeMaintenance {
			t.Fatalf("expected maintenance, got %v", err)
		}
	}
}

// TestTelegramLogin walks the server side of the QR login: the page starts a
// challenge, the bot (/login) confirms it for a bound chat, the page polls and
// receives a session; a second poll is refused.
func TestTelegramLogin(t *testing.T) {
	svc, ctx := testService(t)
	seedDemo(t, svc, ctx)

	// Bind a chat to the demo operator, as /bind does.
	chatID := int64(100200300)
	if err := svc.Store.BindOperatorChat(ctx, seed.DemoOperatorID, &chatID); err != nil {
		t.Fatalf("bind chat: %v", err)
	}

	start, err := svc.StartTelegramLogin(ctx, "")
	if err != nil {
		t.Fatalf("StartTelegramLogin: %v", err)
	}
	if len(start.Code) != 6 {
		t.Fatalf("code = %q, want 6 digits", start.Code)
	}

	// Before the bot confirms: still pending.
	result, pending, err := svc.PollTelegramLogin(ctx, start.Code, "10.0.0.1", "test")
	if err != nil || !pending {
		t.Fatalf("unconfirmed poll: pending=%v err=%v", pending, err)
	}
	if result.Session != nil {
		t.Fatal("session issued before the bot confirmed the code")
	}

	// The bot confirms with the operator's chat.
	if err := svc.Store.ConfirmLoginChallenge(ctx, start.Code, chatID, "@operator_almaty"); err != nil {
		t.Fatalf("ConfirmLoginChallenge: %v", err)
	}

	result, pending, err = svc.PollTelegramLogin(ctx, start.Code, "10.0.0.1", "test")
	if err != nil {
		t.Fatalf("confirmed poll: %v", err)
	}
	if pending {
		t.Fatal("poll still pending after the confirmation")
	}
	if result.Session == nil || result.Token == "" {
		t.Fatalf("no session issued: %+v", result)
	}
	if result.Session.OperatorID != seed.DemoOperatorID {
		t.Fatalf("session operator = %q, want %q", result.Session.OperatorID, seed.DemoOperatorID)
	}

	// The challenge is single-use and the session resolves through the token.
	if _, pending, err := svc.PollTelegramLogin(ctx, start.Code, "10.0.0.1", "test"); err == nil || pending {
		t.Fatalf("the same login code was accepted twice (pending=%v err=%v)", pending, err)
	} else if !strings.Contains(err.Error(), "использован") {
		t.Fatalf("unexpected replay error: %v", err)
	}
	session, err := svc.AdminSession(ctx, result.Token)
	if err != nil || session.OperatorID != seed.DemoOperatorID {
		t.Fatalf("session lookup: %+v (%v)", session, err)
	}
}

// TestTelegramLoginNeedsBinding checks the message an unbound chat gets.
func TestTelegramLoginNeedsBinding(t *testing.T) {
	svc, ctx := testService(t)
	seedDemo(t, svc, ctx)

	start, err := svc.StartTelegramLogin(ctx, "")
	if err != nil {
		t.Fatalf("StartTelegramLogin: %v", err)
	}
	// An unknown chat confirms the code: the poll must refuse with the hint.
	if err := svc.Store.ConfirmLoginChallenge(ctx, start.Code, 424242, ""); err != nil {
		t.Fatalf("ConfirmLoginChallenge: %v", err)
	}
	_, pending, err := svc.PollTelegramLogin(ctx, start.Code, "10.0.0.1", "test")
	if pending || err == nil {
		t.Fatalf("unbound chat: pending=%v err=%v, want an error", pending, err)
	}
	if !strings.Contains(err.Error(), "не привязан") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestTelegramLoginOperatorPrefilled checks the /start login_<code> deep link:
// when the login page sent the handle, no chat binding is needed.
func TestTelegramLoginOperatorPrefilled(t *testing.T) {
	svc, ctx := testService(t)
	seedDemo(t, svc, ctx)

	start, err := svc.StartTelegramLogin(ctx, "@operator_almaty")
	if err != nil {
		t.Fatalf("StartTelegramLogin: %v", err)
	}
	if err := svc.Store.ConfirmLoginChallenge(ctx, start.Code, 777001, ""); err != nil {
		t.Fatalf("ConfirmLoginChallenge: %v", err)
	}
	result, pending, err := svc.PollTelegramLogin(ctx, start.Code, "10.0.0.1", "test")
	if err != nil || pending || result.Session == nil {
		t.Fatalf("prefilled poll: pending=%v err=%v result=%+v", pending, err, result)
	}
}

// TestOrderWorkKind checks that the "Другой док." kind is stored and validated.
func TestOrderWorkKind(t *testing.T) {
	svc, ctx := testService(t)
	seedDemo(t, svc, ctx)

	other := form("")
	other.Product = domain.ProductOther
	other.WorkKind = "СРС"
	other.Units = 1
	created, err := svc.CreateOrder(ctx, other, "", nil)
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	stored, err := svc.Store.GetOrder(ctx, created.Number)
	if err != nil {
		t.Fatalf("GetOrder: %v", err)
	}
	if stored.WorkKind != "СРС" {
		t.Fatalf("work kind = %q, want СРС", stored.WorkKind)
	}

	// An empty kind is rejected for that product.
	empty := form("")
	empty.Product = domain.ProductOther
	empty.WorkKind = "   "
	if _, err := svc.CreateOrder(ctx, empty, "", nil); err == nil {
		t.Fatal("an empty work kind was accepted for 'other'")
	}

	// For the other products the field is dropped, never stored.
	essay := form("")
	essay.WorkKind = "СРС"
	essayOrder, err := svc.CreateOrder(ctx, essay, "", nil)
	if err != nil {
		t.Fatalf("CreateOrder (essay): %v", err)
	}
	if essayOrder.WorkKind != "" {
		t.Fatalf("essay order kept the work kind: %q", essayOrder.WorkKind)
	}
}

// TestBindOperatorChatCredentials walks /bind end to end: the placeholder handle
// the old bot hint printed ("@operator") and a wrong password are refused with a
// message that names the expected login, while the real handle binds the chat.
func TestBindOperatorChatCredentials(t *testing.T) {
	svc, ctx := testService(t)
	seedDemo(t, svc, ctx)

	chatID := int64(555001)

	// The literal "@operator" from the old hint must not bind.
	err := svc.BindOperatorChat(ctx, "@operator", seed.DemoOperatorPassword, chatID)
	var apiErr *service.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("placeholder handle: err = %v, want an APIError", err)
	}
	if apiErr.Message != service.BindWrongCredentials {
		t.Fatalf("message = %q, want the bind answer", apiErr.Message)
	}
	if strings.Contains(strings.ToLower(apiErr.Message), "email") {
		t.Errorf("the bind answer still mentions email: %q", apiErr.Message)
	}
	if !strings.Contains(apiErr.Message, seed.DemoOperatorID) {
		t.Errorf("the bind answer should show the expected handle: %q", apiErr.Message)
	}

	// A wrong password is refused the same way.
	if err := svc.BindOperatorChat(ctx, seed.DemoOperatorID, "not-the-password", chatID); err == nil {
		t.Fatal("a wrong password bound the chat")
	}

	// The real handle and password bind the chat, and it resolves as the operator.
	if err := svc.BindOperatorChat(ctx, seed.DemoOperatorID, seed.DemoOperatorPassword, chatID); err != nil {
		t.Fatalf("bind with the right credentials: %v", err)
	}
	operator, err := svc.Store.OperatorByChatID(ctx, chatID)
	if err != nil || operator.ID != seed.DemoOperatorID {
		t.Fatalf("OperatorByChatID = %q, %v", operator.ID, err)
	}
}

// TestPromoRules checks the promo messages against the database content.
func TestPromoRules(t *testing.T) {
	svc, ctx := testService(t)
	seedDemo(t, svc, ctx)

	cases := []struct {
		code   string
		req    service.PromoCheckRequest
		reason string
	}{
		{"", service.PromoCheckRequest{Product: "essay", Units: 10}, "Введите промокод"},
		{"NOPE", service.PromoCheckRequest{Product: "essay", Units: 10}, "Промокод не найден"},
		{"OLDFIVE", service.PromoCheckRequest{Product: "essay", Units: 10}, "Промокод отключён"},
		{"SPRING25", service.PromoCheckRequest{Product: "essay", Units: 10}, "Срок действия промокода истёк"},
		{"FIRST15", service.PromoCheckRequest{Product: "essay", Units: 10, IsFirstOrder: false}, "Промокод действует только на первый заказ"},
		{"LABHIT10", service.PromoCheckRequest{Product: "essay", Units: 10, Subtotal: intPtr(800)}, "Промокод действует только на «Лабораторная»"},
		{"WINTER20", service.PromoCheckRequest{Product: "essay", Units: 3, Subtotal: intPtr(240)}, "Промокод действует от 5 страниц"},
	}
	for _, testCase := range cases {
		request := testCase.req
		request.Code = testCase.code
		result, err := svc.CheckPromo(ctx, request)
		if err != nil {
			t.Fatalf("CheckPromo(%s): %v", testCase.code, err)
		}
		if result.OK || result.Reason != testCase.reason {
			t.Errorf("%s: got ok=%v reason=%q, want %q", testCase.code, result.OK, result.Reason, testCase.reason)
		}
	}

	ok, err := svc.CheckPromo(ctx, service.PromoCheckRequest{
		Code: "WINTER20", Product: "essay", Units: 10, Subtotal: intPtr(800),
	})
	if err != nil || !ok.OK || ok.Promo == nil || ok.Promo.DiscountPct != 20 {
		t.Fatalf("valid promo rejected: %+v (%v)", ok, err)
	}
}

func intPtr(value int) *int { return &value }

// TestMiniAppChatIdentity walks the Mini App flow where the Telegram chat is
// the identity: an order created with a chat session is listed by /api/me/orders,
// is readable and claimable by that chat without the order token, and a foreign
// chat gets the same neutral not-found as a wrong number.
func TestMiniAppChatIdentity(t *testing.T) {
	svc, ctx := testService(t)
	seedDemo(t, svc, ctx)

	chatID := int64(555000111)
	client := &store.ClientSession{ChatID: chatID, Username: "anuarbek"}
	created, err := svc.CreateOrder(ctx, form(""), "key-mini-app", client)
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}

	// The chat's list carries the order and the status counters.
	mine, err := svc.MyOrders(ctx, chatID)
	if err != nil {
		t.Fatalf("MyOrders: %v", err)
	}
	if len(mine.Orders) != 1 || mine.Orders[0].Number != created.Number {
		t.Fatalf("MyOrders = %+v, want the created order", mine.Orders)
	}
	if mine.Counts[domain.StatusAwaitingPayment] != 1 {
		t.Fatalf("counts = %v, want one awaiting_payment", mine.Counts)
	}

	// The full card is readable by the owning chat and hidden from others.
	if own, err := svc.OwnOrder(ctx, created.Number, chatID); err != nil || own.Number != created.Number {
		t.Fatalf("OwnOrder(own chat) = %+v (%v)", own, err)
	}
	foreign := int64(999000222)
	if _, err := svc.OwnOrder(ctx, created.Number, foreign); err == nil {
		t.Fatal("a foreign chat read the full order")
	} else if apiError := (*service.APIError)(nil); !errors.As(err, &apiError) || apiError.Status != 404 {
		t.Fatalf("foreign order read must answer with the neutral 404, got %v", err)
	}
	if other, err := svc.MyOrders(ctx, foreign); err != nil || len(other.Orders) != 0 {
		t.Fatalf("foreign list = %+v (%v), want empty", other.Orders, err)
	}

	// «Я оплатил» without a token: a foreign chat is refused with the same 404,
	// the owning chat moves the order to payment_claimed.
	if _, err := svc.ClaimPaymentAsChat(ctx, created.Number, foreign, "Ануар А."); err == nil {
		t.Fatal("a foreign chat claimed the payment")
	}
	claimed, err := svc.ClaimPaymentAsChat(ctx, created.Number, chatID, "Ануар А.")
	if err != nil {
		t.Fatalf("ClaimPaymentAsChat: %v", err)
	}
	if claimed.Status != domain.StatusPaymentClaimed || claimed.SenderName != "Ануар А." {
		t.Fatalf("claimed order = %s / %q", claimed.Status, claimed.SenderName)
	}
	if money := svc.ResultSignature(created.Number); len(money) != 32 {
		t.Fatalf("result signature = %q, want 32 hex chars", money)
	}
}
