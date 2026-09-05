// Package obs wires observability. For now that is a structured slog logger.
package obs

import (
	"log/slog"
	"os"
	"strings"
)

// NewLogger returns a JSON slog logger at the given level (debug|info|warn|error).
func NewLogger(level string) *slog.Logger {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl, AddSource: true})
	return slog.New(h)
}
