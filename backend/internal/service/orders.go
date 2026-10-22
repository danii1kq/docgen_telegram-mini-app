package service

import (
	"context"
	"errors"
	"strings"

	"github.com/srunal/srunal-go/internal/auth"
	"github.com/srunal/srunal-go/internal/domain"
	"github.com/srunal/srunal-go/internal/pricing"
	"github.com/srunal/srunal-go/internal/store"
	"github.com/srunal/srunal-go/internal/validate"
)

// CreateOrderResult is the order plus the promo-drop warning (an extension
// over types.ts that the frontend may ignore).
type CreateOrderResult struct {
	domain.Order
	PromoDropReason string `json:"promoDropReason,omitempty"`
}

// CreateOrder is POST /api/orders: validate, price server-side, allocate the
// number and the access token, persist and notify. When the caller passed a
// client session (Telegram login), the order is attached to that chat, so the
// status notifications reach the client without any manual linking.
func (s *Service) CreateOrder(ctx context.Context, form validate.OrderForm, idempotencyKey string, client *store.ClientSession) (*CreateOrderResult, error) {
	settings, err := s.Store.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	if settings.Maintenance {
		return nil, Maintenance()
	}

	if fields := validate.Form(&form); len(fields) > 0 {
		return nil, ValidationError(fields)
	}

	config, err := s.Store.GetPricing(ctx)
	if err != nil {
		return nil, err
	}

	// 1) A quote without the promo, to learn the subtotal for the promo rules.
	baseQuote := pricing.Calc(pricing.Input{
		Product: form.Product,
		Works:   form.Works,
		Units:   form.Units,
	}, config, nil)

	// 2) The promo. A logged-in client is identified by their Telegram chat
	//    (exact history); otherwise the contact name stands in for it.
	isFirstOrder := false
	if client != nil {
		count, err := s.Store.CountOrdersByChat(ctx, client.ChatID)
		if err != nil {
			return nil, err
		}
		isFirstOrder = count == 0
	} else if form.ContactName != "" {
		if count, err := s.Store.CountOrdersByContact(ctx, form.ContactName); err == nil {
			isFirstOrder = count == 0
		}
	}
	dropReason := ""
	var appliedPromo *pricing.AppliedPromo
	var promoID *string
	if form.PromoCode != nil {
		resolved, reason := s.resolvePromo(ctx, *form.PromoCode, baseQuote, isFirstOrder)
		if resolved != nil {
			appliedPromo = &pricing.AppliedPromo{Code: resolved.Code, Pct: resolved.DiscountPct}
			promoID = &resolved.ID
		} else {
			dropReason = reason
		}
	}

	quote := baseQuote
	if appliedPromo != nil {
		quote = pricing.Calc(pricing.Input{
			Product: form.Product,
			Works:   form.Works,
			Units:   form.Units,
		}, config, appliedPromo)
	}

	token, err := auth.AccessToken()
	if err != nil {
		return nil, err
	}

	// The kind of work is only meaningful for "Другой док.".
	workKind := ""
	if form.Product == domain.ProductOther {
		workKind = strings.TrimSpace(form.WorkKind)
	}

	order := &domain.Order{
		Status:         domain.StatusAwaitingPayment,
		SenderName:     "",
		Product:        form.Product,
		Works:          quote.Works,
		Units:          quote.Units,
		Topics:         strings.TrimSpace(form.Topics),
		Notes:          strings.TrimSpace(form.Notes),
		WorkKind:       workKind,
		Language:       form.Language,
		Institute:      strings.TrimSpace(form.Institute),
		Department:     strings.TrimSpace(form.Department),
		StudentName:    strings.TrimSpace(form.StudentName),
		DisciplineCode: strings.TrimSpace(form.DisciplineCode),
		TeacherName:    strings.TrimSpace(form.TeacherName),
		Attachments:    sanitizeAttachments(form.Attachments),
		ContactName:    strings.TrimSpace(form.ContactName),
		ContactType:    "telegram",
		Contact:        "",
		Amount:         quote.Total,
		QuoteLines:     quote.Lines,
		ClientNote:     "",
	}
	if appliedPromo != nil {
		code := appliedPromo.Code
		pct := appliedPromo.Pct
		order.PromoCode = &code
		order.PromoDiscountPct = &pct
	}

	// Auto-attach to Telegram: the client session's chat receives every status
	// change, so no manual linking (DG-xxx + code) is needed.
	if client != nil {
		order.TelegramChatID = client.ChatID
		order.ContactType = "telegram"
		if order.Contact == "" {
			order.Contact = client.Username
		}
	}

	if err := s.Store.CreateOrder(ctx, order, token, auth.HashToken(s.Cfg.TokenPepper, token), promoID, idempotencyKey); err != nil {
		if errors.Is(err, store.ErrConflict) && idempotencyKey != "" {
			if existing, token, lookupErr := s.Store.FindIdempotentOrder(ctx, idempotencyKey); lookupErr == nil && existing != "" {
				found, loadErr := s.Store.GetOrder(ctx, existing)
				if loadErr != nil {
					return nil, loadErr
				}
				// Replay the original response: same order, same access token.
				found.Token = token
				return &CreateOrderResult{Order: *found}, nil
			}
		}
		return nil, FromStore(err, "Заказ не найден")
	}

	order.Token = token
	s.notifyNewOrder(ctx, order)
	return &CreateOrderResult{Order: *order, PromoDropReason: dropReason}, nil
}

