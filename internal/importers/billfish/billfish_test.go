package billfish_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/importers"
	"github.com/Everlasting-Elysium/hetu/internal/importers/billfish"
)

const sampleDir = "testdata/sample"

func collect(t *testing.T, dir string) map[string]importers.ImportItem {
	t.Helper()
	src, err := billfish.Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if src.Kind() != importers.KindBillfish {
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

func TestBillfish_Each_MapsAllFields(t *testing.T) {
	items := collect(t, sampleDir)
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2", len(items))
	}

	photo, ok := items["photo.png"]
	if !ok {
		t.Fatal("photo.png missing")
	}
	if photo.Rating != 4 {
		t.Errorf("rating = %d, want 4", photo.Rating)
	}
	if photo.Note != "Mountain vista at dawn" {
		t.Errorf("note = %q", photo.Note)
	}
	if photo.SourceURL != "https://source.example.com/photo" {
		t.Errorf("origin = %q", photo.SourceURL)
	}
	if want := time.Unix(1700000000, 0).UTC(); !photo.CreatedAt.Equal(want) {
		t.Errorf("createdAt = %v, want %v", photo.CreatedAt, want)
	}
	// Folder via bf_file.pid -> "Nature" (root).
	if got := photo.Folders; len(got) != 1 || len(got[0]) != 1 || got[0][0] != "Nature" {
		t.Errorf("folders = %v, want [[Nature]]", got)
	}
	// Tags: landscape (root) and landscape/mountain (hierarchy via pid).
	if !hasPath(photo.Tags, "landscape") || !hasPath(photo.Tags, "landscape", "mountain") {
		t.Errorf("tags = %v, want landscape + landscape/mountain", photo.Tags)
	}

	// diagram.png sits in nested folder Work/Diagrams.
	diagram, ok := items["diagram.png"]
	if !ok {
		t.Fatal("diagram.png missing")
	}
	if got := diagram.Folders; len(got) != 1 || len(got[0]) != 2 ||
		got[0][0] != "Work" || got[0][1] != "Diagrams" {
		t.Errorf("diagram folders = %v, want [[Work Diagrams]]", got)
	}
}

// TestBillfish_ReadOnly is the acceptance guard: importing must never modify the
// source library. It hashes the catalog before and after a full read and checks
// that no journal/WAL sidecar files were created.
func TestBillfish_ReadOnly(t *testing.T) {
	dbPath := filepath.Join(sampleDir, ".bf", "billfish.db")
	before := hashFile(t, dbPath)

	src, err := billfish.Open(sampleDir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := src.Each(context.Background(), func(importers.ImportItem) error { return nil }); err != nil {
		t.Fatalf("each: %v", err)
	}

	if after := hashFile(t, dbPath); after != before {
		t.Fatalf("catalog modified: %s != %s", before, after)
	}
	for _, side := range []string{"-wal", "-shm", "-journal"} {
		if _, err := os.Stat(dbPath + side); err == nil {
			t.Errorf("sidecar %s created (source not opened immutable)", side)
		}
	}
}

func hashFile(t *testing.T, path string) string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		t.Fatalf("hash: %v", err)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func hasPath(tags []importers.NamePath, segs ...string) bool {
	for _, p := range tags {
		if len(p) != len(segs) {
			continue
		}
		match := true
		for i := range segs {
			if p[i] != segs[i] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
