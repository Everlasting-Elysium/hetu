package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/store"
)

// tagNames lists an asset's tag names (ordered by name) for assertions.
func tagNames(t *testing.T, ctx context.Context, st *store.SQLite, id domain.AssetID) []string {
	t.Helper()
	tags, err := st.ListAssetTags(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, len(tags))
	for i, tg := range tags {
		names[i] = tg.Name
	}
	return names
}

func mkTag(t *testing.T, owner domain.OwnerID, id, name, color string) domain.Tag {
	t.Helper()
	tid, err := domain.NewTagID(id)
	if err != nil {
		t.Fatal(err)
	}
	return domain.Tag{ID: tid, Owner: owner, Name: name, Color: color}
}

func TestSQLite_TagCRUD(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	red := mkTag(t, owner, "t1", "red", "#FF0000")
	blue := mkTag(t, owner, "t2", "blue", "#0000FF")
	if err := st.CreateTag(ctx, red); err != nil {
		t.Fatalf("create red: %v", err)
	}
	if err := st.CreateTag(ctx, blue); err != nil {
		t.Fatalf("create blue: %v", err)
	}

	tags, err := st.ListTags(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 2 {
		t.Fatalf("tags = %d, want 2", len(tags))
	}
	// Ordered by name: blue, red.
	if tags[0].Name != "blue" || tags[1].Name != "red" {
		t.Fatalf("order = %q,%q, want blue,red", tags[0].Name, tags[1].Name)
	}
	if tags[0].Color != "#0000FF" {
		t.Errorf("blue color = %q, want #0000FF", tags[0].Color)
	}

	if err := st.DeleteTag(ctx, owner, red.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	tags, err = st.ListTags(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 1 || tags[0].Name != "blue" {
		t.Fatalf("after delete = %+v, want [blue]", tags)
	}
}

func TestSQLite_BatchTagsAndUntag(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	a1 := seedAsset(t, ctx, st, owner, "a1", "1.png")
	a2 := seedAsset(t, ctx, st, owner, "a2", "2.png")
	alpha := mkTag(t, owner, "t1", "alpha", "")
	beta := mkTag(t, owner, "t2", "beta", "")
	if err := st.CreateTag(ctx, alpha); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateTag(ctx, beta); err != nil {
		t.Fatal(err)
	}

	assets := []domain.AssetID{a1, a2}
	tagIDs := []domain.TagID{alpha.ID, beta.ID}
	if err := st.BatchAddTags(ctx, owner, assets, tagIDs); err != nil {
		t.Fatalf("add tags: %v", err)
	}
	// Idempotent: re-adding must not duplicate rows.
	if err := st.BatchAddTags(ctx, owner, assets, tagIDs); err != nil {
		t.Fatalf("re-add tags: %v", err)
	}
	got, err := st.ListAssetTags(ctx, a1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "alpha" || got[1].Name != "beta" {
		t.Fatalf("a1 tags = %+v, want [alpha beta]", got)
	}

	if err := st.BatchRemoveTags(ctx, owner, assets, alpha.ID); err != nil {
		t.Fatalf("remove: %v", err)
	}
	for _, id := range assets {
		got, err := st.ListAssetTags(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].Name != "beta" {
			t.Fatalf("%s tags after untag = %+v, want [beta]", id, got)
		}
	}
}

// mkChildTag builds an owner-scoped tag nested under parentID.
func mkChildTag(t *testing.T, owner domain.OwnerID, id, name, parentID string) domain.Tag {
	t.Helper()
	tid, err := domain.NewTagID(id)
	if err != nil {
		t.Fatal(err)
	}
	return domain.Tag{ID: tid, Owner: owner, Name: name, ParentID: parentID}
}

// getTag fetches a tag from the owner's list by id, or nil if absent.
func findTag(t *testing.T, ctx context.Context, st *store.SQLite, owner domain.OwnerID, id string) *domain.Tag {
	t.Helper()
	tags, err := st.ListTags(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	for i := range tags {
		if tags[i].ID.String() == id {
			return &tags[i]
		}
	}
	return nil
}

func TestSQLite_MergeTags_dedupAndDelete(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	a1 := seedAsset(t, ctx, st, owner, "a1", "1.png")
	a2 := seedAsset(t, ctx, st, owner, "a2", "2.png")
	a3 := seedAsset(t, ctx, st, owner, "a3", "3.png")
	from := mkTag(t, owner, "tf", "from", "")
	into := mkTag(t, owner, "ti", "into", "")
	if err := st.CreateTag(ctx, from); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateTag(ctx, into); err != nil {
		t.Fatal(err)
	}
	// a1 has only from; a2 has BOTH from and into (dedup case); a3 has only into.
	if err := st.BatchAddTags(ctx, owner, []domain.AssetID{a1, a2}, []domain.TagID{from.ID}); err != nil {
		t.Fatal(err)
	}
	if err := st.BatchAddTags(ctx, owner, []domain.AssetID{a2, a3}, []domain.TagID{into.ID}); err != nil {
		t.Fatal(err)
	}

	if err := st.MergeTags(ctx, owner, from.ID, into.ID); err != nil {
		t.Fatalf("merge: %v", err)
	}

	// The source tag is gone globally.
	if findTag(t, ctx, st, owner, "tf") != nil {
		t.Fatal("from tag still exists after merge")
	}
	// Every asset that had from now has into, and a2 keeps a SINGLE into row.
	for _, id := range []domain.AssetID{a1, a2, a3} {
		names := tagNames(t, ctx, st, id)
		if len(names) != 1 || names[0] != "into" {
			t.Fatalf("%s tags after merge = %v, want [into]", id, names)
		}
	}
}

func TestSQLite_MergeTags_reparentsChildren(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	// grandparent -> from -> child; from is merged into sibling "into".
	gp := mkTag(t, owner, "gp", "grandparent", "")
	from := mkChildTag(t, owner, "tf", "from", "gp")
	child := mkChildTag(t, owner, "tc", "child", "tf")
	into := mkTag(t, owner, "ti", "into", "")
	for _, tg := range []domain.Tag{gp, from, child, into} {
		if err := st.CreateTag(ctx, tg); err != nil {
			t.Fatal(err)
		}
	}

	if err := st.MergeTags(ctx, owner, from.ID, into.ID); err != nil {
		t.Fatalf("merge: %v", err)
	}

	// child is promoted to from's parent (grandparent), never left dangling at
	// the deleted "from" tag.
	got := findTag(t, ctx, st, owner, "tc")
	if got == nil {
		t.Fatal("child tag vanished")
	}
	if got.ParentID != "gp" {
		t.Fatalf("child parent = %q, want gp (promoted to from's parent)", got.ParentID)
	}
}

// TestSQLite_MergeTags_targetIsChild proves the tricky case: merging a tag whose
// merge target is itself a child of it. The target must survive with a valid
// (promoted) parent, never a dangling or self-referential parent_id.
func TestSQLite_MergeTags_targetIsChild(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	from := mkTag(t, owner, "tf", "from", "")
	into := mkChildTag(t, owner, "ti", "into", "tf") // into is a child of from
	if err := st.CreateTag(ctx, from); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateTag(ctx, into); err != nil {
		t.Fatal(err)
	}

	if err := st.MergeTags(ctx, owner, from.ID, into.ID); err != nil {
		t.Fatalf("merge: %v", err)
	}

	got := findTag(t, ctx, st, owner, "ti")
	if got == nil {
		t.Fatal("into tag vanished")
	}
	// from's parent was empty (top level), so into is promoted to top level —
	// not pointing at the deleted "from", and not at itself.
	if got.ParentID == "tf" || got.ParentID == "ti" {
		t.Fatalf("into parent = %q, want promoted (not dangling/self)", got.ParentID)
	}
}

func TestSQLite_MergeTags_errors(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	a := mkTag(t, owner, "ta", "a", "")
	if err := st.CreateTag(ctx, a); err != nil {
		t.Fatal(err)
	}
	ghost, _ := domain.NewTagID("ghost")

	// Merge into self.
	if err := st.MergeTags(ctx, owner, a.ID, a.ID); !errors.Is(err, domain.ErrSameTag) {
		t.Fatalf("self merge err = %v, want ErrSameTag", err)
	}
	// Unknown source.
	if err := st.MergeTags(ctx, owner, ghost, a.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unknown source err = %v, want ErrNotFound", err)
	}
	// Unknown target.
	if err := st.MergeTags(ctx, owner, a.ID, ghost); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unknown target err = %v, want ErrNotFound", err)
	}
	// Foreign tag (another owner) is not found for this owner.
	other, err := domain.NewOwnerID("intruder")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureOwner(ctx, other); err != nil {
		t.Fatal(err)
	}
	theirs := mkTag(t, other, "tx", "theirs", "")
	if err := st.CreateTag(ctx, theirs); err != nil {
		t.Fatal(err)
	}
	if err := st.MergeTags(ctx, owner, theirs.ID, a.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("foreign source err = %v, want ErrNotFound", err)
	}
}

func TestSQLite_BatchReplaceTag_subsetOnly(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	a1 := seedAsset(t, ctx, st, owner, "a1", "1.png")
	a2 := seedAsset(t, ctx, st, owner, "a2", "2.png")
	a3 := seedAsset(t, ctx, st, owner, "a3", "3.png") // NOT in the replace subset
	from := mkTag(t, owner, "tf", "from", "")
	into := mkTag(t, owner, "ti", "into", "")
	if err := st.CreateTag(ctx, from); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateTag(ctx, into); err != nil {
		t.Fatal(err)
	}
	// All three carry "from"; a2 ALSO carries "into" (dedup case).
	if err := st.BatchAddTags(ctx, owner, []domain.AssetID{a1, a2, a3}, []domain.TagID{from.ID}); err != nil {
		t.Fatal(err)
	}
	if err := st.BatchAddTags(ctx, owner, []domain.AssetID{a2}, []domain.TagID{into.ID}); err != nil {
		t.Fatal(err)
	}

	// Replace only within {a1, a2}.
	if err := st.BatchReplaceTag(ctx, owner, []domain.AssetID{a1, a2}, from.ID, into.ID); err != nil {
		t.Fatalf("replace: %v", err)
	}

	// a1: from -> into.
	if names := tagNames(t, ctx, st, a1); len(names) != 1 || names[0] != "into" {
		t.Fatalf("a1 tags = %v, want [into]", names)
	}
	// a2: had both, keeps a single into row (dedup, from removed).
	if names := tagNames(t, ctx, st, a2); len(names) != 1 || names[0] != "into" {
		t.Fatalf("a2 tags = %v, want [into]", names)
	}
	// a3: untouched, still carries "from" (outside the subset).
	if names := tagNames(t, ctx, st, a3); len(names) != 1 || names[0] != "from" {
		t.Fatalf("a3 tags = %v, want [from] (unaffected)", names)
	}
	// The source tag itself is NOT deleted (a3 still uses it).
	if findTag(t, ctx, st, owner, "tf") == nil {
		t.Fatal("from tag deleted by batch replace; it must survive")
	}
}

func TestSQLite_BatchReplaceTag_sameTag(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	a1 := seedAsset(t, ctx, st, owner, "a1", "1.png")
	tag := mkTag(t, owner, "tx", "x", "")
	if err := st.CreateTag(ctx, tag); err != nil {
		t.Fatal(err)
	}
	if err := st.BatchAddTags(ctx, owner, []domain.AssetID{a1}, []domain.TagID{tag.ID}); err != nil {
		t.Fatal(err)
	}
	// Replacing a tag with itself must error, not silently strip the tag.
	if err := st.BatchReplaceTag(ctx, owner, []domain.AssetID{a1}, tag.ID, tag.ID); !errors.Is(err, domain.ErrSameTag) {
		t.Fatalf("same-tag replace err = %v, want ErrSameTag", err)
	}
	if names := tagNames(t, ctx, st, a1); len(names) != 1 || names[0] != "x" {
		t.Fatalf("a1 tags after rejected replace = %v, want [x] intact", names)
	}
}
