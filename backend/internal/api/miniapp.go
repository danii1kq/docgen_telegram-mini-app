package api

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// spaHandler serves the built Telegram Mini App (Vite output) with a
// single-page-app fallback: real files are served as-is, unknown paths receive
// index.html so the client-side router can resolve them.
//
// Hashed Vite assets under /assets/ get an immutable cache (the file name
// changes with the content); everything else stays revalidated so a fresh
// build is picked up immediately.
type spaHandler struct {
	dir   string
	files http.Handler
}

// newSPAHandler returns nil when no usable directory is configured, so the
// server keeps its plain JSON 404 behaviour outside Telegram deployments.
func newSPAHandler(dir string) http.Handler {
	trimmed := strings.TrimSpace(dir)
	if trimmed == "" {
		return nil
	}
	info, err := os.Stat(trimmed)
	if err != nil || !info.IsDir() {
		return nil
	}
	if _, err := os.Stat(filepath.Join(trimmed, "index.html")); err != nil {
		return nil
	}
	return spaHandler{dir: trimmed, files: http.FileServer(http.Dir(trimmed))}
}

func (h spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	clean := path.Clean("/" + r.URL.Path)
	full := filepath.Join(h.dir, filepath.FromSlash(clean))

	if info, err := os.Stat(full); err == nil && !info.IsDir() {
		if strings.HasPrefix(clean, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		h.files.ServeHTTP(w, r)
		return
	}

	// Client-side route (or a missing asset): hand out the shell.
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, filepath.Join(h.dir, "index.html"))
}
