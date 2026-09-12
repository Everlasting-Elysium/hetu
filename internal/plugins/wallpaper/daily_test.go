package wallpaper

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

// getDaily decodes the single-object /daily response.
func (e *testEnv) getDaily(path string) wallpaperDTO {
	e.t.Helper()
	resp := e.get(path)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		e.t.Fatalf("GET %s = %d, want 200", path, resp.StatusCode)
	}
	var d wallpaperDTO
	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		e.t.Fatalf("decode daily: %v", err)
	}
	return d
}

// TestDailyDeterministic covers the date-seeded pick: same day → same asset,
// and a different day maps to a different offset. now is injected so the assert
// is exact. Seven assets with distinct indexed times give a strict latest order,
// and 20260912 % 7 = 0, 20260913 % 7 = 1, so the picks are latest[0]/latest[1].
func TestDailyDeterministic(t *testing.T) {
	e := newTestEnv(t)
	base := time.Now().UTC().Truncate(time.Second)
	ids := make([]string, 7)
	for i := range 7 {
		aid := e.add(seed{id: fmt.Sprintf("a%d", i), name: fmt.Sprintf("w%d.png", i), indexed: base.Add(time.Duration(i) * time.Second)})
		ids[i] = aid.String()
	}
	// latest order is indexed DESC, so latest[0] is the highest offset asset.
	newest, second := ids[6], ids[5]

	e.plugin.now = func() time.Time { return time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC) }
	d1 := e.getDaily("/api/wallpaper/daily")
	d2 := e.getDaily("/api/wallpaper/daily")
	if d1.ID != d2.ID {
		t.Errorf("same day gave %s then %s, want identical", d1.ID, d2.ID)
	}
	if d1.ID != newest {
		t.Errorf("2026-09-12 daily = %s, want idx0 %s", d1.ID, newest)
	}

	e.plugin.now = func() time.Time { return time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC) }
	d3 := e.getDaily("/api/wallpaper/daily")
	if d3.ID != second {
		t.Errorf("2026-09-13 daily = %s, want idx1 %s", d3.ID, second)
	}
	if d3.ID == d1.ID {
		t.Error("different days resolved to the same asset, want different")
	}
}

// TestDailyEmpty404 covers the no-match branch: an empty population is 404, not
// a 500 or an empty 200.
func TestDailyEmpty404(t *testing.T) {
	e := newTestEnv(t)
	resp := e.get("/api/wallpaper/daily")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("empty daily = %d, want 404", resp.StatusCode)
	}
}
