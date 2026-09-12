package store_test

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Everlasting-Elysium/hetu/internal/color"
	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/store"
)

// mustHex parses a hex color for a test, failing on a malformed literal.
func mustHex(t *testing.T, s string) color.RGB {
	t.Helper()
	rgb, err := color.ParseHex(s)
	if err != nil {
		t.Fatalf("parse hex %q: %v", s, err)
	}
	return rgb
}

// paletteHexes reads an asset's swatches and returns their hexes, ord-ascending,
// so a test can assert both the count and the order in one comparison.
func paletteHexes(t *testing.T, ctx context.Context, st *store.SQLite, owner domain.OwnerID, id domain.AssetID) []string {
	t.Helper()
	sw, err := st.ListAssetColors(ctx, owner, id)
	if err != nil {
		t.Fatalf("list asset colors: %v", err)
	}
	out := make([]string, len(sw))
	for i, s := range sw {
		out[i] = s.Hex()
	}
	return out
}

func swatchHexes(sw []color.Swatch) []string {
	out := make([]string, len(sw))
	for i, s := range sw {
		out[i] = s.Hex()
	}
	return out
}

func TestSQLite_AddAssetColor(t *testing.T) {
	ctx, st, owner := mustOpen(t)

	// Given: an asset whose palette starts empty. When the first color is added,
	// Then it takes ord 0 and the returned palette holds just that swatch.
	empty := seedAsset(t, ctx, st, owner, "empty", "empty.png")
	got, err := st.AddAssetColor(ctx, owner, empty, mustHex(t, "#123456"))
	if err != nil {
		t.Fatalf("add to empty: %v", err)
	}
	if want := []string{"#123456"}; !reflect.DeepEqual(swatchHexes(got), want) {
		t.Fatalf("after first add = %v, want %v", swatchHexes(got), want)
	}

	// Given: a two-swatch extracted palette. When a color is added, Then it is
	// appended at the end (next ord) and the existing swatches keep their order.
	aid := seedAsset(t, ctx, st, owner, "a", "a.png")
	if err := st.IndexPaletteByID(ctx, owner, aid, color.Palette{
		{RGB: mustHex(t, "#ff0000"), Weight: 0.6},
		{RGB: mustHex(t, "#00ff00"), Weight: 0.4},
	}); err != nil {
		t.Fatalf("seed palette: %v", err)
	}
	got, err = st.AddAssetColor(ctx, owner, aid, mustHex(t, "#0000ff"))
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if want := []string{"#ff0000", "#00ff00", "#0000ff"}; !reflect.DeepEqual(swatchHexes(got), want) {
		t.Fatalf("after add = %v, want %v", swatchHexes(got), want)
	}
}

func TestSQLite_UpdateAssetColor(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	aid := seedAsset(t, ctx, st, owner, "a", "a.png")
	if err := st.IndexPaletteByID(ctx, owner, aid, color.Palette{
		{RGB: mustHex(t, "#ff0000"), Weight: 0.6},
		{RGB: mustHex(t, "#00ff00"), Weight: 0.4},
	}); err != nil {
		t.Fatalf("seed palette: %v", err)
	}

	// When ord 1 is repointed, Then only that swatch changes and its stored Lab
	// tracks the new color (a later color search must find it near cyan).
	got, err := st.UpdateAssetColor(ctx, owner, aid, 1, mustHex(t, "#00ffff"))
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if want := []string{"#ff0000", "#00ffff"}; !reflect.DeepEqual(swatchHexes(got), want) {
		t.Fatalf("after update = %v, want %v", swatchHexes(got), want)
	}
	near, err := st.SearchByColor(ctx, owner, mustHex(t, "#00ffff").Lab(), 5, 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(near) != 1 || near[0].Asset.ID != aid {
		t.Fatalf("cyan search after update = %+v, want the edited asset (Lab was recomputed)", near)
	}

	// A swatch index past the end is not found.
	if _, err := st.UpdateAssetColor(ctx, owner, aid, 9, mustHex(t, "#000000")); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("update missing ord err = %v, want ErrNotFound", err)
	}
}

