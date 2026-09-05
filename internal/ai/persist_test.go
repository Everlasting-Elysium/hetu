package ai

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/store"
)

// aiTestStore opens a real store with an ensured owner and one seeded asset
// ("photo-1"), returning the db path for raw-SQL assertions.
func aiTestStore(t *testing.T) (context.Context, *store.SQLite, domain.OwnerID, domain.AssetID, string) {
	t.Helper()
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	st, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	owner, err := domain.NewOwnerID("tester")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureOwner(ctx, owner); err != nil {
		t.Fatalf("ensure owner: %v", err)
	}
	return ctx, st, owner, seedTestAsset(t, ctx, st, owner, "photo-1", "photos/cat.jpg"), dbPath
}

func seedTestAsset(t *testing.T, ctx context.Context, st *store.SQLite, owner domain.OwnerID, id, path string) domain.AssetID {
	t.Helper()
	aid, err := domain.NewAssetID(id)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	if err := st.UpsertAsset(ctx, domain.Asset{
		ID: aid, Owner: owner, Kind: domain.KindImage, Provider: "local",
		StoragePath: path, Name: path, Ext: "jpg", Size: 1, Hash: "h-" + id,
		CreatedAt: now, IndexedAt: now,
	}); err != nil {
		t.Fatalf("seed asset %s: %v", id, err)
	}
	return aid
}

// fakeTagSidecar serves POST /tag with real canned tags + caption, mirroring
// ai/server.py once #11 lands.
func fakeTagSidecar(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/tag" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, TagResult{
			Tags:    []Tag{{Name: "cat", Confidence: 0.98}, {Name: "animal", Confidence: 0.8}},
			Caption: "a cat on a mat",
			Model:   "stub-v1",
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func queryTagSource(t *testing.T, ctx context.Context, dbPath, assetID, tagName string) (string, bool) {
	t.Helper()
	raw := mustRawDB(t, dbPath)
	defer func() { _ = raw.Close() }()
	var source string
	err := raw.QueryRowContext(ctx,
		`SELECT at.source FROM asset_tags at JOIN tags t ON t.id = at.tag_id
		 WHERE at.asset_id = ? AND t.name = ?`, assetID, tagName).Scan(&source)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false
	}
	if err != nil {
		t.Fatalf("query tag source: %v", err)
	}
	return source, true
}

func queryAnnotation(t *testing.T, ctx context.Context, dbPath, assetID, layer, key string) (value, model string, ok bool) {
	t.Helper()
	raw := mustRawDB(t, dbPath)
	defer func() { _ = raw.Close() }()
	err := raw.QueryRowContext(ctx,
		`SELECT value, model FROM annotations WHERE asset_id=? AND layer=? AND "key"=?`,
		assetID, layer, key).Scan(&value, &model)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", false
	}
	if err != nil {
		t.Fatalf("query annotation: %v", err)
	}
	return value, model, true
}

func mustRawDB(t *testing.T, dbPath string) *sql.DB {
	t.Helper()
	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func seedManualCaption(t *testing.T, ctx context.Context, dbPath, assetID, jsonValue string) {
	t.Helper()
	raw := mustRawDB(t, dbPath)
	defer func() { _ = raw.Close() }()
	if _, err := raw.ExecContext(ctx,
		`INSERT INTO annotations (asset_id, layer, "key", value, model, created_at)
		 VALUES (?, 'manual', 'caption', ?, '', 0)`, assetID, jsonValue); err != nil {
		t.Fatalf("seed manual caption: %v", err)
	}
}

// TestTagJob_PersistsToAILayer_viaFakeSidecar is the end-to-end acceptance test:
// a real ai.Client talks to an httptest fake sidecar returning real tags and a
// caption; the ai_tag job persists them to the ai layer of a real store, while a
// pre-existing manual tag and manual caption on the same asset stay untouched.
func TestTagJob_PersistsToAILayer_viaFakeSidecar(t *testing.T) {
	ctx, st, owner, aid, dbPath := aiTestStore(t)

	// Pre-seed manual data the AI pipeline must never overwrite.
	manCat, _ := domain.NewTagID("man-cat")
	if err := st.CreateTag(ctx, domain.Tag{ID: manCat, Owner: owner, Name: "cat"}); err != nil {
		t.Fatalf("create manual tag: %v", err)
	}
	if err := st.BatchAddTags(ctx, owner, []domain.AssetID{aid}, []domain.TagID{manCat}); err != nil {
		t.Fatalf("link manual tag: %v", err)
	}
	seedManualCaption(t, ctx, dbPath, "photo-1", `"human wrote this"`)

	srv := fakeTagSidecar(t)
	asset := domain.Asset{ID: aid, Owner: owner, Kind: domain.KindImage, Provider: "local", StoragePath: "photos/cat.jpg", Name: "cat.jpg"}
	o := &orchestrator{tagger: testClient(srv.URL), store: st, log: discardLogger()}
	if err := o.tagJob(asset)(ctx); err != nil {
		t.Fatalf("tagJob: %v", err)
	}

	// The AI-only tag lands with source='ai'.
	if src, ok := queryTagSource(t, ctx, dbPath, "photo-1", "animal"); !ok || src != "ai" {
		t.Errorf("animal tag source = %q (ok=%v), want ai", src, ok)
	}
	// The manual 'cat' link stays manual (AI must not overwrite it).
	if src, ok := queryTagSource(t, ctx, dbPath, "photo-1", "cat"); !ok || src != "manual" {
		t.Errorf("cat tag source = %q (ok=%v), want manual", src, ok)
	}
	// The caption lands in the ai layer with the sidecar's model id.
	if v, m, ok := queryAnnotation(t, ctx, dbPath, "photo-1", "ai", "caption"); !ok || v != `"a cat on a mat"` || m != "stub-v1" {
		t.Errorf("ai caption = (%q, %q, ok=%v), want (\"a cat on a mat\", stub-v1)", v, m, ok)
	}
	// The manual caption is a separate row, untouched.
	if v, _, ok := queryAnnotation(t, ctx, dbPath, "photo-1", "manual", "caption"); !ok || v != `"human wrote this"` {
		t.Errorf("manual caption = (%q, ok=%v), want \"human wrote this\"", v, ok)
	}
}
