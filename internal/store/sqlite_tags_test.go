package store_test

import (
	"testing"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
)

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
