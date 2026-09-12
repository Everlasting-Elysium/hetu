package dam_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/color"
	"github.com/Everlasting-Elysium/hetu/internal/domain"
)

// editColors performs a JSON method request against url, asserts the status, and
// decodes a 200 body into the swatch list every manual-edit endpoint returns.
func editColors(t *testing.T, method, url, body string, wantStatus int) []swatchResult {
	t.Helper()
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, rdr)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != wantStatus {
		t.Fatalf("%s %s status = %d, want %d; body=%s", method, url, resp.StatusCode, wantStatus, raw)
	}
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	var got []swatchResult
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("decode %q: %v", raw, err)
	}
	return got
}

func swatchResultHexes(sw []swatchResult) []string {
	out := make([]string, len(sw))
	for i, s := range sw {
		out[i] = s.Hex
	}
	return out
}

// seedEditablePalette upserts a palette-bearing (model) asset and seeds pal.
func seedEditablePalette(t *testing.T, st interface {
	IndexPaletteByID(context.Context, domain.OwnerID, domain.AssetID, color.Palette) error
}, owner domain.OwnerID, aid domain.AssetID, pal color.Palette) {
	t.Helper()
	if err := st.IndexPaletteByID(context.Background(), owner, aid, pal); err != nil {
		t.Fatalf("seed palette: %v", err)
	}
}

func TestManualPaletteEdit_AddUpdateDelete(t *testing.T) {
	srv, owner, st := newTestServer(t)
	aid := upsertThumbAsset(t, st, owner, "edit-asset")
	seedEditablePalette(t, st, owner, aid, color.Palette{
		{RGB: color.RGB{R: 0xff}, Weight: 0.6}, // #ff0000
		{RGB: color.RGB{G: 0x80}, Weight: 0.4}, // #008000
	})
	base := srv.URL + "/api/dam/assets/edit-asset/colors"

	// Add appends a swatch at the end (next ord).
	got := editColors(t, http.MethodPost, base, `{"hex":"#0000ff"}`, http.StatusOK)
	if h := swatchResultHexes(got); !reflect.DeepEqual(h, []string{"#ff0000", "#008000", "#0000ff"}) {
		t.Fatalf("after add = %v", h)
	}

	// Update repoints ord 1 only.
	got = editColors(t, http.MethodPut, base+"/1", `{"hex":"#00ffff"}`, http.StatusOK)
	if h := swatchResultHexes(got); !reflect.DeepEqual(h, []string{"#ff0000", "#00ffff", "#0000ff"}) {
		t.Fatalf("after update = %v", h)
	}

	// Delete ord 0 (dominant) renumbers survivors to a contiguous 0..N-1.
	got = editColors(t, http.MethodDelete, base+"/0", "", http.StatusOK)
	if h := swatchResultHexes(got); !reflect.DeepEqual(h, []string{"#00ffff", "#0000ff"}) {
		t.Fatalf("after delete = %v", h)
	}

	// The read endpoint reflects the same curated palette.
	var final []swatchResult
	if err := json.Unmarshal([]byte(getColorsRaw(t, base, http.StatusOK)), &final); err != nil {
		t.Fatalf("decode final: %v", err)
	}
	if h := swatchResultHexes(final); !reflect.DeepEqual(h, []string{"#00ffff", "#0000ff"}) {
		t.Fatalf("GET after edits = %v", h)
	}
}

func TestManualPaletteEdit_AddToEmpty(t *testing.T) {
	srv, owner, st := newTestServer(t)
	upsertThumbAsset(t, st, owner, "empty-asset")
	base := srv.URL + "/api/dam/assets/empty-asset/colors"

	// An asset with no extracted palette can still start a manual one at ord 0.
	got := editColors(t, http.MethodPost, base, `{"hex":"#abcdef"}`, http.StatusOK)
	if h := swatchResultHexes(got); !reflect.DeepEqual(h, []string{"#abcdef"}) {
		t.Fatalf("add to empty = %v, want [#abcdef]", h)
	}
}

