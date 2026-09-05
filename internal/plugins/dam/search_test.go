package dam_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/api"
	"github.com/Everlasting-Elysium/hetu/internal/color"
	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/kernel"
	"github.com/Everlasting-Elysium/hetu/internal/plugins/dam"
	"github.com/Everlasting-Elysium/hetu/internal/store"
)

func TestSearchByColor_HTTP(t *testing.T) {
	router, _ := seedRouter(t)

	// A red query returns the red asset with the matched swatch and distance.
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/dam/search?color=ff0000&tol=15", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var got []struct {
		Name     string  `json:"name"`
		MatchHex string  `json:"match_hex"`
		Distance float64 `json:"color_distance"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v (body=%s)", err, rec.Body.String())
	}
	if len(got) != 1 || got[0].Name != "red.png" {
		t.Fatalf("results = %+v, want only red.png", got)
	}
	if got[0].MatchHex != "#ff0000" || got[0].Distance > 2 {
		t.Fatalf("match = %+v, want #ff0000 near 0", got[0])
	}
}

func TestSearchByColor_BadRequest(t *testing.T) {
	router, _ := seedRouter(t)
	for _, q := range []string{"/api/dam/search", "/api/dam/search?color=nothex"} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, q, nil))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("GET %s status = %d, want 400", q, rec.Code)
		}
	}
}

// seedRouter builds a router over a store holding one red and one blue asset.
func seedRouter(t *testing.T) (http.Handler, domain.OwnerID) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	owner, err := domain.NewOwnerID("t")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureOwner(ctx, owner); err != nil {
		t.Fatal(err)
	}
	seed := func(id, path string, c color.RGB) {
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
			t.Fatal(err)
		}
		if err := st.IndexPalette(ctx, owner, "local", path, color.Palette{{RGB: c, Weight: 1}}); err != nil {
			t.Fatal(err)
		}
	}
	seed("red", "red.png", color.RGB{R: 255})
	seed("blue", "blue.png", color.RGB{B: 255})

	k := kernel.New(kernel.Deps{
		Log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:     st,
		JobBuffer: 1,
	})
	p := dam.New(owner)
	if err := p.Init(ctx, k); err != nil {
		t.Fatal(err)
	}
	return api.NewRouter(k, []kernel.Plugin{p}, fstest.MapFS{}), owner
}
