package importers_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/importers"
)

// sliceSource is a minimal in-memory importers.Source yielding a fixed item list
// in order, so a merge test can feed two content-identical items with distinct
// paths/metadata through ImportSource without an on-disk Eagle library.
type sliceSource struct {
	kind  importers.SourceKind
	items []importers.ImportItem
}

func (s sliceSource) Kind() importers.SourceKind { return s.kind }

func (s sliceSource) Each(_ context.Context, fn func(importers.ImportItem) error) error {
	for _, it := range s.items {
		if err := fn(it); err != nil {
			return err
		}
	}
	return nil
}

// copyFixture writes the sample PNG's bytes to dir/name and returns its absolute
// path, so two imports can carry identical content (same hash) at distinct paths.
func copyFixture(t *testing.T, dir, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(eagleLib, "images", "MEAGLE0000000000000001.info", "sunset.png"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	dst := filepath.Join(dir, name)
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	abs, err := filepath.Abs(dst)
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

// TestImportSource_ConflictMerge feeds two content-identical items (distinct
// paths) through ImportSource under the merge policy: the first is imported, the
// second folds its metadata into that asset — no second asset row, tags
// combined, and the rating the first import set is preserved (never overwritten).
func TestImportSource_ConflictMerge(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	dir := t.TempDir()
	pathA := copyFixture(t, dir, "a.png")
	pathB := copyFixture(t, dir, "b.png")

	src := sliceSource{kind: importers.KindEagle, items: []importers.ImportItem{
		{AbsPath: pathA, Name: "a.png", Tags: []importers.NamePath{{"first"}}, Rating: 4},
		{AbsPath: pathB, Name: "b.png", Tags: []importers.NamePath{{"second"}}, Rating: 2, Note: "merged"},
	}}
	res, err := importers.New(h.k, h.owner).ImportSource(ctx, src,
		importers.Options{Mode: importers.ModeIndex, Conflict: importers.ConflictMerge}, domain.JobID{})
	if err != nil {
		t.Fatalf("import merge: %v", err)
	}
	if res.Imported != 1 || res.Merged != 1 || res.Skipped != 0 || res.Failed != 0 {
		t.Fatalf("res = %+v, want Imported:1 Merged:1 Skipped:0 Failed:0", res)
	}

	assets, err := h.st.ListAssets(ctx, h.owner, 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 1 {
		t.Fatalf("assets = %d, want 1 (merge must not create a second asset)", len(assets))
	}
	a := assets[0]
	if a.Rating != 4 {
		t.Errorf("rating = %d, want 4 (merge must not overwrite an existing rating)", a.Rating)
	}
	tags, _ := h.st.ListAssetTags(ctx, a.ID)
	names := map[string]bool{}
	for _, tg := range tags {
		names[tg.Name] = true
	}
	if !names["first"] || !names["second"] {
		t.Errorf("tags = %v, want first + second (merge folds in new tags)", names)
	}
}

// TestImportItem_ConflictMerge drives the merge policy one item at a time: with
// no duplicate present it imports (OutcomeImported); a later content-identical
// item at a different path merges into that asset (OutcomeMerged, same id) and
// fills the still-empty rating (proving the empty-field guard writes when unset).
func TestImportItem_ConflictMerge(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	dir := t.TempDir()
	pathA := copyFixture(t, dir, "a.png")
	pathB := copyFixture(t, dir, "b.png")

	svc := importers.New(h.k, h.owner)
	first, outcome, err := svc.ImportItem(ctx,
		importers.ImportItem{AbsPath: pathA, Name: "a.png", Tags: []importers.NamePath{{"first"}}},
		importers.Options{Mode: importers.ModeIndex, Conflict: importers.ConflictMerge})
	if err != nil {
		t.Fatalf("first import: %v", err)
	}
	if outcome != importers.OutcomeImported {
		t.Fatalf("outcome = %q, want imported (no duplicate yet)", outcome)
	}

	merged, outcome, err := svc.ImportItem(ctx,
		importers.ImportItem{AbsPath: pathB, Name: "b.png", Tags: []importers.NamePath{{"second"}}, Rating: 3},
		importers.Options{Mode: importers.ModeIndex, Conflict: importers.ConflictMerge})
	if err != nil {
		t.Fatalf("merge import: %v", err)
	}
	if outcome != importers.OutcomeMerged {
		t.Fatalf("outcome = %q, want merged (content duplicate)", outcome)
	}
	if merged.ID.String() != first.ID.String() {
		t.Fatalf("merged id = %s, want existing %s", merged.ID.String(), first.ID.String())
	}

	assets, _ := h.st.ListAssets(ctx, h.owner, 100, 0)
	if len(assets) != 1 {
		t.Fatalf("assets = %d, want 1 (merge lands no new asset)", len(assets))
	}
	if assets[0].Rating != 3 {
		t.Errorf("rating = %d, want 3 (merge fills an empty rating)", assets[0].Rating)
	}
}
