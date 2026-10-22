// Package api is the HTTP layer: routing, JSON envelopes, CORS, rate limiting
// and the operator session middleware.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/srunal/srunal-go/internal/config"
	"github.com/srunal/srunal-go/internal/service"
	"github.com/srunal/srunal-go/internal/store"
)

// Server wires the service into net/http.
type Server struct {
	svc     *service.Service
	cfg     *config.Config
	log     *slog.Logger
	handler http.Handler
	limiter *limiter
}

// New builds the server and its route table.
func New(svc *service.Service, cfg *config.Config, log *slog.Logger) *Server {
	server := &Server{
		svc:     svc,
		cfg:     cfg,
		log:     log,
		limiter: newLimiter(),
	}
	server.handler = server.buildRouter()
	return server
}

// Handler returns the root http.Handler.
func (s *Server) Handler() http.Handler { return s.handler }

func (s *Server) buildRouter() http.Handler {
	mux := http.NewServeMux()

	// --- public ---------------------------------------------------------
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("POST /api/quote", s.handleQuote)
	mux.HandleFunc("POST /api/promos/check", s.handlePromoCheck)
	mux.HandleFunc("GET /api/products", s.handleProducts)
	mux.HandleFunc("GET /api/settings", s.handleSettings)
	mux.HandleFunc("GET /api/settings/payment-qr", s.handlePaymentQr)

	mux.HandleFunc("POST /api/orders", s.handleCreateOrder)
	mux.HandleFunc("GET /api/orders/{number}", s.handleGetOrder)
	mux.HandleFunc("GET /api/orders/{number}/result", s.handleOrderResult)
	mux.HandleFunc("POST /api/orders/lookup", s.handleLookupOrder)
	mux.HandleFunc("POST /api/orders/batch", s.handleBatchOrders)
	mux.HandleFunc("POST /api/orders/{number}/claim", s.handleClaimPayment)
	mux.HandleFunc("POST /api/orders/{number}/revision", s.handleRequestRevision)

	// --- client authorization through the Telegram bot -------------------
	mux.HandleFunc("POST /api/auth/telegram/start", s.handleClientAuthStart)
	mux.HandleFunc("POST /api/auth/telegram/poll", s.handleClientAuthPoll)
	mux.HandleFunc("GET /api/auth/me", s.requireClient(s.handleClientMe))
	mux.HandleFunc("POST /api/auth/logout", s.handleClientLogout)

	// The Mini App reads its own data by chat identity (initData): the orders
	// list and the full order card need no number+token pairs.
	mux.HandleFunc("GET /api/me/orders", s.requireClient(s.handleMyOrders))

	mux.HandleFunc("GET /api/reviews", s.handleListReviews)
	mux.HandleFunc("POST /api/reviews", s.handleAddReview)

	// --- admin auth (no session required) -------------------------------
	mux.HandleFunc("POST /api/admin/auth/login", s.handleAdminLogin)
	mux.HandleFunc("POST /api/admin/auth/telegram/start", s.handleTelegramLoginStart)
	mux.HandleFunc("POST /api/admin/auth/telegram/poll", s.handleTelegramLoginPoll)

	// --- admin (session required) ---------------------------------------
	admin := func(pattern string, handler http.HandlerFunc) {
		mux.Handle("GET "+pattern, s.requireAdmin(handler))
	}
	mux.Handle("POST /api/admin/auth/logout", s.requireAdmin(http.HandlerFunc(s.handleAdminLogout)))
	admin("/api/admin/me", s.handleAdminMe)
	admin("/api/admin/orders", s.handleAdminOrders)
	admin("/api/admin/orders/{number}", s.handleAdminOrder)
	admin("/api/admin/orders/{number}/console", s.handleAdminConsole)
	admin("/api/admin/reviews", s.handleAdminReviews)
	admin("/api/admin/settings", s.handleAdminSettingsGet)
	admin("/api/admin/pricing", s.handleAdminPricingGet)
	admin("/api/admin/promos", s.handleAdminPromos)
	admin("/api/admin/stats", s.handleAdminStats)
	mux.Handle("PUT /api/admin/settings", s.requireAdmin(http.HandlerFunc(s.handleAdminSettingsPut)))
	mux.Handle("PUT /api/admin/pricing", s.requireAdmin(http.HandlerFunc(s.handleAdminPricingPut)))
	mux.Handle("POST /api/admin/settings/payment-qr", s.requireAdmin(http.HandlerFunc(s.handleAdminQrUpload)))
	mux.Handle("POST /api/admin/orders/{number}/confirm", s.requireAdmin(http.HandlerFunc(s.handleAdminConfirm)))
	mux.Handle("POST /api/admin/orders/{number}/reject", s.requireAdmin(http.HandlerFunc(s.handleAdminReject)))
	mux.Handle("POST /api/admin/orders/{number}/cancel", s.requireAdmin(http.HandlerFunc(s.handleAdminCancel)))
	mux.Handle("POST /api/admin/orders/{number}/retry", s.requireAdmin(http.HandlerFunc(s.handleAdminRetry)))
	mux.Handle("POST /api/admin/orders/{number}/rework", s.requireAdmin(http.HandlerFunc(s.handleAdminRework)))
	mux.Handle("PATCH /api/admin/orders/{number}", s.requireAdmin(http.HandlerFunc(s.handleAdminNote)))
	// The log accepts an admin session OR the signed ?token= link (opened in a
	// plain browser), so the handler checks the identity itself.
	mux.HandleFunc("GET /api/admin/orders/{number}/console/log", s.handleAdminConsoleLog)
	mux.Handle("PATCH /api/admin/reviews/{id}", s.requireAdmin(http.HandlerFunc(s.handleAdminModerateReview)))
	mux.Handle("POST /api/admin/promos", s.requireAdmin(http.HandlerFunc(s.handleAdminCreatePromo)))
	mux.Handle("PATCH /api/admin/promos/{id}", s.requireAdmin(http.HandlerFunc(s.handleAdminUpdatePromo)))
	mux.Handle("DELETE /api/admin/promos/{id}", s.requireAdmin(http.HandlerFunc(s.handleAdminDeletePromo)))

	// Unmatched /api/* routes keep the JSON 404 envelope.
	mux.HandleFunc("/api/", s.handleNotFound)

	// Everything else is the Mini App SPA when MINIAPP_DIR points at a build.
	// The app is served from the same origin as the API, so initData requests
	// need no CORS at all.
	if spa := newSPAHandler(s.cfg.MiniAppDir); spa != nil {
		s.log.Info("miniapp enabled", "dir", s.cfg.MiniAppDir)
		mux.Handle("/", spa)
	} else {
		mux.HandleFunc("/", s.handleNotFound)
	}

	return s.withMiddleware(mux)
}

