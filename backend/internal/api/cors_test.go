package api

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/srunal/srunal-go/internal/config"
)

func testServer(t *testing.T) *Server {
	t.Helper()
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	return &Server{
		cfg: &config.Config{
			CORSOrigins: []string{"http://localhost:5173", "https://srunal.kz"},
		},
		log: log,
	}
}

// TestCORSPreflight checks the browser preflight for a JSON POST with the
// admin bearer token - the request the admin panel makes.
func TestCORSPreflight(t *testing.T) {
	server := testServer(t)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("preflight must not reach the router")
	})
	handler := server.cors(next)

	request := httptest.NewRequest(http.MethodOptions, "/api/admin/orders", nil)
	request.Header.Set("Origin", "http://localhost:5173")
	request.Header.Set("Access-Control-Request-Method", "POST")
	request.Header.Set("Access-Control-Request-Headers", "authorization,content-type,idempotency-key")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want 204", recorder.Code)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Errorf("Allow-Origin = %q", got)
	}
	for _, header := range []string{"Authorization", "Content-Type", "Idempotency-Key"} {
		if !containsHeader(recorder.Header().Get("Access-Control-Allow-Headers"), header) {
			t.Errorf("Allow-Headers misses %s: %q", header, recorder.Header().Get("Access-Control-Allow-Headers"))
		}
	}
	if !containsHeader(recorder.Header().Get("Access-Control-Allow-Methods"), "PATCH") {
		t.Errorf("Allow-Methods misses PATCH: %q", recorder.Header().Get("Access-Control-Allow-Methods"))
	}
}

// TestCORSActualRequest checks that a normal request carries the origin header.
func TestCORSActualRequest(t *testing.T) {
	server := testServer(t)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := server.cors(next)

	request := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	request.Header.Set("Origin", "https://srunal.kz")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "https://srunal.kz" {
		t.Errorf("Allow-Origin = %q", got)
	}
	if got := recorder.Header().Get("Access-Control-Expose-Headers"); !containsHeader(got, "Retry-After") {
		t.Errorf("Expose-Headers = %q, want Retry-After", got)
	}
}

// TestCORSRejectsUnknownOrigin makes sure a foreign site gets no permission.
func TestCORSRejectsUnknownOrigin(t *testing.T) {
	server := testServer(t)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("a foreign preflight must not reach the router")
	})
	handler := server.cors(next)

	request := httptest.NewRequest(http.MethodOptions, "/api/orders", nil)
	request.Header.Set("Origin", "https://evil.example")
	request.Header.Set("Access-Control-Request-Method", "POST")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("foreign preflight status = %d, want 403", recorder.Code)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Allow-Origin must stay empty, got %q", got)
	}
}

// TestCORSSameOrigin passes requests without an Origin header through.
func TestCORSSameOrigin(t *testing.T) {
	server := testServer(t)
	reached := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	})
	handler := server.cors(next)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/orders", nil))

	if !reached || recorder.Code != http.StatusOK {
		t.Fatalf("plain request was not forwarded (reached=%v status=%d)", reached, recorder.Code)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Allow-Origin must not be set without Origin, got %q", got)
	}
}

func containsHeader(list, needle string) bool {
	return strings.Contains(strings.ToLower(list), strings.ToLower(needle))
}
