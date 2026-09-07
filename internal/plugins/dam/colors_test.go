package dam_test

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"strings"
	"testing"

	"github.com/Everlasting-Elysium/hetu/internal/color"
)

// swatchResult mirrors the {hex,weight} objects GET /assets/{id}/colors returns,
// so a test can decode the palette JSON and assert its shape and order.
type swatchResult struct {
	Hex    string  `json:"hex"`
	Weight float64 `json:"weight"`
}

// getColorsRaw performs GET url, asserts the status, and returns the response
// body with its trailing newline trimmed. Returning the raw body (rather than a
// decoded slice) lets the no-palette test distinguish "[]" from "null".
func getColorsRaw(t *testing.T, url string, wantStatus int) string {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if resp.StatusCode != wantStatus {
		t.Fatalf("GET %s status = %d, want %d; body=%s", url, resp.StatusCode, wantStatus, body)
	}
	return strings.TrimSpace(string(body))
}

func TestAssetColors_HasPalette(t *testing.T) {
	srv, owner, st := newTestServer(t)
	aid := upsertThumbAsset(t, st, owner, "palette-asset")

	// Given: a palette seeded dominant-first (descending weight). writePaletteTx
	// stores ord = slice index, so the endpoint must echo this exact sequence.
	pal := color.Palette{
		{RGB: color.RGB{R: 0xff, G: 0x00, B: 0x00}, Weight: 0.6}, // #ff0000
		{RGB: color.RGB{R: 0x00, G: 0x80, B: 0x00}, Weight: 0.3}, // #008000
		{RGB: color.RGB{R: 0x00, G: 0x00, B: 0xff}, Weight: 0.1}, // #0000ff
	}
	if err := st.IndexPaletteByID(context.Background(), owner, aid, pal); err != nil {
		t.Fatalf("seed palette: %v", err)
	}

	// When: the palette is fetched.
	body := getColorsRaw(t, srv.URL+"/api/dam/assets/palette-asset/colors", http.StatusOK)
	var got []swatchResult
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("decode %q: %v", body, err)
	}

	// Then: 200 with the swatches in ord order, dominant first, hex + weight intact.
	want := []swatchResult{
		{Hex: "#ff0000", Weight: 0.6},
		{Hex: "#008000", Weight: 0.3},
		{Hex: "#0000ff", Weight: 0.1},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d swatches, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i].Hex != want[i].Hex || math.Abs(got[i].Weight-want[i].Weight) > 1e-9 {
			t.Errorf("swatch[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestAssetColors_NoPalette(t *testing.T) {
	srv, owner, st := newTestServer(t)
	upsertThumbAsset(t, st, owner, "bare-asset")

	// Given an asset with no palette, When fetched, Then 200 with an empty JSON
	// array — "[]" and never "null", so a client always iterates a list.
	body := getColorsRaw(t, srv.URL+"/api/dam/assets/bare-asset/colors", http.StatusOK)
	if body != "[]" {
		t.Fatalf("body = %q, want %q (empty array, not null)", body, "[]")
	}
}

func TestAssetColors_NotFound(t *testing.T) {
	srv, _, _ := newTestServer(t)

	// Given no asset with this id, When fetched, Then 404 (GetAsset -> ErrNotFound).
	getColorsRaw(t, srv.URL+"/api/dam/assets/no-such-asset/colors", http.StatusNotFound)
}
