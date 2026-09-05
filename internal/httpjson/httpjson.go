// Package httpjson writes JSON HTTP responses and reads typed query params.
// It is imported by the API layer and by plugins, so it depends on nothing
// internal to avoid import cycles.
package httpjson

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
)

// WriteJSON encodes v as JSON with the given status code.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("write json response", slog.Any("err", err))
	}
}

// WriteError writes err as a JSON body {"error": ...} with the given status.
func WriteError(w http.ResponseWriter, status int, err error) {
	WriteJSON(w, status, map[string]string{"error": err.Error()})
}

// QueryInt reads an int query parameter, returning def when absent or invalid.
func QueryInt(r *http.Request, key string, def int) int {
	s := r.URL.Query().Get(key)
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}
