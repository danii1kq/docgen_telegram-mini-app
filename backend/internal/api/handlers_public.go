package api

import (
	"context"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/srunal/srunal-go/internal/domain"
	"github.com/srunal/srunal-go/internal/service"
	"github.com/srunal/srunal-go/internal/store"
	"github.com/srunal/srunal-go/internal/validate"
)

// handleHealth is used by the frontend and by uptime checks.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	status := "ok"
	code := http.StatusOK
	if err := s.svc.Store.Ping(r.Context()); err != nil {
		status = "degraded"
		code = http.StatusServiceUnavailable
	}
	writeJSON(w, code, map[string]any{
		"status":    status,
		"time":      time.Now().UTC(),
		"database":  status,
		"version":   "1.0.0",
		"botActive": s.cfg.BotConfigured(),
	})
}

// handleQuote is POST /api/quote (debounced 300 ms by the client).
func (s *Server) handleQuote(w http.ResponseWriter, r *http.Request) {
	var request service.QuoteRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	quote, err := s.svc.Quote(r.Context(), request)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, quote)
}

// handlePromoCheck is POST /api/promos/check.
func (s *Server) handlePromoCheck(w http.ResponseWriter, r *http.Request) {
	if !s.limit(w, r, "promo", s.cfg.PromoCheckPerMin, time.Minute,
		"Слишком много проверок, подождите минуту") {
		return
	}
	var request service.PromoCheckRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	result, err := s.svc.CheckPromo(r.Context(), request)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// handleProducts serves the catalogue (names, unit words, ranges).
func (s *Server) handleProducts(w http.ResponseWriter, r *http.Request) {
	products := service.Products()
	// Live unit prices from the pricing table (the admin panel can edit them):
	// the Mini App calculator shows the server price, never a hardcoded one.
	prices := map[string]int{}
	if config, err := s.svc.Pricing(r.Context()); err == nil {
		for _, product := range config.Products {
			prices[product.Code] = product.UnitPrice
		}
	}
	items := make([]map[string]any, 0, len(products))
	for _, product := range products {
		price := product.PricePerUnit
		if configured, ok := prices[product.Code]; ok {
			price = configured
		}
		items = append(items, map[string]any{
			"code":      product.Code,
			"name":      product.Name,
			"short":     product.Short,
			"unitOne":   product.UnitOne,
			"unitFew":   product.UnitFew,
			"unitMany":  product.UnitMany,
			"unitTitle": product.UnitTitle,
			"min":       product.Min,
			"max":       product.Max,
			"fixed":     product.Fixed,
			"price":     price,
			"unitLabel": product.UnitLabel(),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"products": items})
}

// handleSettings is GET /api/settings (public subset).
func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := s.svc.GetSettings(r.Context())
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

// handlePaymentQr serves the uploaded Kaspi QR image.
func (s *Server) handlePaymentQr(w http.ResponseWriter, r *http.Request) {
	path := s.svc.QrFilePath()
	if path == "" {
		writeError(w, service.NotFound("QR не загружен"))
		return
	}
	w.Header().Set("Content-Type", s.svc.QrContentType())
	w.Header().Set("Cache-Control", "public, max-age=300")
	http.ServeFile(w, r, path)
}

// handleCreateOrder is POST /api/orders. The client must be signed in through
// the Telegram bot: the session's chat receives the order notifications, so the
// operator always has a reachable contact.
func (s *Server) handleCreateOrder(w http.ResponseWriter, r *http.Request) {
	if !s.limit(w, r, "orders", s.cfg.CreateOrderPerHour, time.Hour,
		"Слишком много заказов с одного адреса, попробуйте позже") {
		return
	}
	session, ok := s.clientSessionOf(w, r)
	if !ok {
		return
	}
	var form validate.OrderForm
	if !decodeJSON(w, r, &form) {
		return
	}
	result, err := s.svc.CreateOrder(r.Context(), form, strings.TrimSpace(r.Header.Get("Idempotency-Key")), &session)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

// handleGetOrder is GET /api/orders/{number}: the public status page. A valid
// ?token= upgrades the response to the full order; the Mini App reaches the
// same full order through its chat identity (initData), no token needed.
// Without either, the operator's notes and the contact details stay hidden.
func (s *Server) handleGetOrder(w http.ResponseWriter, r *http.Request) {
	number := r.PathValue("number")
	if token := strings.TrimSpace(r.URL.Query().Get("token")); token != "" {
		order, err := s.svc.GetOrderByToken(r.Context(), number, token)
		if err != nil {
			s.writeServiceError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, order)
		return
	}

	if session, ok := s.clientSession(r); ok {
		// A signed-in Mini App client asking for someone else's order gets the
		// neutral not-found state instead of the public view (design.md §9.2):
		// the app never reveals that the number exists.
		order, err := s.svc.OwnOrder(r.Context(), number, session.ChatID)
		if err != nil {
			s.writeServiceError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, order)
		return
	}

	order, err := s.svc.GetOrderPublic(r.Context(), number)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, order.Redacted())
}

// handleLookupOrder is POST /api/orders/lookup (the /track page).
func (s *Server) handleLookupOrder(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Number string `json:"number"`
		Token  string `json:"token"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	order, err := s.svc.GetOrderByToken(r.Context(), body.Number, body.Token)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, order)
}

// handleBatchOrders is POST /api/orders/batch (the /profile page).
func (s *Server) handleBatchOrders(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Items []service.TokenPair `json:"items"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if len(body.Items) > 50 {
		body.Items = body.Items[:50]
	}
	orders, err := s.svc.BatchOrders(r.Context(), body.Items)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, orders)
}

// handleClaimPayment is POST /api/orders/{number}/claim («Я оплатил»). The
// order token identifies the client in the web flow; the Mini App sends no
// token and is identified by its Telegram chat (initData).
func (s *Server) handleClaimPayment(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token      string `json:"token"`
		SenderName string `json:"senderName"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	var (
		order *domain.Order
		err   error
	)
	if strings.TrimSpace(body.Token) != "" {
		order, err = s.svc.ClaimPayment(r.Context(), r.PathValue("number"), body.Token, body.SenderName)
	} else if session, ok := s.clientSession(r); ok {
		order, err = s.svc.ClaimPaymentAsChat(r.Context(), r.PathValue("number"), session.ChatID, body.SenderName)
	} else {
		order, err = s.svc.ClaimPayment(r.Context(), r.PathValue("number"), body.Token, body.SenderName)
	}
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, order)
}

// handleRequestRevision is POST /api/orders/{number}/revision: the chat that
// owns the order lists what has to be fixed in the delivered work («Нужны
// правки»). The operator reviews the text and sends the order back to the agent.
func (s *Server) handleRequestRevision(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Note string `json:"note"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	session, ok := s.clientSession(r)
	if !ok {
		s.writeServiceError(w, r, service.Unauthorized("Откройте приложение из Telegram, чтобы отправить правки"))
		return
	}
	order, err := s.svc.RequestRevisionAsChat(r.Context(), r.PathValue("number"), session.ChatID, body.Note)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, order)
}

