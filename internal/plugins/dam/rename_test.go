package dam

import (
	"testing"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
)

func mkAsset(t *testing.T, id, name, display string) domain.Asset {
	t.Helper()
	aid, err := domain.NewAssetID(id)
	if err != nil {
		t.Fatal(err)
	}
	return domain.Asset{ID: aid, Name: name, DisplayName: display}
}

func nameByID(t *testing.T, m map[domain.AssetID]string, id string) string {
	t.Helper()
	aid, err := domain.NewAssetID(id)
	if err != nil {
		t.Fatal(err)
	}
	return m[aid]
}

func TestSequenceRenames(t *testing.T) {
	assets := []domain.Asset{
		mkAsset(t, "a1", "one.png", ""),
		mkAsset(t, "a2", "two.png", ""),
		mkAsset(t, "a3", "three.png", ""),
	}
	now := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	got := sequenceRenames(assets, "photo_{n}_{date}", 1, now)
	want := map[string]string{
		"a1": "photo_1_2026-01-02",
		"a2": "photo_2_2026-01-02",
		"a3": "photo_3_2026-01-02",
	}
	for id, w := range want {
		if g := nameByID(t, got, id); g != w {
			t.Errorf("%s => %q, want %q", id, g, w)
		}
	}
}

func TestSequenceRenames_CustomStart(t *testing.T) {
	assets := []domain.Asset{mkAsset(t, "a1", "x", ""), mkAsset(t, "a2", "y", "")}
	got := sequenceRenames(assets, "img{n}", 10, time.Now())
	if g := nameByID(t, got, "a1"); g != "img10" {
		t.Errorf("a1 => %q, want img10", g)
	}
	if g := nameByID(t, got, "a2"); g != "img11" {
		t.Errorf("a2 => %q, want img11", g)
	}
}

func TestFindReplaceRenames(t *testing.T) {
	assets := []domain.Asset{
		mkAsset(t, "a1", "IMG_001.jpg", ""),         // no display name -> uses Name
		mkAsset(t, "a2", "ignored", "vacation_IMG"), // display name wins over Name
	}
	got := findReplaceRenames(assets, "IMG", "photo")
	if g := nameByID(t, got, "a1"); g != "photo_001.jpg" {
		t.Errorf("a1 => %q, want photo_001.jpg", g)
	}
	if g := nameByID(t, got, "a2"); g != "vacation_photo" {
		t.Errorf("a2 => %q, want vacation_photo", g)
	}
}
