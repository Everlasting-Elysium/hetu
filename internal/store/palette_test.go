package store_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/color"
	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/store"
)

func TestSQLite_IndexPaletteAndSearch(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	st, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	owner, err := domain.NewOwnerID("tester")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureOwner(ctx, owner); err != nil {
		t.Fatal(err)
	}
	seedAsset(t, ctx, st, owner, "red-asset", "red.png")
	seedAsset(t, ctx, st, owner, "blue-asset", "blue.png")

	redPal := color.Palette{
		{RGB: color.RGB{R: 255}, Weight: 0.8},
		{RGB: color.RGB{R: 200, G: 50, B: 50}, Weight: 0.2},
	}
	bluePal := color.Palette{{RGB: color.RGB{B: 255}, Weight: 1}}
	if err := st.IndexPalette(ctx, owner, "local", "red.png", redPal); err != nil {
		t.Fatalf("index red: %v", err)
	}
	if err := st.IndexPalette(ctx, owner, "local", "blue.png", bluePal); err != nil {
		t.Fatalf("index blue: %v", err)
	}

	red, _ := color.ParseHex("#ff0000")
	blue, _ := color.ParseHex("#0000ff")
	green, _ := color.ParseHex("#00ff00")

	// Searching near red returns the red asset nearest-first; blue is far enough
	// to fall outside a moderate tolerance.
	near := mustSearch(t, ctx, st, owner, red, 15, 50)
	if len(near) != 1 || near[0].Asset.StoragePath != "red.png" {
		t.Fatalf("red tol=15 = %+v, want only red.png", near)
	}
	if near[0].Distance > 2 {
		t.Fatalf("red match distance = %v, want ~0", near[0].Distance)
	}

	// Tolerance is adjustable: widening it pulls in the blue asset too, still
	// ordered nearest-first (red before blue for a red query).
	wide := mustSearch(t, ctx, st, owner, red, 200, 50)
	if len(wide) != 2 || wide[0].Asset.StoragePath != "red.png" || wide[1].Asset.StoragePath != "blue.png" {
		t.Fatalf("red tol=200 = %+v, want [red blue]", wide)
	}
	if wide[0].Distance > wide[1].Distance {
		t.Fatalf("results not sorted ascending: %v then %v", wide[0].Distance, wide[1].Distance)
	}

	// A blue query nearest-matches the blue asset.
	if b := mustSearch(t, ctx, st, owner, blue, 15, 50); len(b) != 1 || b[0].Asset.StoragePath != "blue.png" {
		t.Fatalf("blue tol=15 = %+v, want only blue.png", b)
	}

	// Green is far from both palettes: a tight tolerance yields nothing.
	if g := mustSearch(t, ctx, st, owner, green, 5, 50); len(g) != 0 {
		t.Fatalf("green tol=5 = %+v, want none", g)
	}

	// limit caps the result count.
	if lim := mustSearch(t, ctx, st, owner, red, 200, 1); len(lim) != 1 {
		t.Fatalf("limit=1 returned %d results", len(lim))
	}

	assertPaletteAnnotations(t, ctx, dbPath, "red-asset", "#ff0000")

	// Re-indexing with a new palette replaces the color index (no stale rows).
	if err := st.IndexPalette(ctx, owner, "local", "red.png", bluePal); err != nil {
		t.Fatalf("re-index red: %v", err)
	}
	if again := mustSearch(t, ctx, st, owner, red, 15, 50); len(again) != 0 {
		t.Fatalf("after re-index to blue, red tol=15 = %+v, want none", again)
	}
}

func seedAsset(t *testing.T, ctx context.Context, st *store.SQLite, owner domain.OwnerID, id, path string) {
	t.Helper()
	aid, err := domain.NewAssetID(id)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	if err := st.UpsertAsset(ctx, domain.Asset{
		ID: aid, Owner: owner, Kind: domain.KindImage, Provider: "local",
		StoragePath: path, Name: path, Ext: "png", Size: 1, Hash: id,
		CreatedAt: now, IndexedAt: now,
	}); err != nil {
		t.Fatalf("seed %s: %v", id, err)
	}
}

func mustSearch(t *testing.T, ctx context.Context, st *store.SQLite, owner domain.OwnerID, c color.RGB, tol float64, limit int) []domain.ColorMatch {
	t.Helper()
	m, err := st.SearchByColor(ctx, owner, c.Lab(), tol, limit)
	if err != nil {
		t.Fatalf("search %s: %v", c.Hex(), err)
	}
	return m
}

// assertPaletteAnnotations confirms IndexPalette wrote the extracted-layer
// palette (JSON array) and dominant (JSON hex string) required by the issue,
// read back through a separate connection to the committed database.
func assertPaletteAnnotations(t *testing.T, ctx context.Context, dbPath, assetID, wantDominant string) {
	t.Helper()
	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()

	var paletteVal string
	err = raw.QueryRowContext(ctx,
		`SELECT value FROM annotations WHERE asset_id=? AND layer='extracted' AND "key"='palette'`,
		assetID).Scan(&paletteVal)
	if err != nil {
		t.Fatalf("read palette annotation: %v", err)
	}
	var swatches []struct {
		Hex    string  `json:"hex"`
		Weight float64 `json:"weight"`
	}
	if err := json.Unmarshal([]byte(paletteVal), &swatches); err != nil || len(swatches) == 0 {
		t.Fatalf("palette value %q not a non-empty JSON array: %v", paletteVal, err)
	}

	var dominantVal string
	err = raw.QueryRowContext(ctx,
		`SELECT value FROM annotations WHERE asset_id=? AND layer='extracted' AND "key"='dominant'`,
		assetID).Scan(&dominantVal)
	if err != nil {
		t.Fatalf("read dominant annotation: %v", err)
	}
	if dominantVal != `"`+wantDominant+`"` {
		t.Fatalf("dominant = %s, want %q", dominantVal, wantDominant)
	}
}