// handleOrderResult is GET /api/orders/{number}/result: the ready document.
// Access is granted by a signed link (?sig=), by the order token, to an
// authenticated operator, or to the Telegram chat that owns the order (the
// Mini App download button).
func (s *Server) handleOrderResult(w http.ResponseWriter, r *http.Request) {
	number := r.PathValue("number")
	query := r.URL.Query()

	authorized := false
	switch {
	case query.Get("sig") != "":
		authorized = s.svc.VerifyResultSignature(number, query.Get("sig"))
	case query.Get("token") != "":
		if _, err := s.svc.GetOrderByToken(r.Context(), number, query.Get("token")); err == nil {
			authorized = true
		}
	default:
		if token := bearerToken(r); token != "" {
			if _, err := s.svc.AdminSession(r.Context(), token); err == nil {
				authorized = true
			}
		}
		if !authorized {
			if session, ok := s.clientSession(r); ok {
				if _, err := s.svc.OwnOrder(r.Context(), number, session.ChatID); err == nil {
					authorized = true
				}
			}
		}
	}
	if !authorized {
		writeError(w, service.Unauthorized("Ссылка на файл недействительна, откройте заказ заново"))
		return
	}

	file, err := func() (store.OrderFileRow, error) {
		if raw := strings.TrimSpace(query.Get("file")); raw != "" {
			id, parseErr := strconv.ParseInt(raw, 10, 64)
			if parseErr != nil {
				return store.OrderFileRow{}, service.NotFound("Файл не найден")
			}
			// A multi-work order has one document per work: ?file=<id> picks one.
			return s.svc.GetResultFileByID(r.Context(), number, id)
		}
		return s.svc.GetResultFile(r.Context(), number)
	}()
	if err != nil {
		s.writeServiceError(w, r, service.NotFound("Результат ещё не готов"))
		return
	}
	if file.ExpiresAt != nil && file.ExpiresAt.Before(time.Now().UTC()) {
		writeError(w, service.NotFound("Срок хранения файла истёк, напишите в поддержку"))
		return
	}
	path := s.svc.ResultFilePath(file)
	if path == "" {
		writeError(w, service.NotFound("Файл не найден"))
		return
	}
	if _, err := os.Stat(path); err != nil {
		writeError(w, service.NotFound("Файл не найден"))
		return
	}

	w.Header().Set("Content-Type", file.MimeType)
	w.Header().Set("Content-Disposition", "attachment; filename=\""+file.FileName+"\"")
	http.ServeFile(w, r, path)
}

// handleListReviews is GET /api/reviews?status=published&limit=&offset=.
func (s *Server) handleListReviews(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	limit := intQuery(r, "limit", 100)
	offset := intQuery(r, "offset", 0)
	reviews, err := s.svc.ListReviews(r.Context(), status, limit, offset)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, reviews)
}

// handleAddReview is POST /api/reviews.
func (s *Server) handleAddReview(w http.ResponseWriter, r *http.Request) {
	if !s.limit(w, r, "reviews", s.cfg.ReviewPerHour, time.Hour,
		"Слишком много отзывов, попробуйте позже") {
		return
	}
	var input domain.ReviewInput
	if !decodeJSON(w, r, &input) {
		return
	}
	review, err := s.svc.AddReview(r.Context(), input)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, review)
}

// handleNotFound keeps unknown API paths JSON-shaped.
func (s *Server) handleNotFound(w http.ResponseWriter, r *http.Request) {
	writeError(w, &service.APIError{
		Status:  http.StatusNotFound,
		Code:    service.CodeNotFound,
		Message: "Метод не найден",
	})
}

// ---------------------------------------------------------------------------
// small helpers shared by the handlers
// ---------------------------------------------------------------------------

func intQuery(r *http.Request, name string, fallback int) int {
	raw := strings.TrimSpace(r.URL.Query().Get(name))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

// operatorFrom returns the session of the authenticated operator.
func operatorFrom(ctx context.Context) store.SessionRow {
	session, _ := ctx.Value(contextKeyOperator).(store.SessionRow)
	return session
}
