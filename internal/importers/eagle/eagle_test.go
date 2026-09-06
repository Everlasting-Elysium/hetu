package eagle_test

import (
	"context"
	"testing"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/importers"
	"github.com/Everlasting-Elysium/hetu/internal/importers/eagle"
)

func collect(t *testing.T, dir string) map[string]importers.ImportItem {
	t.Helper()
	src, err := eagle.Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if src.Kind() != importers.KindEagle {
		t.Fatalf("kind = %q", src.Kind())
	}
	items := map[string]importers.ImportItem{}
	if err := src.Each(context.Background(), func(it importers.ImportItem) error {
		items[it.Name] = it
		return nil
	}); err != nil {
		t.Fatalf("each: %v", err)
	}
	return items
}

func TestEagle_Each_MapsAllFields(t *testing.T) {
	items := collect(t, "testdata/sample.library")
	if len(items) != 3 {
		t.Fatalf("items = %d, want 3", len(items))
	}

	sunset, ok := items["sunset.png"]
	if !ok {
		t.Fatal("sunset.png missing")
	}
	if sunset.Rating != 5 {
		t.Errorf("rating = %d, want 5", sunset.Rating)
	}
	if sunset.Note != "A calm sunset over the sea" {
		t.Errorf("note = %q", sunset.Note)
	}
	if sunset.SourceURL != "https://example.com/sunset" {
		t.Errorf("url = %q", sunset.SourceURL)
	}
	// btime 1700000000000 ms → seconds → 2023-11-14T22:13:20Z (ms conversion).
	if want := time.UnixMilli(1700000000000).UTC(); !sunset.CreatedAt.Equal(want) {
		t.Errorf("createdAt = %v, want %v", sunset.CreatedAt, want)
	}
	// Folder id LFOLDER_PHOTOS resolves to a single-segment path ["Photos"].
	if got := sunset.Folders; len(got) != 1 || len(got[0]) != 1 || got[0][0] != "Photos" {
		t.Errorf("folders = %v, want [[Photos]]", got)
	}
	// Eagle tags are flat: "nature" and "nature/sky" are two distinct labels.
	if !hasTag(sunset.Tags, "nature") || !hasTag(sunset.Tags, "nature/sky") {
		t.Errorf("tags = %v, want nature + nature/sky", sunset.Tags)
	}

	// character.png sits in the nested folder Illustrations/Characters.
	char, ok := items["character.png"]
	if !ok {
		t.Fatal("character.png missing")
	}
	if got := char.Folders; len(got) != 1 || len(got[0]) != 2 ||
		got[0][0] != "Illustrations" || got[0][1] != "Characters" {
		t.Errorf("char folders = %v, want [[Illustrations Characters]]", got)
	}

	// logo.png has star 0 and no url/note — those map to zero values.
	logo, ok := items["logo.png"]
	if !ok {
		t.Fatal("logo.png missing")
	}
	if logo.Rating != 0 || logo.SourceURL != "" || logo.Note != "" {
		t.Errorf("logo = %+v, want zero rating/url/note", logo)
	}
}

func hasTag(tags []importers.NamePath, leaf string) bool {
	for _, p := range tags {
		if len(p) > 0 && p[len(p)-1] == leaf {
			return true
		}
	}
	return false
}
