package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/store"
)

func mustVersionID(t *testing.T, s string) domain.VersionID {
	t.Helper()
	id, err := domain.NewVersionID(s)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// addTwoVersions seeds an asset and adds one uploaded version, triggering the
// lazy v1 backfill; it returns the asset id and the (v1, v2) version ids. v1
// carries 320x240, v2 carries 100x80 so current-version resolution is testable.
func addTwoVersions(t *testing.T, ctx context.Context, st *store.SQLite, owner domain.OwnerID) (domain.AssetID, domain.VersionID, domain.VersionID) {
	t.Helper()
	aid := seedAsset(t, ctx, st, owner, "a1", "design.png")
	anchor := getAsset(t, ctx, st, owner, aid)
	v1 := mustVersionID(t, "v1")
	v2 := mustVersionID(t, "v2")
	base := domain.AssetVersion{
		ID: v1, AssetID: aid, Owner: owner, Provider: anchor.Provider,
		StoragePath: anchor.StoragePath, Hash: anchor.Hash, Size: anchor.Size,
		Width: 320, Height: 240, Note: "initial", CreatedAt: anchor.CreatedAt,
	}
	newV := domain.AssetVersion{
		ID: v2, AssetID: aid, Owner: owner, Provider: "local",
		StoragePath: ".hetu/versions/a1/v2/design.png", Hash: "h-v2", Size: 2,
		Width: 100, Height: 80, Note: "revised", CreatedAt: time.Now().UTC(),
	}
	created, err := st.AddVersion(ctx, owner, base, newV)
	if err != nil {
		t.Fatalf("add version: %v", err)
	}
	if created.VersionNo != 2 {
		t.Fatalf("new version_no = %d, want 2", created.VersionNo)
	}
	return aid, v1, v2
}

func TestSQLite_AddVersion_BackfillsAnchorAndSetsCurrent(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	aid, _, v2 := addTwoVersions(t, ctx, st, owner)

	versions, err := st.ListVersions(ctx, owner, aid)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 2 {
		t.Fatalf("versions = %d, want 2 (backfilled v1 + v2)", len(versions))
	}
	cur, err := st.CurrentVersionID(ctx, owner, aid)
	if err != nil {
		t.Fatal(err)
	}
	if cur != v2.String() {
		t.Fatalf("current = %q, want v2", cur)
	}
	// Reads resolve display fields to the current version (v2 = 100x80).
	a := getAsset(t, ctx, st, owner, aid)
	if a.Width != 100 || a.Height != 80 {
		t.Fatalf("current dims = %dx%d, want 100x80", a.Width, a.Height)
	}
}

// TestSQLite_DeleteVersion_RefusesCurrent locks the atomic guard: the current
// version cannot be deleted (deleted=false, no error), while a non-current
// version is removed and the current pointer stays valid.
func TestSQLite_DeleteVersion_RefusesCurrent(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	aid, v1, v2 := addTwoVersions(t, ctx, st, owner)

	deleted, err := st.DeleteVersion(ctx, owner, aid, v2)
	if err != nil {
		t.Fatalf("delete current: %v", err)
	}
	if deleted {
		t.Fatal("deleted=true for the current version, want false (refused)")
	}
	if v, _ := st.ListVersions(ctx, owner, aid); len(v) != 2 {
		t.Fatalf("versions after refused delete = %d, want 2", len(v))
	}

	deleted, err = st.DeleteVersion(ctx, owner, aid, v1)
	if err != nil {
		t.Fatalf("delete v1: %v", err)
	}
	if !deleted {
		t.Fatal("deleted=false for a non-current version, want true")
	}
	if v, _ := st.ListVersions(ctx, owner, aid); len(v) != 1 {
		t.Fatalf("versions after delete = %d, want 1", len(v))
	}
	if cur, _ := st.CurrentVersionID(ctx, owner, aid); cur != v2.String() {
		t.Fatalf("current = %q, want v2 still valid", cur)
	}
}

// TestSQLite_SetCurrentVersion_RejectsMissing locks the atomic existence check:
// pointing current at a non-existent version is rejected (ErrNotFound) and never
// leaves current_version_id dangling — the defense against the set/delete race.
func TestSQLite_SetCurrentVersion_RejectsMissing(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	aid, v1, _ := addTwoVersions(t, ctx, st, owner)

	if err := st.SetCurrentVersion(ctx, owner, aid, v1); err != nil {
		t.Fatalf("set current v1: %v", err)
	}
	a := getAsset(t, ctx, st, owner, aid)
	if a.Width != 320 || a.Height != 240 {
		t.Fatalf("after switch dims = %dx%d, want 320x240 (v1)", a.Width, a.Height)
	}

	ghost := mustVersionID(t, "ghost")
	if err := st.SetCurrentVersion(ctx, owner, aid, ghost); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("set current ghost err = %v, want ErrNotFound", err)
	}
	if cur, _ := st.CurrentVersionID(ctx, owner, aid); cur != v1.String() {
		t.Fatalf("current = %q, want v1 unchanged", cur)
	}
}
