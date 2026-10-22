package api

import (
	"html"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/srunal/srunal-go/internal/auth"
	"github.com/srunal/srunal-go/internal/domain"
	"github.com/srunal/srunal-go/internal/service"
	"github.com/srunal/srunal-go/internal/store"
)

// ---------------------------------------------------------------------------
// auth
// ---------------------------------------------------------------------------

// handleAdminLogin is POST /api/admin/auth/login.
// Body: {operatorId, code} (frontend) or {operatorId, password, totp} (2FA).
func (s *Server) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	var input service.LoginInput
	if !decodeJSON(w, r, &input) {
		return
	}
	result, err := s.svc.AdminLogin(r.Context(), input, s.clientIP(r), r.UserAgent())
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// handleAdminLogout is POST /api/admin/auth/logout.
func (s *Server) handleAdminLogout(w http.ResponseWriter, r *http.Request) {
	token := bearerToken(r)
	if token == "" {
		if cookie, err := r.Cookie("srunal_admin"); err == nil {
			token = cookie.Value
		}
	}
	if err := s.svc.AdminLogout(r.Context(), token); err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleAdminMe is GET /api/admin/me (session restore after a reload).
func (s *Server) handleAdminMe(w http.ResponseWriter, r *http.Request) {
	session := operatorFrom(r.Context())
	writeJSON(w, http.StatusOK, domain.AdminSession{
		OperatorID: session.OperatorID,
		Name:       session.DisplayName,
		Since:      session.CreatedAt,
	})
}

// handleTelegramLoginStart is POST /api/admin/auth/telegram/start.
func (s *Server) handleTelegramLoginStart(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OperatorID string `json:"operatorId"`
	}
	if r.ContentLength > 0 {
		if !decodeJSON(w, r, &body) {
			return
		}
	}
	start, err := s.svc.StartTelegramLogin(r.Context(), body.OperatorID)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, start)
}

// handleTelegramLoginPoll is POST /api/admin/auth/telegram/poll.
func (s *Server) handleTelegramLoginPoll(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Code string `json:"code"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	result, pending, err := s.svc.PollTelegramLogin(r.Context(), body.Code, s.clientIP(r), r.UserAgent())
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	if pending {
		writeJSON(w, http.StatusOK, map[string]any{"pending": true})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// ---------------------------------------------------------------------------
// orders
// ---------------------------------------------------------------------------

// handleAdminOrders is GET /api/admin/orders?status=&q=&sort=&page=&limit=.
// Default shape: {orders, counts, total, page, limit}. `?shape=array` returns
// a plain Order[] so the frontend can keep its current listOrders() contract.
func (s *Server) handleAdminOrders(w http.ResponseWriter, r *http.Request) {
	filter := store.OrderFilter{
		Status: strings.TrimSpace(r.URL.Query().Get("status")),
		Query:  strings.TrimSpace(r.URL.Query().Get("q")),
		Sort:   strings.TrimSpace(r.URL.Query().Get("sort")),
		Page:   intQuery(r, "page", 1),
		Limit:  intQuery(r, "limit", 50),
	}
	list, err := s.svc.ListOrders(r.Context(), filter)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	if r.URL.Query().Get("shape") == "array" {
		writeJSON(w, http.StatusOK, list.Orders)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"orders": list.Orders,
		"counts": list.Counts,
		"total":  list.Total,
		"page":   filter.Page,
		"limit":  filter.Limit,
	})
}

// handleAdminOrder is GET /api/admin/orders/{number} (full order).
func (s *Server) handleAdminOrder(w http.ResponseWriter, r *http.Request) {
	order, err := s.svc.AdminOrder(r.Context(), r.PathValue("number"))
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, order)
}

// handleAdminConsole is GET /api/admin/orders/{number}/console: the live view of
// the generation run, rendered as readable agent events.
func (s *Server) handleAdminConsole(w http.ResponseWriter, r *http.Request) {
	console, err := s.svc.AdminOrderConsole(r.Context(), r.PathValue("number"))
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, console)
}

// handleAdminConsoleLog is GET /api/admin/orders/{number}/console/log. Two ways
// in: the admin session (the panel) or a signed short-lived ?token= link, which
// is what the operator clicks to open the log in a plain browser - there is no
// initData there, so without the token the request used to answer 401.
//
// ?json=1 returns the parsed events (the viewer polls them), ?download=1 the raw
// agent log as a file, and everything else the standalone HTML viewer.
func (s *Server) handleAdminConsoleLog(w http.ResponseWriter, r *http.Request) {
	number := r.PathValue("number")
	token := r.URL.Query().Get("token")
	if !s.adminAuthorized(r) && !auth.VerifyLogLinkToken(s.svc.LogTokenSecret(), number, token, time.Now()) {
		s.writeServiceError(w, r, service.Unauthorized("Ссылка на лог устарела. Откройте её заново из панели оператора."))
		return
	}
	if _, err := s.svc.AdminOrder(r.Context(), number); err != nil {
		s.writeServiceError(w, r, err)
		return
	}

	if r.URL.Query().Get("download") == "1" {
		file, err := os.Open(s.svc.ConsoleLogPath(number))
		if err != nil {
			s.writeServiceError(w, r, service.NotFound("Лог генерации ещё не создан"))
			return
		}
		defer file.Close()
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="agent-`+number+`.log"`)
		_, _ = io.Copy(w, file)
		return
	}

	console, err := s.svc.AdminOrderConsole(r.Context(), number)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	if r.URL.Query().Get("json") == "1" {
		writeJSON(w, http.StatusOK, console)
		return
	}

	page := strings.ReplaceAll(consoleLogPage, "__NUMBER__", html.EscapeString(number))
	page = strings.ReplaceAll(page, "__TOKEN__", html.EscapeString(token))
	page = strings.ReplaceAll(page, "__STATUS__", html.EscapeString(console.Status))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, page)
}