// ---------------------------------------------------------------------------
// middleware
// ---------------------------------------------------------------------------

type contextKey string

const (
	contextKeyRequestID contextKey = "requestID"
	contextKeyOperator  contextKey = "operator"
	contextKeyClient    contextKey = "client"
)

// initDataHeader carries the raw Mini App payload (tg.initData). The server
// validates its HMAC signature on every request, see
// internal/auth.ValidateInitData: the chat identity needs no login flow.
const initDataHeader = "X-Telegram-Init-Data"

func (s *Server) withMiddleware(next http.Handler) http.Handler {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		requestID := fmt.Sprintf("%d", started.UnixNano())
		w.Header().Set("X-Request-ID", requestID)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		// Defence in depth: the API is never framed and never loaded as a
		// cross-origin resource (the Mini App and the API share one origin).
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")

		wrapped := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(wrapped, r.WithContext(context.WithValue(r.Context(), contextKeyRequestID, requestID)))

		s.log.Info("http",
			"method", r.Method,
			"path", r.URL.Path,
			"status", wrapped.status,
			"duration_ms", time.Since(started).Milliseconds(),
			"request_id", requestID)
	})

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				s.log.Error("panic", "value", recovered, "path", r.URL.Path)
				writeError(w, service.Internal())
			}
		}()
		s.cors(handler).ServeHTTP(w, r)
	})
}