func TestSQLite_DeleteAssetColor(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	// A fresh asset per scenario: the first delete flips palette_manual=1, which
	// (correctly) makes a re-seed via IndexPaletteByID a no-op, so scenarios must
	// not share an asset.
	seed := func(id string) domain.AssetID {
		aid := seedAsset(t, ctx, st, owner, id, id+".png")
		if err := st.IndexPaletteByID(ctx, owner, aid, color.Palette{
			{RGB: mustHex(t, "#ff0000"), Weight: 0.5},
			{RGB: mustHex(t, "#00ff00"), Weight: 0.3},
			{RGB: mustHex(t, "#0000ff"), Weight: 0.2},
		}); err != nil {
			t.Fatalf("seed palette: %v", err)
		}
		return aid
	}

	// Deleting a middle swatch closes the gap: survivors keep their relative
	// order and are renumbered to a contiguous 0..N-1 (green -> ord 1 collapses).
	mid := seed("mid")
	got, err := st.DeleteAssetColor(ctx, owner, mid, 1)
	if err != nil {
		t.Fatalf("delete middle: %v", err)
	}
	if want := []string{"#ff0000", "#0000ff"}; !reflect.DeepEqual(swatchHexes(got), want) {
		t.Fatalf("after delete middle = %v, want %v", swatchHexes(got), want)
	}

	// Deleting ord 0 (the dominant) promotes the next swatch to the new ord 0, so
	// the dominant-color slot is never left empty.
	dom := seed("dom")
	got, err = st.DeleteAssetColor(ctx, owner, dom, 0)
	if err != nil {
		t.Fatalf("delete dominant: %v", err)
	}
	if want := []string{"#00ff00", "#0000ff"}; !reflect.DeepEqual(swatchHexes(got), want) {
		t.Fatalf("after delete dominant = %v, want %v", swatchHexes(got), want)
	}

	// A swatch index past the end is not found.
	if _, err := st.DeleteAssetColor(ctx, owner, mid, 9); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("delete missing ord err = %v, want ErrNotFound", err)
	}
}

// TestSQLite_ManualPaletteSurvivesRescan locks the load-bearing invariant: once a
// palette is edited by hand (palette_manual = 1), neither IndexPaletteByID nor
// IndexPalette overwrites it — the curated asset_colors rows AND the extracted
// annotations are both left untouched (the skip is all-or-nothing).
func TestSQLite_ManualPaletteSurvivesRescan(t *testing.T) {
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

	aid := seedAsset(t, ctx, st, owner, "m1", "m1.png")
	if err := st.IndexPaletteByID(ctx, owner, aid, color.Palette{
		{RGB: mustHex(t, "#ff0000"), Weight: 0.7},
		{RGB: mustHex(t, "#00ff00"), Weight: 0.3},
	}); err != nil {
		t.Fatalf("auto index: %v", err)
	}
	// A hand edit flips palette_manual and yields [blue, green].
	if _, err := st.UpdateAssetColor(ctx, owner, aid, 0, mustHex(t, "#0000ff")); err != nil {
		t.Fatalf("manual edit: %v", err)
	}

	// Both re-index entry points must now be no-ops for this asset.
	if err := st.IndexPaletteByID(ctx, owner, aid, color.Palette{{RGB: mustHex(t, "#ffff00"), Weight: 1}}); err != nil {
		t.Fatalf("rescan by id: %v", err)
	}
	if err := st.IndexPalette(ctx, owner, "local", "m1.png", color.Palette{{RGB: mustHex(t, "#00ffff"), Weight: 1}}); err != nil {
		t.Fatalf("rescan by path: %v", err)
	}

	// asset_colors is untouched: still the curated [blue, green], not the yellow
	// or cyan a naive re-scan would have written.
	if want := []string{"#0000ff", "#00ff00"}; !reflect.DeepEqual(paletteHexes(t, ctx, st, owner, aid), want) {
		t.Fatalf("after rescans = %v, want the curated %v", paletteHexes(t, ctx, st, owner, aid), want)
	}
	// annotations are untouched too (all-or-nothing skip): the dominant still
	// reflects the ORIGINAL extracted red, proving upsertPaletteAnnotations was
	// also skipped rather than overwritten to yellow/cyan.
	assertPaletteAnnotations(t, ctx, dbPath, "m1", "#ff0000")
}
