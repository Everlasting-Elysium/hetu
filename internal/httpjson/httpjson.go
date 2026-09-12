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

// QueryInt64 reads an int64 query parameter, returning def when absent or
// invalid. File sizes can exceed the 32-bit range on some platforms, so size
// filters use this instead of QueryInt.
func QueryInt64(r *http.Request, key string, def int64) int64 {
	s := r.URL.Query().Get(key)
	if s == "" {
		return def
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return def
	}
	return n
}

// QueryFloat64 reads a float64 query parameter, returning def when absent or
// invalid. Durations are seconds and may be fractional, so the duration filter
// uses this instead of QueryInt64.
func QueryFloat64(r *http.Request, key string, def float64) float64 {
	s := r.URL.Query().Get(key)
	if s == "" {
		return def
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return def
	}
	return n
}

// NormalizeRange clamps a min/max pair to hetu's range-filter contract:
// negative values collapse to 0 (unbounded on that side), and an inverted range
// (max>0 AND max<min) drops max back to 0 (unbounded) rather than swapping the
// two — a confused range widens instead of silently reinterpreting the caller's
// numbers in swapped roles. Shared by the DAM facet parser and the wallpaper
// filter (issue #114); it lives here alongside the QueryInt/QueryFloat64
// helpers because normalizing query-derived ranges is the same concern.
func NormalizeRange(min, max int64) (int64, int64) {
	if min < 0 {
		min = 0
	}
	if max < 0 {
		max = 0
	}
	if max > 0 && max < min {
		max = 0
	}
	return min, max
}

// NormalizeRangeFloat is NormalizeRange for float64 ranges (the duration facet,
// seconds — which may be fractional, so it cannot reuse the int64 version). Same
// contract: negative -> 0 (unbounded), inverted max<min -> drop max to unbounded
// rather than swapping.
func NormalizeRangeFloat(min, max float64) (float64, float64) {
	if min < 0 {
		min = 0
	}
	if max < 0 {
		max = 0
	}
	if max > 0 && max < min {
		max = 0
	}
	return min, max
}