// cors answers browser preflights and marks the configured web origins as
// allowed. It runs before the router so OPTIONS never reaches the route table.
func (s *Server) cors(next http.Handler) http.Handler {
	allowed := make(map[string]bool, len(s.cfg.CORSOrigins))
	for _, origin := range s.cfg.CORSOrigins {
		if trimmed := strings.ToLower(strings.TrimSpace(origin)); trimmed != "" {
			allowed[trimmed] = true
		}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		originAllowed := origin != "" && allowed[strings.ToLower(origin)]

		if originAllowed {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Expose-Headers", "Retry-After, X-Request-ID")
			w.Header().Add("Vary", "Origin")
		}

		// Preflight request (the browser asks before a JSON POST with headers).
		if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
			if !originAllowed {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers",
				"Accept, Authorization, Content-Type, Idempotency-Key, X-Request-ID, X-Telegram-Init-Data")
			w.Header().Set("Access-Control-Max-Age", "600")
			w.Header().Add("Vary", "Access-Control-Request-Method, Access-Control-Request-Headers")
			w.WriteHeader(http.StatusNoContent)
			return
		}

		if r.Method == http.MethodOptions {
			// A plain OPTIONS without preflight headers: nothing to do.
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// statusWriter remembers the status code for logging.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

// clientIP resolves the caller address (honours X-Forwarded-For when enabled).
func (s *Server) clientIP(r *http.Request) string {
	if s.cfg.TrustProxy {
		if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
			parts := strings.Split(forwarded, ",")
			if ip := strings.TrimSpace(parts[0]); ip != "" {
				return ip
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// adminAuthorized reports whether the request carries a valid operator identity
// (bearer token or Mini App initData) without writing a response. Handlers that
// accept more than one way in (the log link) use it directly.
//
// The legacy "srunal_admin" cookie is deliberately not accepted any more: a
// cookie travels with every cross-site request, so it would make the admin API
// a CSRF target. The panel (and the old web panel) send the bearer token.
func (s *Server) adminAuthorized(r *http.Request) bool {
	token := bearerToken(r)
	if token != "" {
		if _, err := s.svc.AdminSession(r.Context(), token); err == nil {
			return true
		}
	}
	if raw := strings.TrimSpace(r.Header.Get(initDataHeader)); raw != "" {
		if _, err := s.svc.AdminSessionFromInitData(r.Context(), raw); err == nil {
			return true
		}
	}
	return false
}

// requireAdmin guards /api/admin/* with a bearer token or the Mini App
// identity: a validated initData payload whose chat is bound to an operator
// opens the panel without a password or a login code.
func (s *Server) requireAdmin(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only the bearer token or the Mini App initData are accepted: the
		// legacy cookie is gone, otherwise any site could drive the admin API
		// with the operator's session.
		token := bearerToken(r)
		session, err := s.svc.AdminSession(r.Context(), token)
		if err != nil {
			if raw := strings.TrimSpace(r.Header.Get(initDataHeader)); raw != "" {
				if miniSession, miniErr := s.svc.AdminSessionFromInitData(r.Context(), raw); miniErr == nil {
					session, err = miniSession, nil
				}
			}
		}
		if err != nil {
			s.writeServiceError(w, r, err)
			return
		}
		ctx := context.WithValue(r.Context(), contextKeyOperator, session)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// requireClient guards the endpoints that need a client session (the Telegram
// login identity): ordering and the client's own session info.
func (s *Server) requireClient(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session, ok := s.clientSession(r)
		if !ok {
			writeError(w, service.Unauthorized("Войдите через Telegram-бота, чтобы продолжить"))
			return
		}
		ctx := context.WithValue(r.Context(), contextKeyClient, session)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

// clientSession resolves the client identity of the request: either a bearer
// token (the web login) or the Mini App initData header, whose HMAC signature
// proves the Telegram chat. The Mini App needs no login code - every request
// is self-authenticating.
func (s *Server) clientSession(r *http.Request) (store.ClientSession, bool) {
	if token := bearerToken(r); token != "" {
		session, err := s.svc.ClientSession(r.Context(), token)
		if err != nil {
			return store.ClientSession{}, false
		}
		return session, true
	}
	if raw := strings.TrimSpace(r.Header.Get(initDataHeader)); raw != "" {
		session, err := s.svc.ClientSessionFromInitData(raw)
		if err != nil {
			return store.ClientSession{}, false
		}
		return session, true
	}
	return store.ClientSession{}, false
}

// clientFrom returns the session stored by requireClient.
func clientFrom(ctx context.Context) store.ClientSession {
	session, _ := ctx.Value(contextKeyClient).(store.ClientSession)
	return session
}

func bearerToken(r *http.Request) string {
	header := r.Header.Get("Authorization")
	if len(header) > 7 && strings.EqualFold(header[:7], "bearer ") {
		return strings.TrimSpace(header[7:])
	}
	return ""
}

// ---------------------------------------------------------------------------
// JSON helpers
// ---------------------------------------------------------------------------

const maxRequestBody = 1 << 20 // 1 MB of JSON

// errorEnvelope mirrors README section 7: {"error":{code,message,fields}}.
type errorEnvelope struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if value == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(value); err != nil {
		// The client went away; nothing else to do.
		_ = err
	}
}

func writeError(w http.ResponseWriter, apiError *service.APIError) {
	if apiError == nil {
		apiError = service.Internal()
	}
	if apiError.RetryAfter > 0 {
		w.Header().Set("Retry-After", fmt.Sprintf("%d", apiError.RetryAfter))
	}
	writeJSON(w, apiError.Status, errorEnvelope{Error: errorBody{
		Code:    apiError.Code,
		Message: apiError.Message,
		Fields:  apiError.Fields,
	}})
}

// writeServiceError maps any error from the service to the JSON envelope.
func (s *Server) writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	var apiError *service.APIError
	if errors.As(err, &apiError) {
		writeError(w, apiError)
		return
	}
	s.log.Error("internal error", "path", r.URL.Path, "error", err)
	writeError(w, service.Internal())
}

// decodeJSON reads a bounded JSON body. A leading UTF-8 BOM (PowerShell 5.1
// tools add one) is stripped before decoding.
func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	reader := http.MaxBytesReader(w, r.Body, maxRequestBody)
	data, err := io.ReadAll(reader)
	if err != nil {
		message := "Некорректный запрос"
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			message = "Запрос слишком большой"
		}
		writeError(w, &service.APIError{Status: 400, Code: service.CodeValidation, Message: message})
		return false
	}
	data = bytes.TrimPrefix(data, utf8BOM)
	if len(bytes.TrimSpace(data)) == 0 {
		writeError(w, &service.APIError{
			Status: 400, Code: service.CodeValidation, Message: "Пустой запрос",
		})
		return false
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeError(w, &service.APIError{
			Status: 400, Code: service.CodeValidation, Message: "Некорректный запрос",
		})
		return false
	}
	return true
}

// utf8BOM is the byte order mark some Windows tools prepend.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// limit runs the per-IP token bucket and writes the 429 envelope when needed.
func (s *Server) limit(w http.ResponseWriter, r *http.Request, bucket string, limit int, window time.Duration, message string) bool {
	if limit <= 0 {
		return true
	}
	if s.limiter == nil {
		return true // no limiter wired (tests): never block
	}
	key := bucket + "|" + s.clientIP(r)
	allowed, retryAfter := s.limiter.allow(key, limit, window)
	if allowed {
		return true
	}
	writeError(w, service.RateLimited(retryAfter, message))
	return false
}