// handleAdminRework is POST /api/admin/orders/{number}/rework: the operator
// confirms the corrections («Нужны правки») and the finished work goes back to
// the agent, which regenerates the file with them applied.
func (s *Server) handleAdminRework(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Note string `json:"note"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	order, err := s.svc.ReworkOrder(r.Context(), r.PathValue("number"), body.Note)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, order)
}

// handleAdminConfirm is POST /api/admin/orders/{number}/confirm.
func (s *Server) handleAdminConfirm(w http.ResponseWriter, r *http.Request) {
	session := operatorFrom(r.Context())
	order, err := s.svc.ConfirmPayment(r.Context(), r.PathValue("number"), session.OperatorID)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, order)
}

// handleAdminReject is POST /api/admin/orders/{number}/reject.
func (s *Server) handleAdminReject(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Reason string `json:"reason"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	order, err := s.svc.RejectPayment(r.Context(), r.PathValue("number"), body.Reason)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, order)
}

// handleAdminCancel is POST /api/admin/orders/{number}/cancel.
func (s *Server) handleAdminCancel(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Reason string `json:"reason"`
	}
	if r.ContentLength > 0 {
		if !decodeJSON(w, r, &body) {
			return
		}
	}
	order, err := s.svc.CancelOrder(r.Context(), r.PathValue("number"), body.Reason)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, order)
}

// handleAdminRetry is POST /api/admin/orders/{number}/retry.
func (s *Server) handleAdminRetry(w http.ResponseWriter, r *http.Request) {
	order, err := s.svc.RetryGeneration(r.Context(), r.PathValue("number"))
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, order)
}

// handleAdminNote is PATCH /api/admin/orders/{number} (saved on blur).
func (s *Server) handleAdminNote(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OperatorNote *string `json:"operatorNote"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	note := ""
	if body.OperatorNote != nil {
		note = *body.OperatorNote
	}
	order, err := s.svc.SaveOperatorNote(r.Context(), r.PathValue("number"), note)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, order)
}

// ---------------------------------------------------------------------------
// reviews
// ---------------------------------------------------------------------------

// handleAdminReviews is GET /api/admin/reviews?status= with tab counters.
func (s *Server) handleAdminReviews(w http.ResponseWriter, r *http.Request) {
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	reviews, err := s.svc.ListReviews(r.Context(), status, intQuery(r, "limit", 200), intQuery(r, "offset", 0))
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	counts, err := s.svc.ReviewCounts(r.Context())
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	if r.URL.Query().Get("shape") == "array" {
		writeJSON(w, http.StatusOK, reviews)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"reviews": reviews, "counts": counts})
}

// handleAdminModerateReview is PATCH /api/admin/reviews/{id}.
func (s *Server) handleAdminModerateReview(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Status string `json:"status"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	session := operatorFrom(r.Context())
	review, err := s.svc.ModerateReview(r.Context(), r.PathValue("id"), body.Status, session.OperatorID)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, review)
}