// GetOrderPublic returns the order for the /order/:number status page. The
// caller layer redacts private fields before sending it to a client.
func (s *Service) GetOrderPublic(ctx context.Context, number string) (*domain.Order, error) {
	order, err := s.Store.GetOrder(ctx, normalizeNumber(number))
	if err != nil {
		return nil, FromStore(err, "Заказ не найден. Проверьте номер и ссылку.")
	}
	s.attachResultURL(order)
	return order, nil
}

// GetOrderByToken returns the full order for a matching (number, token) pair.
func (s *Service) GetOrderByToken(ctx context.Context, number, token string) (*domain.Order, error) {
	order, err := s.Store.GetOrderByToken(ctx, normalizeNumber(number), auth.HashToken(s.Cfg.TokenPepper, token))
	if err != nil {
		// Never reveal which half of the pair was wrong (README section 11).
		return nil, FromStore(err, "Заказ не найден. Проверьте номер и ссылку.")
	}
	s.attachResultURL(order)
	return order, nil
}

// TokenPair is one {number, token} pair of the client's browser list.
type TokenPair struct {
	Number string `json:"number"`
	Token  string `json:"token"`
}

// BatchOrders is POST /api/orders/batch for the profile page.
func (s *Service) BatchOrders(ctx context.Context, pairs []TokenPair) ([]domain.Order, error) {
	storePairs := make([]store.TokenPair, 0, len(pairs))
	for _, pair := range pairs {
		storePairs = append(storePairs, store.TokenPair{
			Number:    normalizeNumber(pair.Number),
			TokenHash: auth.HashToken(s.Cfg.TokenPepper, pair.Token),
		})
	}
	orders, err := s.Store.GetOrdersByTokens(ctx, storePairs)
	if err != nil {
		return nil, err
	}
	for i := range orders {
		s.attachResultURL(&orders[i])
	}
	return orders, nil
}

// ClaimPayment is POST /api/orders/{number}/claim («Я оплатил»).
func (s *Service) ClaimPayment(ctx context.Context, number, token, senderName string) (*domain.Order, error) {
	if message := validate.SenderName(senderName); message != "" {
		return nil, ValidationError(map[string]string{"senderName": message})
	}
	order, err := s.Store.ClaimPayment(ctx, normalizeNumber(number),
		auth.HashToken(s.Cfg.TokenPepper, token), strings.TrimSpace(senderName))
	if err != nil {
		return nil, FromStore(err, "Заказ не найден. Проверьте номер и ссылку.")
	}
	s.attachResultURL(order)
	s.notifyPaymentClaimed(ctx, order)
	return order, nil
}

// ClaimPaymentAsChat is «Я оплатил» for the Mini App: the identity is the
// Telegram chat from initData, so the order token is not required (the chat
// owns the order). A foreign chat gets the same neutral not-found error as a
// wrong number+token pair.
func (s *Service) ClaimPaymentAsChat(ctx context.Context, number string, chatID int64, senderName string) (*domain.Order, error) {
	if message := validate.SenderName(senderName); message != "" {
		return nil, ValidationError(map[string]string{"senderName": message})
	}
	order, err := s.Store.ClaimPaymentForChat(ctx, normalizeNumber(number), chatID,
		strings.TrimSpace(senderName))
	if err != nil {
		return nil, FromStore(err, "Заказ не найден. Проверьте номер и ссылку.")
	}
	s.attachResultURL(order)
	s.notifyPaymentClaimed(ctx, order)
	return order, nil
}

