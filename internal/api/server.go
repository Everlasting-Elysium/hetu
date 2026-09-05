// Package api builds the HTTP transport: a chi router that mounts each enabled
// plugin under /api/<name>, plus a health check and structured request logging.
package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/Everlasting-Elysium/hetu/internal/httpjson"
	"github.com/Everlasting-Elysium/hetu/internal/kernel"
)

// NewRouter builds the router and mounts the given plugins.
func NewRouter(k *kernel.Kernel, plugins []kernel.Plugin) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.Recoverer, requestLogger(k.Log))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		httpjson.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
	for _, p := range plugins {
		mount(r, p)
	}
	return r
}

func mount(r chi.Router, p kernel.Plugin) {
	r.Route("/api/"+p.Name(), func(sub chi.Router) {
		for _, route := range p.Routes() {
			sub.Method(route.Method, route.Pattern, route.Handler)
		}
	})
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
