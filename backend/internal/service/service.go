// Package service holds the business logic used by the HTTP API, the Telegram
// bot and the background worker: quoting, order creation, promo checks, the
// state machine and notifications.
package service

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/srunal/srunal-go/internal/config"
	"github.com/srunal/srunal-go/internal/store"
)

// Service bundles the store, configuration and logger.
type Service struct {
	Store *store.Store
	Cfg   *config.Config
	Log   *slog.Logger
	// rates caches the USD -> KZT exchange rate for the operator panel.
	rates rateCache
}

// New builds the service.
func New(st *store.Store, cfg *config.Config, log *slog.Logger) *Service {
	return &Service{Store: st, Cfg: cfg, Log: log}
}

// APIError is the error envelope of the API:
// {"error":{"code":"...","message":"...","fields":{...}}}
type APIError struct {
	Status  int               `json:"-"`
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
	// RetryAfter is sent as the Retry-After header for 429 responses.
	RetryAfter int `json:"-"`
}

func (e *APIError) Error() string { return e.Code + ": " + e.Message }

// Error codes from README section 7.
const (
	CodeValidation   = "validation_failed"
	CodeNotFound     = "not_found"
	CodeMaintenance  = "maintenance"
	CodeRateLimited  = "rate_limited"
	CodeInvalidPromo = "invalid_promo"
	CodeUnauthorized = "unauthorized"
	CodeConflict     = "conflict"
	CodeInternal     = "internal"
)

// ValidationError builds the standard 422 response.
func ValidationError(fields map[string]string) *APIError {
	return &APIError{
		Status:  422,
		Code:    CodeValidation,
		Message: "Проверьте выделенные поля",
		Fields:  fields,
	}
}

// NotFound builds a 404 with the text the client shows.
func NotFound(message string) *APIError {
	return &APIError{Status: 404, Code: CodeNotFound, Message: message}
}

// Maintenance builds the 503 shown when the order form is switched off.
func Maintenance() *APIError {
	return &APIError{
		Status:  503,
		Code:    CodeMaintenance,
		Message: "Приём заказов временно приостановлен, попробуйте позже",
	}
}

// RateLimited builds a 429 with Retry-After seconds.
func RateLimited(retryAfter int, message string) *APIError {
	return &APIError{Status: 429, Code: CodeRateLimited, Message: message, RetryAfter: retryAfter}
}

// Unauthorized builds a 401; the frontend shows the same text for a wrong
// operator id and a wrong password (README section 5.6).
func Unauthorized(message string) *APIError {
	return &APIError{Status: 401, Code: CodeUnauthorized, Message: message}
}

// Forbidden answers "authenticated, but not an operator" (the Mini App panel
// gate).
func Forbidden(message string) *APIError {
	return &APIError{Status: 403, Code: CodeUnauthorized, Message: message}
}

// Conflict builds a 409 for impossible state changes.
func Conflict(message string) *APIError {
	return &APIError{Status: 409, Code: CodeConflict, Message: message}
}

// Internal builds a 500 without leaking details.
func Internal() *APIError {
	return &APIError{
		Status:  500,
		Code:    CodeInternal,
		Message: "Внутренняя ошибка сервиса, попробуйте позже",
	}
}

// FromStore maps store errors onto API errors.
func FromStore(err error, notFoundMessage string) *APIError {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, store.ErrNotFound), errors.Is(err, store.ErrWrongToken):
		return NotFound(notFoundMessage)
	case errors.Is(err, store.ErrConflict):
		return &APIError{Status: 409, Code: CodeConflict, Message: "Такая запись уже существует"}
	case errors.Is(err, store.ErrInvalidTransition):
		return Conflict("Это действие недоступно для текущего статуса заказа")
	default:
		return Internal()
	}
}

// StoragePath resolves a path inside the configured storage directory.
func (s *Service) StoragePath(elem ...string) string {
	parts := append([]string{s.Cfg.StorageDir}, elem...)
	return filepath.Join(parts...)
}

// EnsureStorage creates the storage directory tree.
func (s *Service) EnsureStorage() error {
	for _, dir := range []string{
		s.Cfg.StorageDir,
		s.StoragePath("results"),
		s.StoragePath("settings"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("service: storage dir %s: %w", dir, err)
		}
	}
	return nil
}

// DiskUsage walks the storage directory for the admin stats screen.
func (s *Service) DiskUsage() int64 {
	var total int64
	_ = filepath.WalkDir(s.Cfg.StorageDir, func(_ string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		if info, err := entry.Info(); err == nil {
			total += info.Size()
		}
		return nil
	})
	return total
}
