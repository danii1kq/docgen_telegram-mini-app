package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSecurityHeaders checks the response hardening every request carries.
func TestSecurityHeaders(t *testing.T) {
	server := testServer(t)
	handler := server.withMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	request := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	want := map[string]string{
		"X-Content-Type-Options":       "nosniff",
		"Referrer-Policy":              "no-referrer",
		"X-Frame-Options":              "DENY",
		"Cross-Origin-Resource-Policy": "same-origin",
	}
	for header, value := range want {
		if got := recorder.Header().Get(header); got != value {
			t.Errorf("%s = %q, want %q", header, got, value)
		}
	}
	if recorder.Header().Get("X-Request-ID") == "" {
		t.Error("no X-Request-ID header")
	}
	if got := recorder.Header().Get("Permissions-Policy"); got == "" {
		t.Error("no Permissions-Policy header")
	}
}

// TestMiniAppCSP checks the Content-Security-Policy of the Mini App shell.
func TestMiniAppCSP(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<!doctype html><html></html>"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}
	handler := newSPAHandler(dir)
	if handler == nil {
		t.Fatal("no SPA handler for a directory with index.html")
	}

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	csp := recorder.Header().Get("Content-Security-Policy")
	for _, directive := range []string{
		"default-src 'self'",
		"frame-ancestors 'none'",
		"object-src 'none'",
		"script-src 'self' https://telegram.org",
		"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com",
	} {
		if !strings.Contains(csp, directive) {
			t.Errorf("CSP misses %q: %s", directive, csp)
		}
	}
}
