package api

import (
	"net/http"

	"github.com/srunal/srunal-go/internal/service"
	"github.com/srunal/srunal-go/internal/store"
)

// Client authorization through the Telegram bot:
//
//	POST /api/auth/telegram/start -> one-time code
//	the client sends /login <code> to the bot
//	POST /api/auth/telegram/poll  -> {session, token} (+ operator part when the
//	                                 chat is bound to an operator)
//	GET  /api/auth/me             -> current session
//	POST /api/auth/logout         -> revoke it

// handleClientAuthStart is POST /api/auth/telegram/start.
func (s *Server) handleClientAuthStart(w http.ResponseWriter, r *http.Request) {
	if !s.limit(w, r, "client-login", s.cfg.LoginMaxAttempts*4, s.cfg.LoginLockout,
		"Слишком много попыток входа, попробуйте позже") {
		return
	}
	start, err := s.svc.StartClientLogin(r.Context())
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, start)
}

// handleClientAuthPoll is POST /api/auth/telegram/poll.
func (s *Server) handleClientAuthPoll(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Code string `json:"code"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	result, pending, err := s.svc.PollClientLogin(r.Context(), body.Code, s.clientIP(r), r.UserAgent())
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

// handleClientMe is GET /api/auth/me (session restore after a reload). The
// answer carries the operator identity when the chat is bound to an operator,
// so the Mini App can show the panel entry.
func (s *Server) handleClientMe(w http.ResponseWriter, r *http.Request) {
	session := clientFrom(r.Context())
	profile := service.ClientProfile(session)
	if operatorID, operatorName, ok := s.svc.OperatorForChat(r.Context(), session.ChatID); ok {
		profile.OperatorID = operatorID
		profile.OperatorName = operatorName
	}
	writeJSON(w, http.StatusOK, profile)
}

// handleMyOrders is GET /api/me/orders: the orders of this Telegram chat,
// newest first, with the status counters (the Mini App home and list screens).
func (s *Server) handleMyOrders(w http.ResponseWriter, r *http.Request) {
	session := clientFrom(r.Context())
	orders, err := s.svc.MyOrders(r.Context(), session.ChatID)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, orders)
}

// handleClientLogout is POST /api/auth/logout.
func (s *Server) handleClientLogout(w http.ResponseWriter, r *http.Request) {
	if err := s.svc.ClientLogout(r.Context(), bearerToken(r)); err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// clientSessionOf resolves the session for handlers that require one but are
// not wrapped in requireClient (order creation reports its own message).
func (s *Server) clientSessionOf(w http.ResponseWriter, r *http.Request) (store.ClientSession, bool) {
	session, ok := s.clientSession(r)
	if !ok {
		writeError(w, service.Unauthorized("Чтобы оформить заказ, войдите через Telegram-бота"))
		return store.ClientSession{}, false
	}
	return session, true
}