// MyOrders is GET /api/me/orders: the orders of one Telegram chat, newest
// first, with the status counters the Mini App filter chips read. No
// number+token lookups are needed - the chat is the identity.
type MyOrders struct {
	Orders []domain.Order `json:"orders"`
	Counts map[string]int `json:"counts"`
}

// MyOrders returns the chat's orders (the Mini App list and home screen).
func (s *Service) MyOrders(ctx context.Context, chatID int64) (MyOrders, error) {
	orders, err := s.Store.ListOrdersForChat(ctx, chatID, 50)
	if err != nil {
		return MyOrders{}, err
	}
	counts := map[string]int{}
	for i := range orders {
		s.attachResultURL(&orders[i])
		counts[orders[i].Status]++
	}
	return MyOrders{Orders: orders, Counts: counts}, nil
}

// OwnOrder returns the full order when it belongs to the chat (Mini App deep
// links: startapp=order_<number>, no token involved). A foreign order answers
// with the same neutral 404 as an unknown number.
func (s *Service) OwnOrder(ctx context.Context, number string, chatID int64) (*domain.Order, error) {
	order, err := s.Store.GetOrder(ctx, normalizeNumber(number))
	if err != nil {
		return nil, FromStore(err, "Заказ не найден. Проверьте номер.")
	}
	if order.TelegramChatID != chatID {
		return nil, NotFound("Заказ не найден. Проверьте номер.")
	}
	s.attachResultURL(order)
	return order, nil
}

// ---------------------------------------------------------------------------
// operator actions (audited: each one writes an order_events row)
// ---------------------------------------------------------------------------

// ConfirmPayment is POST /api/admin/orders/{number}/confirm.
func (s *Service) ConfirmPayment(ctx context.Context, number, operatorID string) (*domain.Order, error) {
	order, err := s.Store.ConfirmPayment(ctx, normalizeNumber(number), operatorID)
	if err != nil {
		return nil, FromStore(err, "Заказ не найден")
	}
	s.notifyPaymentConfirmed(ctx, order)
	return order, nil
}

// RejectPayment is POST /api/admin/orders/{number}/reject.
func (s *Service) RejectPayment(ctx context.Context, number, reason string) (*domain.Order, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, ValidationError(map[string]string{"reason": "Укажите причину отклонения"})
	}
	if len([]rune(reason)) > 300 {
		return nil, ValidationError(map[string]string{"reason": "Не больше 300 символов"})
	}
	order, err := s.Store.RejectPayment(ctx, normalizeNumber(number), reason)
	if err != nil {
		return nil, FromStore(err, "Заказ не найден")
	}
	s.notifyPaymentRejected(ctx, order, reason)
	return order, nil
}

// CancelOrder is POST /api/admin/orders/{number}/cancel.
func (s *Service) CancelOrder(ctx context.Context, number, reason string) (*domain.Order, error) {
	order, err := s.Store.CancelOrder(ctx, normalizeNumber(number), strings.TrimSpace(reason))
	if err != nil {
		return nil, FromStore(err, "Заказ не найден")
	}
	s.notifyCancelled(ctx, order)
	return order, nil
}

// RetryGeneration is POST /api/admin/orders/{number}/retry.
func (s *Service) RetryGeneration(ctx context.Context, number string) (*domain.Order, error) {
	order, err := s.Store.RetryGeneration(ctx, normalizeNumber(number))
	if err != nil {
		return nil, FromStore(err, "Заказ не найден")
	}
	s.notifyQueued(ctx, order)
	return order, nil
}

// SaveOperatorNote is PATCH /api/admin/orders/{number}.
func (s *Service) SaveOperatorNote(ctx context.Context, number, note string) (*domain.Order, error) {
	if len([]rune(note)) > 5000 {
		return nil, ValidationError(map[string]string{"operatorNote": "Не больше 5000 символов"})
	}
	order, err := s.Store.SaveOperatorNote(ctx, normalizeNumber(number), note)
	if err != nil {
		return nil, FromStore(err, "Заказ не найден")
	}
	return order, nil
}

// RequestRevision is POST /api/orders/{number}/revision: the client lists what
// has to be fixed in a finished work. The status does not change (the delivered
// file is still the current one) - the operator decides when to re-run the agent.
func (s *Service) RequestRevision(ctx context.Context, number, note string) (*domain.Order, error) {
	note = strings.TrimSpace(note)
	if len([]rune(note)) < 5 {
		return nil, ValidationError(map[string]string{"revision": "Опишите, что нужно исправить (минимум 5 символов)"})
	}
	if len([]rune(note)) > 2000 {
		return nil, ValidationError(map[string]string{"revision": "Не больше 2000 символов"})
	}
	order, err := s.Store.RequestRevision(ctx, normalizeNumber(number), note)
	if err != nil {
		return nil, FromStore(err, "Заказ не найден")
	}
	s.notifyRevisionRequested(ctx, order)
	return order, nil
}

