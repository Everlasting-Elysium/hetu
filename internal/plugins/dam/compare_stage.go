package dam

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

// compareTTL bounds how long a staged comparison's inputs live on disk. One hour
// keeps an observable window — long enough to re-open the overlay while reviewing
// a result — without letting random one-shot reqIDs accumulate unboundedly.
const compareTTL = time.Hour

// stageCompareImage writes one side's bytes to <CompareDir>/<reqID>/<side>.<ext>
// for the overlay endpoint to serve back, returning the staged path.
func (p *Plugin) stageCompareImage(reqID, side string, s sideData) (string, error) {
	dir := filepath.Join(p.k.CompareDir, reqID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create compare dir: %w", err)
	}
	path := filepath.Join(dir, side+"."+s.ext)
	if err := os.WriteFile(path, s.raw, 0o600); err != nil {
		return "", fmt.Errorf("write staged image: %w", err)
	}
	return path, nil
}

// sweepCompareDir opportunistically removes staged comparison dirs older than
// compareTTL, judged by each dir's mtime (bumped when its files are written).
// It runs at the start of each POST /compare so cleanup rides normal traffic
// rather than a long-lived goroutine/ticker entangled with server lifecycle.
// Every failure is logged and swallowed — a cleanup problem (e.g. permissions)
// must never fail the comparison in progress. A missing CompareDir is normal on
// first use and not logged.
func (p *Plugin) sweepCompareDir(ctx context.Context) {
	entries, err := os.ReadDir(p.k.CompareDir)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			p.k.Log.WarnContext(ctx, "compare: sweep read dir", slog.Any("err", err))
		}
		return
	}
	cutoff := time.Now().Add(-compareTTL)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			dir := filepath.Join(p.k.CompareDir, e.Name())
			if err := os.RemoveAll(dir); err != nil {
				p.k.Log.WarnContext(ctx, "compare: sweep remove",
					slog.String("dir", dir), slog.Any("err", err))
			}
		}
	}
}

// cleanupCompareDir removes a single request's staged dir. It is used to roll
// back a partially-staged request when staging fails mid-way.
func (p *Plugin) cleanupCompareDir(reqID string) {
	_ = os.RemoveAll(filepath.Join(p.k.CompareDir, reqID))
}