// ---------------------------------------------------------------------------
// settings, pricing, promos, stats
// ---------------------------------------------------------------------------

// handleAdminSettingsGet is GET /api/admin/settings.
func (s *Server) handleAdminSettingsGet(w http.ResponseWriter, r *http.Request) {
	settings, err := s.svc.GetSettings(r.Context())
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

// handleAdminSettingsPut is PUT /api/admin/settings.
func (s *Server) handleAdminSettingsPut(w http.ResponseWriter, r *http.Request) {
	// The payment link is a newer field: clients that do not know it (the legacy
	// web panel) send no key at all, and an empty string would wipe it. Decode
	// the field separately so "absent" keeps the stored value while an explicit
	// "" clears it.
	var body struct {
		domain.Settings
		PaymentLink *string `json:"paymentLink"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	settings := body.Settings
	if body.PaymentLink != nil {
		settings.PaymentLink = *body.PaymentLink
	} else if current, err := s.svc.GetSettings(r.Context()); err == nil {
		settings.PaymentLink = current.PaymentLink
	}

	saved, err := s.svc.SaveSettings(r.Context(), settings)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

// handleAdminQrUpload is POST /api/admin/settings/payment-qr (multipart).
func (s *Server) handleAdminQrUpload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(3 << 20); err != nil {
		writeError(w, service.ValidationError(map[string]string{"file": "Не удалось прочитать файл"}))
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		file, _, err = r.FormFile("qr")
	}
	if err != nil {
		writeError(w, service.ValidationError(map[string]string{"file": "Прикрепите файл QR"}))
		return
	}
	defer file.Close()

	const maxUpload = 1536 * 1024
	data, err := io.ReadAll(io.LimitReader(file, maxUpload+1))
	if err != nil {
		writeError(w, service.ValidationError(map[string]string{"file": "Не удалось прочитать файл"}))
		return
	}
	if len(data) > maxUpload {
		writeError(w, service.ValidationError(map[string]string{"file": "Файл больше 1.5 МБ"}))
		return
	}

	url, err := s.svc.SaveQrImage(r.Context(), data)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	settings, err := s.svc.GetSettings(r.Context())
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"paymentQr": url,
		"settings":  settings,
	})
}

// handleAdminPricingGet is GET /api/admin/pricing.
func (s *Server) handleAdminPricingGet(w http.ResponseWriter, r *http.Request) {
	config, err := s.svc.Pricing(r.Context())
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, config)
}

// handleAdminPricingPut is PUT /api/admin/pricing.
func (s *Server) handleAdminPricingPut(w http.ResponseWriter, r *http.Request) {
	var config domain.PricingConfig
	if !decodeJSON(w, r, &config) {
		return
	}
	saved, err := s.svc.SavePricing(r.Context(), config)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

// handleAdminPromos is GET /api/admin/promos.
func (s *Server) handleAdminPromos(w http.ResponseWriter, r *http.Request) {
	promos, err := s.svc.ListPromos(r.Context())
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, promos)
}

// handleAdminCreatePromo is POST /api/admin/promos.
func (s *Server) handleAdminCreatePromo(w http.ResponseWriter, r *http.Request) {
	var input domain.PromoInput
	if !decodeJSON(w, r, &input) {
		return
	}
	promo, err := s.svc.CreatePromo(r.Context(), input)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, promo)
}

// handleAdminUpdatePromo is PATCH /api/admin/promos/{id}.
func (s *Server) handleAdminUpdatePromo(w http.ResponseWriter, r *http.Request) {
	var patch domain.PromoInput
	if !decodeJSON(w, r, &patch) {
		return
	}
	promo, err := s.svc.UpdatePromo(r.Context(), r.PathValue("id"), patch)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, promo)
}

// handleAdminDeletePromo is DELETE /api/admin/promos/{id}.
func (s *Server) handleAdminDeletePromo(w http.ResponseWriter, r *http.Request) {
	if err := s.svc.DeletePromo(r.Context(), r.PathValue("id")); err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleAdminStats is GET /api/admin/stats.
func (s *Server) handleAdminStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.svc.Stats(r.Context())
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, stats)
}