// TestManualPaletteEdit_ProtectsFromRescan proves an HTTP edit flips
// palette_manual so a subsequent scan leaves the curated swatches intact.
func TestManualPaletteEdit_ProtectsFromRescan(t *testing.T) {
	srv, owner, st := newTestServer(t)
	aid := upsertThumbAsset(t, st, owner, "guard-asset")
	seedEditablePalette(t, st, owner, aid, color.Palette{{RGB: color.RGB{R: 0xff}, Weight: 1}})
	base := srv.URL + "/api/dam/assets/guard-asset/colors"

	editColors(t, http.MethodPost, base, `{"hex":"#00ff00"}`, http.StatusOK)
	// A re-scan with a completely different palette must be a no-op now.
	if err := st.IndexPaletteByID(context.Background(), owner, aid, color.Palette{
		{RGB: color.RGB{B: 0xff}, Weight: 1},
	}); err != nil {
		t.Fatalf("rescan: %v", err)
	}
	var got []swatchResult
	if err := json.Unmarshal([]byte(getColorsRaw(t, base, http.StatusOK)), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if h := swatchResultHexes(got); !reflect.DeepEqual(h, []string{"#ff0000", "#00ff00"}) {
		t.Fatalf("after rescan = %v, want curated [#ff0000 #00ff00]", h)
	}
}

func TestManualPaletteEdit_NotFound(t *testing.T) {
	srv, _, _ := newTestServer(t)
	base := srv.URL + "/api/dam/assets/ghost/colors"
	editColors(t, http.MethodPost, base, `{"hex":"#ff0000"}`, http.StatusNotFound)
	editColors(t, http.MethodPut, base+"/0", `{"hex":"#ff0000"}`, http.StatusNotFound)
	editColors(t, http.MethodDelete, base+"/0", "", http.StatusNotFound)
}

// TestManualPaletteEdit_CrossOwner proves the endpoints are owner-scoped: the
// server's owner cannot touch an asset another owner owns (answered 404, never
// leaked or mutated).
func TestManualPaletteEdit_CrossOwner(t *testing.T) {
	srv, _, st := newTestServer(t)
	other, err := domain.NewOwnerID("intruder")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := st.EnsureOwner(ctx, other); err != nil {
		t.Fatal(err)
	}
	oid, err := domain.NewAssetID("other-asset")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	if err := st.UpsertAsset(ctx, domain.Asset{
		ID: oid, Owner: other, Kind: domain.KindImage, Provider: "local",
		StoragePath: "other.png", Name: "other.png", Ext: "png", Size: 1, Hash: "h-other",
		CreatedAt: now, IndexedAt: now,
	}); err != nil {
		t.Fatalf("upsert other-owner asset: %v", err)
	}
	base := srv.URL + "/api/dam/assets/other-asset/colors"
	editColors(t, http.MethodPost, base, `{"hex":"#ff0000"}`, http.StatusNotFound)
	editColors(t, http.MethodPut, base+"/0", `{"hex":"#ff0000"}`, http.StatusNotFound)
	editColors(t, http.MethodDelete, base+"/0", "", http.StatusNotFound)
}

// TestManualPaletteEdit_AudioRejected: a kind that carries no palette (audio,
// #88) is rejected with 400, not a 500.
func TestManualPaletteEdit_AudioRejected(t *testing.T) {
	srv, owner, st := newTestServer(t)
	aid, err := domain.NewAssetID("audio-edit")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	if err := st.UpsertAsset(context.Background(), domain.Asset{
		ID: aid, Owner: owner, Kind: domain.KindAudio, Provider: "local",
		StoragePath: "song.mp3", Name: "song.mp3", Ext: "mp3", Size: 1, Hash: "h-audio-edit",
		CreatedAt: now, IndexedAt: now,
	}); err != nil {
		t.Fatalf("upsert audio: %v", err)
	}
	editColors(t, http.MethodPost, srv.URL+"/api/dam/assets/audio-edit/colors", `{"hex":"#ff0000"}`, http.StatusBadRequest)
}

func TestManualPaletteEdit_BadInput(t *testing.T) {
	srv, owner, st := newTestServer(t)
	aid := upsertThumbAsset(t, st, owner, "bad-input")
	seedEditablePalette(t, st, owner, aid, color.Palette{{RGB: color.RGB{R: 0xff}, Weight: 1}})
	base := srv.URL + "/api/dam/assets/bad-input/colors"

	// A malformed hex is a client error on both add and update.
	editColors(t, http.MethodPost, base, `{"hex":"nope"}`, http.StatusBadRequest)
	editColors(t, http.MethodPut, base+"/0", `{"hex":"#zzzzzz"}`, http.StatusBadRequest)

	// A swatch ord past the end is 404; a negative ord is a malformed path (400).
	editColors(t, http.MethodPut, base+"/9", `{"hex":"#ff0000"}`, http.StatusNotFound)
	editColors(t, http.MethodDelete, base+"/9", "", http.StatusNotFound)
	editColors(t, http.MethodPut, base+"/-1", `{"hex":"#ff0000"}`, http.StatusBadRequest)
}
