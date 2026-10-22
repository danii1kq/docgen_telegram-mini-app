package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestOrderReadNeedsTokenOrSession checks that the order number is not a
// public key: the numbers are sequential (DG-26-0391), so anyone could
// enumerate the orders and their topics. Without a token and without a signed-in
// Telegram chat the API answers the neutral not-found state.
func TestOrderReadNeedsTokenOrSession(t *testing.T) {
	server := testServer(t)

	request := httptest.NewRequest(http.MethodGet, "/api/orders/DG-260391", nil)
	recorder := httptest.NewRecorder()
	server.handleGetOrder(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (the order must not be public)", recorder.Code)
	}
	if body := recorder.Body.String(); !containsHeader(body, "not_found") {
		t.Errorf("body = %q, want a not_found envelope", body)
	}
}

// TestOrderLookupIsRateLimited checks that the token-guessing endpoints are
// throttled: the order token is six characters, so a full-speed brute force
// must not be possible.
func TestOrderLookupIsRateLimited(t *testing.T) {
	server := testServer(t)
	server.cfg.OrderLookupPerMin = 2

	status := 0
	for i := 0; i < 4; i++ {
		request := httptest.NewRequest(http.MethodGet, "/api/orders/DG-260391", nil)
		recorder := httptest.NewRecorder()
		server.handleGetOrder(recorder, request)
		status = recorder.Code
	}
	if status != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429 after the limit is reached", status)
	}
}