// ReworkOrder is POST /api/admin/orders/{number}/rework: the operator confirms
// the corrections (edited if needed) and the work goes back to the agent.
func (s *Service) ReworkOrder(ctx context.Context, number, note string) (*domain.Order, error) {
	note = strings.TrimSpace(note)
	if len([]rune(note)) < 5 {
		return nil, ValidationError(map[string]string{"revision": "Опишите правки для генератора (минимум 5 символов)"})
	}
	if len([]rune(note)) > 2000 {
		return nil, ValidationError(map[string]string{"revision": "Не больше 2000 символов"})
	}
	order, err := s.Store.ReworkOrder(ctx, normalizeNumber(number), note)
	if err != nil {
		return nil, FromStore(err, "Заказ не найден")
	}
	s.notifyReworkQueued(ctx, order)
	return order, nil
}

// attachAgentCost fills the tenge value of the generation spend (the panel shows
// dollars and tenge side by side).
func (s *Service) attachAgentCost(ctx context.Context, order *domain.Order) {
	if order == nil || order.AgentCostUSD <= 0 {
		return
	}
	order.AgentCostKZT = s.convertUSDToKZT(ctx, order.AgentCostUSD)
}

// AddAgentCost records what a generation run cost in USD (the operator panel
// shows the per-order total). Failures are logged, never fatal for a run.
func (s *Service) AddAgentCost(ctx context.Context, number string, usd float64) {
	if err := s.Store.AddAgentCost(ctx, number, usd); err != nil {
		s.Log.Error("agent cost", "order", number, "error", err)
	}
}

// RequestRevisionAsChat is the Mini App flavour of RequestRevision: the chat
// owns the order (no token), a foreign chat gets the neutral not-found error.
func (s *Service) RequestRevisionAsChat(ctx context.Context, number string, chatID int64, note string) (*domain.Order, error) {
	if _, err := s.OwnOrder(ctx, number, chatID); err != nil {
		return nil, err
	}
	return s.RequestRevision(ctx, number, note)
}

// notifyRevisionRequested tells the operators about the corrections a client
// asked for on a finished order.
func (s *Service) notifyRevisionRequested(ctx context.Context, order *domain.Order) {
	s.notifyAdmins(ctx, "✏️ Заказ "+order.Number+" - клиент просит правки:\n\n"+
		clipText(order.RevisionNote, 600)+
		"\n\nПанель: Операторы - заказ - «Отправить на доработку».")
}

// notifyReworkQueued tells the client that the fixes went back to the generator.
func (s *Service) notifyReworkQueued(ctx context.Context, order *domain.Order) {
	s.notifyClient(ctx, order, "✏️ Приняли правки по заказу "+order.Number+
		" - запускаем обновление работы. Готовый файл придёт в этот чат.")
}

func clipText(text string, limit int) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit]) + "…"
}

// AdminOrder returns the order with every private field (admin detail page).
func (s *Service) AdminOrder(ctx context.Context, number string) (*domain.Order, error) {
	order, err := s.Store.GetOrder(ctx, normalizeNumber(number))
	if err != nil {
		return nil, FromStore(err, "Заказ не найден")
	}
	s.attachResultURL(order)
	s.attachAgentCost(ctx, order)
	return order, nil
}

// ListOrders is GET /api/admin/orders.
func (s *Service) ListOrders(ctx context.Context, filter store.OrderFilter) (store.OrderList, error) {
	list, err := s.Store.ListOrders(ctx, filter)
	if err != nil {
		return list, err
	}
	for i := range list.Orders {
		s.attachResultURL(&list.Orders[i])
	}
	return list, nil
}

// sanitizeAttachments drops empty file names and fills a missing mime type.
func sanitizeAttachments(files []domain.FileMeta) []domain.FileMeta {
	out := make([]domain.FileMeta, 0, len(files))
	for _, file := range files {
		if strings.TrimSpace(file.Name) == "" {
			continue
		}
		if file.Type == "" {
			file.Type = "application/octet-stream"
		}
		out = append(out, file)
	}
	return out
}

// NormalizeNumber uppercases a human-typed order number.
func NormalizeNumber(number string) string { return normalizeNumber(number) }

func normalizeNumber(number string) string {
	return strings.ToUpper(strings.TrimSpace(number))
}
