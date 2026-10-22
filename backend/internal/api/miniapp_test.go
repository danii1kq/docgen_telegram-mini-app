package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// buildWebapp lays out a miniature Vite dist tree.
func buildWebapp(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatalf("mkdir assets: %v", err)
	}
	files := map[string]string{
		"index.html":             "<!doctype html><div id=root></div>",
		"assets/index-abc123.js": "console.log('app');",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(name)), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return dir
}

func TestNewSPAHandlerDisabled(t *testing.T) {
	if handler := newSPAHandler(""); handler != nil {
		t.Fatal("empty dir must disable the SPA handler")
	}
	if handler := newSPAHandler(filepath.Join(t.TempDir(), "missing")); handler != nil {
		t.Fatal("a missing dir must disable the SPA handler")
	}
	// A directory without index.html is not a built app either.
	empty := t.TempDir()
	if handler := newSPAHandler(empty); handler != nil {
		t.Fatal("a dir without index.html must disable the SPA handler")
	}
}

func TestSPAHandlerCharArray(t *testing.T) {
	handler := newSPAHandler(buildWebapp(t))
	if handler == nil {
		t.Fatal("handler is nil for a valid build")
	}

	cases := []struct {
		path        string
		wantStatus  int
		wantBodyHas string
		wantCache   string
	}{
		{"/", http.StatusOK, "<div id=root></div>", "no-cache"},
		{"/assets/index-abc123.js", http.StatusOK, "console.log", "public, max-age=31536000, immutable"},
		{"/order/DG-260391", http.StatusOK, "<div id=root></div>", "no-cache"}, // SPA fallback
	}

	for _, testCase := range cases {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, testCase.path, nil)
		handler.ServeHTTP(recorder, request)

		response := recorder.Result()
		body, _ := io.ReadAll(response.Body)
		_ = response.Body.Close()

		if response.StatusCode != testCase.wantStatus {
			t.Fatalf("%s: status = %d, want %d", testCase.path, response.StatusCode, testCase.wantStatus)
		}
		if !strings.Contains(string(body), testCase.wantBodyHas) {
			t.Fatalf("%s: body %q does not contain %q", testCase.path, string(body), testCase.wantBodyHas)
		}
		if cache := response.Header.Get("Cache-Control"); cache != testCase.wantCache {
			t.Fatalf("%s: cache-control = %q, want %q", testCase.path, cache, testCase.wantCache)
		}
	}
}

// Traversal-looking URLs must never reach the file system: http.ServeFile
// rejects them before opening anything.
func TestSPAHandlerRejectsTraversal(t *testing.T) {
	handler := newSPAHandler(buildWebapp(t))
	if handler == nil {
		t.Fatal("handler is nil for a valid build")
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/../main.go", nil)
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}
