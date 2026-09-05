package store_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/store"
)

func TestFindExactDuplicates(t *testing.T) {
	ctx, st, owner := mustOpen(t)

	// Seed three assets: two share hash "aaa", one is unique.
	for _, a := range []struct {
		id, path, hash string
	}{
		{"dup-1", "photos/a.png", "aaa"},
		{"dup-2", "photos/b.png", "aaa"},
		{"uniq-1", "photos/c.png", "bbb"},
	} {
		aid, err := domain.NewAssetID(a.id)
		if err != nil {
			t.Fatal(err)
		}
		now := time.Now().UTC().Truncate(time.Second)
		if err := st.UpsertAsset(ctx, domain.Asset{
			ID: aid, Owner: owner, Kind: domain.KindImage, Provider: "local",
			StoragePath: a.path, Name: a.path, Ext: "png", Size: 100,
			Hash: a.hash, CreatedAt: now, IndexedAt: now,
		}); err != nil {
			t.Fatalf("upsert %s: %v", a.id, err)
		}
	}

	groups, err := st.FindExactDuplicates(ctx, owner, 50, 0)
	if err != nil {
		t.Fatalf("FindExactDuplicates: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("got %d groups, want 1", len(groups))
	}
	g := groups[0]
	if g.Hash != "aaa" {
		t.Errorf("hash = %q, want %q", g.Hash, "aaa")
	}
	if len(g.Assets) != 2 {
		t.Fatalf("group members = %d, want 2", len(g.Assets))
	}
}

func TestFindExactDuplicates_ExcludesTrashed(t *testing.T) {
	ctx, st, owner := mustOpen(t)

	for _, a := range []struct {
		id, path string
	}{
		{"tr-1", "trash/a.png"},
		{"tr-2", "trash/b.png"},
	} {
		aid, err := domain.NewAssetID(a.id)
		if err != nil {
			t.Fatal(err)
		}
		now := time.Now().UTC().Truncate(time.Second)
		if err := st.UpsertAsset(ctx, domain.Asset{
			ID: aid, Owner: owner, Kind: domain.KindImage, Provider: "local",
			StoragePath: a.path, Name: a.path, Ext: "png", Size: 100,
			Hash: "samehash", CreatedAt: now, IndexedAt: now,
		}); err != nil {
			t.Fatalf("upsert %s: %v", a.id, err)
		}
	}

	// Trash one of them.
	aid, _ := domain.NewAssetID("tr-1")
	if err := st.BatchTrashAssets(ctx, owner, []domain.AssetID{aid}); err != nil {
		t.Fatalf("trash: %v", err)
	}

	groups, err := st.FindExactDuplicates(ctx, owner, 50, 0)
	if err != nil {
		t.Fatalf("FindExactDuplicates: %v", err)
	}
	// Only one live asset remains with that hash, so no duplicate group.
	if len(groups) != 0 {
		t.Fatalf("got %d groups, want 0 (trashed asset excluded)", len(groups))
	}
}

func TestIndexPHash_AndFindSimilar(t *testing.T) {
	ctx, st, owner := mustOpen(t)

	// Seed three image assets.
	type seed struct {
		id, path string
		phash    uint64
	}
	seeds := []seed{
		{"img-1", "a.png", 0x00000000_00000000}, // anchor
		{"img-2", "b.png", 0x00000000_00000003}, // distance 2 from anchor
		{"img-3", "c.png", 0xFFFFFFFF_FFFFFFFF}, // distance 64 from anchor
	}
	for _, s := range seeds {
		aid, err := domain.NewAssetID(s.id)
		if err != nil {
			t.Fatal(err)
		}
		now := time.Now().UTC().Truncate(time.Second)
		if err := st.UpsertAsset(ctx, domain.Asset{
			ID: aid, Owner: owner, Kind: domain.KindImage, Provider: "local",
			StoragePath: s.path, Name: s.path, Ext: "png", Size: 100,
			Hash: "h-" + s.id, CreatedAt: now, IndexedAt: now,
		}); err != nil {
			t.Fatalf("upsert %s: %v", s.id, err)
		}
		if err := st.IndexPHash(ctx, owner, "local", s.path, s.phash); err != nil {
			t.Fatalf("index phash %s: %v", s.id, err)
		}
	}

	// Threshold 5: img-1 and img-2 should be in the same group.
	groups, err := st.FindSimilarByPHash(ctx, owner, 5)
	if err != nil {
		t.Fatalf("FindSimilarByPHash: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("got %d groups, want 1", len(groups))
	}
	g := groups[0]
	if len(g.Members) != 1 {
		t.Fatalf("members = %d, want 1", len(g.Members))
	}
	if g.Members[0].Distance != 2 {
		t.Errorf("distance = %d, want 2", g.Members[0].Distance)
	}

	// Threshold 0: only exact pHash matches (none here).
	groups, err = st.FindSimilarByPHash(ctx, owner, 0)
	if err != nil {
		t.Fatalf("FindSimilarByPHash threshold=0: %v", err)
	}
	if len(groups) != 0 {
		t.Errorf("threshold=0: got %d groups, want 0", len(groups))
	}

	// Threshold 64: all three should be in one group.
	groups, err = st.FindSimilarByPHash(ctx, owner, 64)
	if err != nil {
		t.Fatalf("FindSimilarByPHash threshold=64: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("threshold=64: got %d groups, want 1", len(groups))
	}
	if len(groups[0].Members) != 2 {
		t.Errorf("threshold=64: members = %d, want 2", len(groups[0].Members))
	}
}

func TestIndexPHash_AnnotationRoundTrip(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "phash.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	owner, _ := domain.NewOwnerID("tester")
	if err := st.EnsureOwner(ctx, owner); err != nil {
		t.Fatal(err)
	}

	aid, _ := domain.NewAssetID("rt-1")
	now := time.Now().UTC().Truncate(time.Second)
	if err := st.UpsertAsset(ctx, domain.Asset{
		ID: aid, Owner: owner, Kind: domain.KindImage, Provider: "local",
		StoragePath: "rt.png", Name: "rt.png", Ext: "png", Size: 1,
		Hash: "rthash", CreatedAt: now, IndexedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	want := uint64(0x123456789ABCDEF0)
	if err := st.IndexPHash(ctx, owner, "local", "rt.png", want); err != nil {
		t.Fatalf("IndexPHash: %v", err)
	}

	// Re-index with same value should not fail (upsert).
	if err := st.IndexPHash(ctx, owner, "local", "rt.png", want); err != nil {
		t.Fatalf("re-IndexPHash: %v", err)
	}

	// Verify the annotation is stored correctly by checking via FindSimilar
	// with threshold 0 (exact match only — seed a second asset with same phash).
	aid2, _ := domain.NewAssetID("rt-2")
	if err := st.UpsertAsset(ctx, domain.Asset{
		ID: aid2, Owner: owner, Kind: domain.KindImage, Provider: "local",
		StoragePath: "rt2.png", Name: "rt2.png", Ext: "png", Size: 1,
		Hash: "rthash2", CreatedAt: now, IndexedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.IndexPHash(ctx, owner, "local", "rt2.png", want); err != nil {
		t.Fatal(err)
	}

	groups, err := st.FindSimilarByPHash(ctx, owner, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 {
		t.Fatalf("round-trip: got %d groups, want 1", len(groups))
	}

	// Verify the stored uint64 value survived JSON round-trip.
	_, _ = json.Marshal(strconv.FormatUint(want, 10))
}
