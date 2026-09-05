// Package api builds the HTTP transport: a chi router that mounts each enabled
// plugin under /api/<name>, plus a health check, structured request logging,
// and an embedded SPA fallback for the web UI.
package api

import (
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/Everlasting-Elysium/hetu/internal/httpjson"
	"github.com/Everlasting-Elysium/hetu/internal/kernel"
)

// NewRouter builds the router and mounts the given plugins. webFS is the
// embedded frontend build output (web/dist); it is served as a SPA fallback
// for any path not matched by the API or health check.
func NewRouter(k *kernel.Kernel, plugins []kernel.Plugin, webFS fs.FS) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.Recoverer, cors, requestLogger(k.Log))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		httpjson.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
	for _, p := range plugins {
		mount(r, p)
		if tlr, ok := p.(kernel.TopLevelRouter); ok {
			for _, route := range tlr.TopLevelRoutes() {
				r.Method(route.Method, route.Pattern, route.Handler)
			}
		}
	}
	r.NotFound(spaHandler(webFS))
	return r
}

func mount(r chi.Router, p kernel.Plugin) {
	r.Route("/api/"+p.Name(), func(sub chi.Router) {
		for _, route := range p.Routes() {
			sub.Method(route.Method, route.Pattern, route.Handler)
		}
	})
}

// cors is a permissive CORS middleware for development (Vite dev server on a
// different port). In production the SPA is served from the same origin.
func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// spaHandler returns an http.HandlerFunc that serves static files from webFS,
// falling back to index.html for client-side routing paths. Hashed assets
// (js/css) get long-lived cache headers; index.html is never cached.
func spaHandler(webFS fs.FS) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		// Try the exact file first.
		f, err := webFS.Open(path)
		if err != nil {
			// Not a real file — serve index.html for SPA routing.
			serveIndex(w, r, webFS)
			return
		}
		f.Close()

		// Hashed assets are immutable; index.html must never be cached.
		if path != "index.html" {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		http.FileServerFS(webFS).ServeHTTP(w, r)
	}
}

func serveIndex(w http.ResponseWriter, r *http.Request, webFS fs.FS) {
	w.Header().Set("Cache-Control", "no-cache")
	r.URL.Path = "/"
	http.FileServerFS(webFS).ServeHTTP(w, r)
}

func requestLogger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			next.ServeHTTP(w, r)
			log.InfoContext(r.Context(), "http request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Duration("elapsed", time.Since(start)))
		})
	}
}
